package service

import (
	"fmt"
	"slices"
	"sync"

	"github.com/seungyeop-lee/directory-watcher/v3/internal/app/domain"
	"github.com/seungyeop-lee/directory-watcher/v3/internal/helper"
	"github.com/seungyeop-lee/directory-watcher/v3/internal/process"
)

type hookInfo struct {
	OnStartWatch   domain.Cmd
	OnBeforeChange domain.Cmd
	OnChange       domain.Cmd
	OnAfterChange  domain.Cmd
	OnFinishWatch  domain.Cmd
}

// namedHook은 설정에 적는 hook 이름과 그 hook의 명령이다.
type namedHook struct {
	name string
	cmd  domain.Cmd
}

// hookFunc는 감시 대상 하나의 hook을 실행한다.
//   - runStartHook은 onStartWatch를 시작하고 끝나기를 기다리지 않는다. onStartWatch가 도는 동안에는 자리가 모두 찬
//     것으로 보므로, interrupt: oldest일 때만 첫 실행이 onStartWatch를 취소한다.
//   - runChangeHook은 RunPolicy에 따라 자리가 나고 취소한 실행이 끝날 때까지 기다린 뒤 event의 실행을 시작하고,
//     그 실행이 끝나기를 기다리지 않는다. 실행 하나는 onBeforeChange, onChange, onAfterChange를 차례로 돌린다.
//   - cancel은 돌고 있는 모든 실행을 취소하고, 기다리던 runChangeHook을 포함해 새 실행을 시작하지 않게 한다.
//   - runFinishHook은 cancel한 뒤 모든 실행이 끝나기를 기다려 onFinishWatch를 실행한다.
type hookFunc struct {
	runStartHook  func()
	runChangeHook func(event domain.Event)
	runFinishHook func()
	cancel        func()
}

// hookRun은 돌고 있는 실행 하나다. start이면 onStartWatch이고, 아니면 path를 처리하는 변경 실행이다.
type hookRun struct {
	ctx   *process.WatcherContext
	path  domain.Path
	start bool
}

type hookRunner struct {
	policy domain.RunPolicy
	alias  string
	target domain.Path
	logger helper.Logger
	base   *process.WatcherContext

	mu sync.Mutex
	// ended는 실행이 끝나거나 cancel을 부를 때 기다리는 쪽을 깨운다.
	ended *sync.Cond
	// runs는 돌고 있는 실행이고, 시작한 순서대로 둔다.
	runs    []*hookRun
	stopped bool
}

// newHookFunc는 별칭이 alias인 감시 대상 target에서 hook 명령 info를 policy와 mode에 따라 실행하는 hookFunc를 만든다.
func newHookFunc(policy domain.RunPolicy, alias string, target domain.Path, info hookInfo, logger helper.Logger, mode process.CommandMode) hookFunc {
	r := &hookRunner{policy: policy, alias: alias, target: target, logger: logger, base: process.NewWatcherContext(alias, mode)}
	r.ended = sync.NewCond(&r.mu)
	startCommands := []namedHook{{"onStartWatch", info.OnStartWatch}}
	changeCommands := []namedHook{{"onBeforeChange", info.OnBeforeChange}, {"onChange", info.OnChange}, {"onAfterChange", info.OnAfterChange}}
	return hookFunc{
		runStartHook: func() {
			r.mu.Lock()
			defer r.mu.Unlock()
			if !r.stopped {
				r.start(&hookRun{start: true}, startCommands, nil)
			}
		},
		runChangeHook: func(event domain.Event) { r.admit(event, changeCommands) },
		cancel:        r.cancel,
		runFinishHook: func() {
			r.cancel()
			r.mu.Lock()
			for len(r.runs) > 0 {
				r.ended.Wait()
			}
			r.mu.Unlock()
			finish := r.base.NewRun()
			defer r.base.Cancel()
			defer finish.Cancel()
			_ = r.runCommand(finish, namedHook{"onFinishWatch", info.OnFinishWatch}, nil)
		},
	}
}

// admit은 event의 실행을 시작할 수 있을 때까지 기다린 뒤 시작한다. 그동안 cancel을 부르면 시작하지 않는다.
func (r *hookRunner) admit(event domain.Event, commands []namedHook) {
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
func (r *hookRunner) toCancel(path domain.Path) []*hookRun {
	switch r.policy.Interrupt {
	case domain.InterruptSamePath:
		var same []*hookRun
		for _, run := range r.runs {
			if !run.start && run.path == path {
				same = append(same, run)
			}
		}
		return same
	case domain.InterruptOldest:
		if r.full() {
			return []*hookRun{r.runs[0]}
		}
	}
	return nil
}

// start는 run으로 commands를 차례로 실행하기 시작한다. r.mu를 잡고 부른다.
func (r *hookRunner) start(run *hookRun, commands []namedHook, event *domain.Event) {
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

func (r *hookRunner) runCommand(ctx *process.WatcherContext, hook namedHook, event *domain.Event) error {
	err := hook.cmd.Run(ctx.WithHook(hook.name), r.target, event)
	// 새 변경이나 감시 종료로 실행을 취소하면 명령도 오류로 끝나는데, 이는 실패가 아니므로 기록하지 않는다.
	if err != nil && ctx.Context().Err() == nil {
		r.logger.Error(fmt.Sprintf("%s hook command failed, skipping remaining commands: %v", r.alias, err))
	}
	return err
}
