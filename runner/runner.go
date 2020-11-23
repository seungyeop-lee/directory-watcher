package runner

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"syscall"
	"time"

	"github.com/seungyeop-lee/directory-watcher/helper"

	"github.com/fsnotify/fsnotify"
)

type runner struct {
	commandSet CommandSet
	logger     logger

	exitCh         chan bool
	endCmdFinished chan bool
}

func NewRunner(commandSet CommandSet, logger logger) *runner {
	return &runner{
		commandSet:     commandSet,
		logger:         logger,
		exitCh:         make(chan bool),
		endCmdFinished: make(chan bool),
	}
}

func (r runner) Do() {
	r.initRun()

	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		r.logger.Error(err.Error())
		return
	}
	defer watcher.Close()

	r.addDir(watcher)

	event := make(chan Event)

	go r.run(event)

	for {
		select {
		case ev := <-watcher.Events:
			if ev.Op&fsnotify.Create == fsnotify.Create {
				if helper.IsExist(ev.Name) && helper.IsDir(ev.Name) && !r.commandSet.ExcludeDir.Equal(Path(ev.Name)) {
					r.logger.Info(fmt.Sprintf("%s has created", ev.Name))
					watcher.Add(ev.Name)
				}
			}
			if ev.Op&fsnotify.Create == fsnotify.Create || ev.Op&fsnotify.Write == fsnotify.Write || ev.Op&fsnotify.Remove == fsnotify.Remove {
				r.logger.Info(fmt.Sprintf("%s has changed", ev.Name))
				event <- NewEventByFsnotify(ev)
			}
		case err := <-watcher.Errors:
			if v, ok := err.(*os.SyscallError); ok {
				if v.Err == syscall.EINTR {
					continue
				}
				r.logger.Error(fmt.Sprint("watcher.Error: SyscallError:", v))
			}
			r.logger.Error(fmt.Sprint("watcher.Error:", err))
		}
	}
}

func (r runner) addDir(watcher *fsnotify.Watcher) {
	err := filepath.Walk(r.commandSet.Path.String(), func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() {
			return nil
		}
		if r.commandSet.ExcludeDir.Equal(Path(path)) {
			return nil
		}

		r.logger.Info(fmt.Sprint("add path:", path))
		return watcher.Add(path)
	})

	if err != nil {
		panic(err)
	}
}

func (r runner) initRun() {
	r.logger.Info(fmt.Sprint("initCmd start:", r.commandSet.InitCmd))
	if err := r.commandSet.InitCmd.Run(r.commandSet.Path); err != nil {
		r.logger.Error(err.Error())
	}
	r.logger.Info(fmt.Sprint("initCmd finished:", r.commandSet.InitCmd))
}

func (r runner) run(ev chan Event) {
	var threshold <-chan time.Time
	for {
		select {
		case <-ev:
			threshold = helper.CreateThreshold()
		case <-threshold:
			r.startBeforeCmd()
			r.startCommand()
			r.startAfterCmd()
		case <-r.exitCh:
			r.stopRun()
			r.endCmdFinished <- true
		}
	}
}

func (r runner) startBeforeCmd() {
	r.logger.Info(fmt.Sprint("global before cmd start:", r.commandSet.Cmd))
	if err := r.commandSet.GlobalBeforeCmd.Run(r.commandSet.Path); err != nil {
		r.logger.Error(err.Error())
	}
	r.logger.Info(fmt.Sprint("global before cmd finished:", r.commandSet.Cmd))
}

func (r runner) startCommand() {
	r.logger.Info(fmt.Sprint("cmd start:", r.commandSet.Cmd))
	if err := r.commandSet.Cmd.Run(r.commandSet.Path); err != nil {
		r.logger.Error(err.Error())
	}
	r.logger.Info(fmt.Sprint("cmd finished:", r.commandSet.Cmd))
}

func (r runner) startAfterCmd() {
	r.logger.Info(fmt.Sprint("global after cmd start:", r.commandSet.Cmd))
	if err := r.commandSet.GlobalAfterCmd.Run(r.commandSet.Path); err != nil {
		r.logger.Error(err.Error())
	}
	r.logger.Info(fmt.Sprint("global after cmd finished:", r.commandSet.Cmd))
}

func (r runner) stopRun() {
	r.logger.Info(fmt.Sprint("endCmd start:", r.commandSet.EndCmd))
	if err := r.commandSet.EndCmd.Run(r.commandSet.Path); err != nil {
		r.logger.Error(err.Error())
	}
	r.logger.Info(fmt.Sprint("endCmd finished:", r.commandSet.EndCmd))
}

func (r runner) Stop(wg *sync.WaitGroup) {
	defer wg.Done()

	r.exitCh <- true
	<-r.endCmdFinished
}
