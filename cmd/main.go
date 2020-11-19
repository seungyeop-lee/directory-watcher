package main

import (
	"directory-watcher/runner"
	"fmt"
	"os"
	"os/signal"
	"sync"
	"syscall"
)

func main() {
	fmt.Println("directory-watcher run")

	done := make(chan bool)
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)

	r := runner.NewRunners(runner.CommandSets{
		{
			InitCmd: "echo start dir1",
			EndCmd:  "echo stop dir1",
			Cmd:     "echo mod dir1",
			Path:    "../dir1",
			ExcludeDir: runner.Paths{
				"../dir1/tmp",
			},
		},
		{
			InitCmd: "echo start dir2",
			EndCmd:  "echo stop dir2",
			Cmd:     "echo mod dir2",
			Path:    "../dir2",
			ExcludeDir: runner.Paths{
				"../dir2/tmp",
			},
		},
	})

	r.Do()

	go func() {
		<-sigs
		wg := sync.WaitGroup{}
		r.Stop(&wg)
		wg.Wait()
		done <- true
	}()

	<-done
}
