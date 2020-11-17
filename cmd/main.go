package main

import (
	"directory-watcher/runner"
	"fmt"
)

func main() {
	fmt.Println("directory-watcher run")
	done := make(chan bool)
	runner.NewRunners(runner.CommandSets{
		{
			InitCmd: "echo start dir1",
			Cmd:     "echo mod dir1",
			Path:    "../dir1",
			ExcludeDir: runner.Paths{
				"../dir1/tmp",
			},
		},
		{
			InitCmd: "echo start dir2",
			Cmd:     "echo mod dir2",
			Path:    "../dir2",
			ExcludeDir: runner.Paths{
				"../dir2/tmp",
			},
		},
	}).Do()
	<-done
}
