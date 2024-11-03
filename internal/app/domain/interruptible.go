package domain

import "github.com/seungyeop-lee/directory-watcher/v2/internal/helper"

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
}

type HookFuncBuilder func(path Path) *HookFunc

func (i Interruptible) BuildHookFuncBuilder(info HookInfo) HookFuncBuilder {
	if i {
		return i.interruptibleHookFuncBuilder(info)
	} else {
		return i.sequentialHookFuncBuilder(info)
	}
}

func (i Interruptible) interruptibleHookFuncBuilder(info HookInfo) HookFuncBuilder {
	hookCtx := helper.NewWatcherContext()
	return func(path Path) *HookFunc {
		return &HookFunc{
			RunStartHook: func() {
				go func() {
					if err := info.OnStartWatch.Run(hookCtx, path, nil); err != nil {
						return
					}
				}()
			},
			RunChangeHook: func(event Event) {
				hookCtx.CancelAndNew()
				hookCtx.SetWaitNumber(1)

				go func() {
					hookCtx.MarkStart()
					if err := info.OnBeforeChange.Run(hookCtx, path, &event); err != nil {
						return
					}
					if err := info.OnChange.Run(hookCtx, path, &event); err != nil {
						return
					}
					if err := info.OnAfterChange.Run(hookCtx, path, &event); err != nil {
						return
					}
				}()

				hookCtx.WaitStart()
			},
			RunFinishHook: func() {
				hookCtx.CancelAndNew()

				if err := info.OnFinishWatch.Run(hookCtx, path, nil); err != nil {
					return
				}
			},
		}
	}
}

func (i Interruptible) sequentialHookFuncBuilder(info HookInfo) HookFuncBuilder {
	hookCtx := helper.NewWatcherContext()
	return func(path Path) *HookFunc {
		return &HookFunc{
			RunStartHook: func() {
				if err := info.OnStartWatch.Run(hookCtx, path, nil); err != nil {
					return
				}
			},
			RunChangeHook: func(event Event) {
				if err := info.OnBeforeChange.Run(hookCtx, path, &event); err != nil {
					return
				}
				if err := info.OnChange.Run(hookCtx, path, &event); err != nil {
					return
				}
				if err := info.OnAfterChange.Run(hookCtx, path, &event); err != nil {
					return
				}
			},
			RunFinishHook: func() {
				hookCtx.CancelAndNew()

				if err := info.OnFinishWatch.Run(hookCtx, path, nil); err != nil {
					return
				}
			},
		}
	}
}
