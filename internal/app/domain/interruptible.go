package domain

import (
	"fmt"
	"sync"

	"github.com/seungyeop-lee/directory-watcher/v3/internal/helper"
)

type Interruptible bool

type HookInfo struct {
	OnStartWatch   Cmd
	OnBeforeChange Cmd
	OnChange       Cmd
	OnAfterChange  Cmd
	OnFinishWatch  Cmd
}

type HookFunc struct {
	RunStartHook  func()
	RunChangeHook func(event Event)
	RunFinishHook func()
	Cancel        func()
}

type HookFuncBuilder func(path Path) *HookFunc

func (i Interruptible) BuildHookFuncBuilder(alias string, info HookInfo, logger helper.Logger) HookFuncBuilder {
	return func(path Path) *HookFunc {
		base := helper.NewWatcherContext(alias)
		startCommands := []Cmd{info.OnStartWatch}
		changeCommands := []Cmd{info.OnBeforeChange, info.OnChange, info.OnAfterChange}
		var mu sync.Mutex
		var active *helper.WatcherContext
		var done chan struct{}
		stopped := false
		runCommand := func(ctx *helper.WatcherContext, command Cmd, event *Event) error {
			err := command.Run(ctx, path, event)
			// 새 변경이나 감시 종료로 실행을 취소하면 명령도 오류로 끝나는데, 이는 실패가 아니므로 기록하지 않는다.
			if err != nil && ctx.Context().Err() == nil {
				logger.Error(fmt.Sprintf("%s hook command failed, skipping remaining commands: %v", alias, err))
			}
			return err
		}
		cancel := func() {
			mu.Lock()
			stopped = true
			if active != nil {
				active.Cancel()
			}
			mu.Unlock()
		}
		run := func(commands []Cmd, event *Event) {
			mu.Lock()
			if stopped {
				mu.Unlock()
				return
			}
			if active != nil {
				active.Cancel()
			}
			previous := done
			mu.Unlock()
			if previous != nil {
				<-previous
			}
			mu.Lock()
			if stopped {
				mu.Unlock()
				return
			}
			ctx := base.NewRun()
			completed := make(chan struct{})
			active, done = ctx, completed
			mu.Unlock()
			execute := func() {
				defer close(completed)
				defer ctx.Cancel()
				for _, command := range commands {
					if ctx.Context().Err() != nil {
						return
					}
					if err := runCommand(ctx, command, event); err != nil {
						return
					}
				}
			}
			if i {
				go execute()
			} else {
				execute()
			}
		}
		return &HookFunc{
			RunStartHook:  func() { run(startCommands, nil) },
			RunChangeHook: func(event Event) { run(changeCommands, &event) },
			Cancel:        cancel,
			RunFinishHook: func() {
				cancel()
				mu.Lock()
				completed := done
				mu.Unlock()
				if completed != nil {
					<-completed
				}
				finish := base.NewRun()
				defer base.Cancel()
				defer finish.Cancel()
				_ = runCommand(finish, info.OnFinishWatch, nil)
			},
		}
	}
}
