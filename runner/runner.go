package runner

import (
	"log"
	"os"
	"path/filepath"
	"sync"
	"syscall"
	"time"

	"directory-watcher/helper"

	"github.com/fsnotify/fsnotify"
)

type runner struct {
	commandSet CommandSet

	exitCh         chan bool
	endCmdFinished chan bool
}

func NewRunner(commandSet CommandSet) *runner {
	return &runner{
		commandSet:     commandSet,
		exitCh:         make(chan bool),
		endCmdFinished: make(chan bool),
	}
}

func (r runner) Do() {
	r.initRun()

	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		log.Fatalln(err)
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
					log.Printf("%s has created", ev.Name)
					watcher.Add(ev.Name)
				}
			}
			if ev.Op&fsnotify.Create == fsnotify.Create || ev.Op&fsnotify.Write == fsnotify.Write || ev.Op&fsnotify.Remove == fsnotify.Remove {
				log.Printf("%s has changed", ev.Name)
				event <- NewEventByFsnotify(ev)
			}
		case err := <-watcher.Errors:
			if v, ok := err.(*os.SyscallError); ok {
				if v.Err == syscall.EINTR {
					continue
				}
				log.Fatal("watcher.Error: SyscallError:", v)
			}
			log.Fatal("watcher.Error:", err)
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

		log.Println("add path:", path)
		return watcher.Add(path)
	})

	if err != nil {
		panic(err)
	}
}

func (r runner) initRun() {
	if r.commandSet.InitCmd != "" {
		log.Println("InitCmd start")
		r.commandSet.InitCmd.Run(r.commandSet.Path)
		log.Println("InitCmd start")
	}
}

func (r runner) run(ev chan Event) {
	var threshold <-chan time.Time
	for {
		select {
		case <-ev:
			threshold = helper.CreateThreshold()
		case <-threshold:
			r.startCommand()
		case <-r.exitCh:
			r.stopRun()
			r.endCmdFinished <- true
		}
	}
}

func (r runner) startCommand() {
	r.commandSet.Cmd.Run(r.commandSet.Path)
}

func (r runner) stopRun() {
	if r.commandSet.EndCmd != "" {
		log.Println("EndCmd start")
		r.commandSet.EndCmd.Run(r.commandSet.Path)
		log.Println("EndCmd stop")
	}
}

func (r runner) Stop(wg *sync.WaitGroup) {
	defer wg.Done()

	r.exitCh <- true
	<-r.endCmdFinished
}
