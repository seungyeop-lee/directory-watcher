package domain

import (
	"fmt"
	"slices"
	"sync"

	"github.com/seungyeop-lee/directory-watcher/v3/internal/helper"
)

// Interrupt는 새 실행을 시작하기 전에 돌고 있는 어떤 실행을 취소할지 정한다.
type Interrupt int

const (
	// InterruptNone은 아무 실행도 취소하지 않고 자리가 날 때까지 기다린다.
	InterruptNone Interrupt = iota
	// InterruptSamePath는 새 실행과 같은 경로를 처리하던 실행을 취소한다. 그런 실행이 없으면 자리가 날 때까지 기다린다.
	InterruptSamePath
	// InterruptOldest는 자리가 다 찼을 때 가장 먼저 시작한 실행을 취소한다.
	InterruptOldest
)

// RunPolicy는 감시 대상의 hook 실행을 동시에 Concurrency개까지 돌리고, 새 실행이 Interrupt에 따라 돌고 있는
// 실행을 취소하게 한다. Concurrency는 1 이상이다.
type RunPolicy struct {
	Concurrency int
	Interrupt   Interrupt
}

type HookInfo struct {
	OnStartWatch   Cmd
	OnBeforeChange Cmd
	OnChange       Cmd
	OnAfterChange  Cmd
	OnFinishWatch  Cmd
}

// HookFunc는 감시 대상 하나의 hook을 실행한다.
//   - RunStartHook은 onStartWatch를 시작하고 끝나기를 기다리지 않는다. onStartWatch가 도는 동안에는 자리가 모두 찬
//     것으로 보므로, interrupt: oldest일 때만 첫 실행이 onStartWatch를 취소한다.
//   - RunChangeHook은 RunPolicy에 따라 자리가 나고 취소한 실행이 끝날 때까지 기다린 뒤 event의 실행을 시작하고,
//     그 실행이 끝나기를 기다리지 않는다. 실행 하나는 onBeforeChange, onChange, onAfterChange를 차례로 돌린다.
//   - Cancel은 돌고 있는 모든 실행을 취소하고, 기다리던 RunChangeHook을 포함해 새 실행을 시작하지 않게 한다.
//   - RunFinishHook은 Cancel한 뒤 모든 실행이 끝나기를 기다려 onFinishWatch를 실행한다.
type HookFunc struct {
	RunStartHook  func()
	RunChangeHook func(event Event)
	RunFinishHook func()
	Cancel        func()
}

type HookFuncBuilder func(path Path) *HookFunc

// hookRun은 돌고 있는 실행 하나다. start이면 onStartWatch이고, 아니면 path를 처리하는 변경 실행이다.
type hookRun struct {
	ctx   *helper.WatcherContext
	path  Path
	start bool
}

type hookRunner struct {
	policy RunPolicy
	alias  string
	target Path
	logger helper.Logger
	base   *helper.WatcherContext

	mu sync.Mutex
	// ended는 실행이 끝나거나 Cancel을 부를 때 기다리는 쪽을 깨운다.
	ended *sync.Cond
	// runs는 돌고 있는 실행이고, 시작한 순서대로 둔다.
	runs    []*hookRun
	stopped bool
}

func (p RunPolicy) BuildHookFuncBuilder(alias string, info HookInfo, logger helper.Logger) HookFuncBuilder {
	return func(path Path) *HookFunc {
		r := &hookRunner{policy: p, alias: alias, target: path, logger: logger, base: helper.NewWatcherContext(alias)}
		r.ended = sync.NewCond(&r.mu)
		startCommands := []Cmd{info.OnStartWatch}
		changeCommands := []Cmd{info.OnBeforeChange, info.OnChange, info.OnAfterChange}
		return &HookFunc{
			RunStartHook: func() {
				r.mu.Lock()
				defer r.mu.Unlock()
				if !r.stopped {
					r.start(&hookRun{start: true}, startCommands, nil)
				}
			},
			RunChangeHook: func(event Event) { r.admit(event, changeCommands) },
			Cancel:        r.cancel,
			RunFinishHook: func() {
				r.cancel()
				r.mu.Lock()
				for len(r.runs) > 0 {
					r.ended.Wait()
				}
				r.mu.Unlock()
				finish := r.base.NewRun()
				defer r.base.Cancel()
				defer finish.Cancel()
				_ = r.runCommand(finish, info.OnFinishWatch, nil)
			},
		}
	}
}

// admit은 event의 실행을 시작할 수 있을 때까지 기다린 뒤 시작한다. 그동안 Cancel을 부르면 시작하지 않는다.
func (r *hookRunner) admit(event Event, commands []Cmd) {
	r.mu.Lock()
	defer r.mu.Unlock()
	// canceled는 이 실행을 시작하려고 취소해서, 끝나기를 기다리는 실행이다.
	var canceled []*hookRun
	for !r.stopped {
		canceled = slices.DeleteFunc(canceled, func(run *hookRun) bool { return !slices.Contains(r.runs, run) })
		if len(canceled) == 0 {
			canceled = r.toCancel(event.Path)
			for _, run := range canceled {
				run.ctx.Cancel()
			}
		}
		if len(canceled) == 0 && !r.full() {
			r.start(&hookRun{path: event.Path}, commands, &event)
			return
		}
		r.ended.Wait()
	}
}

// full은 새 실행이 들어갈 자리가 없는지 판단한다. onStartWatch는 자리를 모두 차지한다. r.mu를 잡고 부른다.
func (r *hookRunner) full() bool {
	return len(r.runs) >= r.policy.Concurrency || len(r.runs) > 0 && r.runs[0].start
}

// toCancel은 경로 path의 새 실행을 시작하려고 취소할 실행을 고른다. r.mu를 잡고 부른다.
func (r *hookRunner) toCancel(path Path) []*hookRun {
	switch r.policy.Interrupt {
	case InterruptSamePath:
		var same []*hookRun
		for _, run := range r.runs {
			if !run.start && run.path == path {
				same = append(same, run)
			}
		}
		return same
	case InterruptOldest:
		if r.full() {
			return []*hookRun{r.runs[0]}
		}
	}
	return nil
}

// start는 run으로 commands를 차례로 실행하기 시작한다. r.mu를 잡고 부른다.
func (r *hookRunner) start(run *hookRun, commands []Cmd, event *Event) {
	run.ctx = r.base.NewRun()
	r.runs = append(r.runs, run)
	go func() {
		for _, command := range commands {
			if run.ctx.Context().Err() != nil {
				break
			}
			if err := r.runCommand(run.ctx, command, event); err != nil {
				break
			}
		}
		run.ctx.Cancel()
		r.mu.Lock()
		defer r.mu.Unlock()
		r.runs = slices.DeleteFunc(r.runs, func(other *hookRun) bool { return other == run })
		r.ended.Broadcast()
	}()
}

func (r *hookRunner) cancel() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.stopped = true
	for _, run := range r.runs {
		run.ctx.Cancel()
	}
	r.ended.Broadcast()
}

func (r *hookRunner) runCommand(ctx *helper.WatcherContext, command Cmd, event *Event) error {
	err := command.Run(ctx, r.target, event)
	// 새 변경이나 감시 종료로 실행을 취소하면 명령도 오류로 끝나는데, 이는 실패가 아니므로 기록하지 않는다.
	if err != nil && ctx.Context().Err() == nil {
		r.logger.Error(fmt.Sprintf("%s hook command failed, skipping remaining commands: %v", r.alias, err))
	}
	return err
}
