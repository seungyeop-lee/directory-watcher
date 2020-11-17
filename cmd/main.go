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
			Cmd:  "echo abcd",
			Path: "../dir",
			ExcludeDir: runner.Paths{
				"../dir/tmp",
			},
		},
	}).Do()
	<-done
}
