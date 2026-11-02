package main

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"
)

func main() {
	file := flag.String("file", "", "record file")
	line := flag.String("line", "", "line to append")
	depth := flag.Int("depth", -1, "long-running process descendant depth")
	run := flag.String("run", "", "process generation identifier")
	flag.Parse()
	if *file == "" {
		fmt.Fprintln(os.Stderr, "hookrec: --file is required")
		os.Exit(2)
	}
	f, err := os.OpenFile(*file, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	defer f.Close()
	appendLine := func(text string) {
		if _, err := fmt.Fprintln(f, text); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	}
	if *depth < 0 {
		appendLine(*line)
		return
	}
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(signals)
	if *run == "" {
		*run = strconv.Itoa(os.Getpid())
	}
	record := func(state string) { appendLine(fmt.Sprintf("%s|%d|%d|%s", *run, os.Getpid(), *depth, state)) }
	record("start")
	var child *exec.Cmd
	if *depth > 0 {
		executable, err := os.Executable()
		if err != nil {
			panic(err)
		}
		child = exec.Command(executable, "--file="+*file, "--depth="+strconv.Itoa(*depth-1), "--run="+*run)
		child.Stdout, child.Stderr = os.Stdout, os.Stderr
		if err := child.Start(); err != nil {
			panic(err)
		}
		deadline := time.Now().Add(5 * time.Second)
		ready := fmt.Sprintf("%s|%d|%d|ready", *run, child.Process.Pid, *depth-1)
		for {
			data, err := os.ReadFile(*file)
			if err == nil && strings.Contains(string(data), ready+"\n") {
				break
			}
			if time.Now().After(deadline) {
				_ = child.Process.Kill()
				_ = child.Wait()
				panic("child readiness timeout")
			}
			time.Sleep(10 * time.Millisecond)
		}
	}
	record("ready")
	<-signals
	record("signal")
	time.Sleep(150 * time.Millisecond)
	if child != nil {
		if err := child.Wait(); err != nil {
			fmt.Fprintln(os.Stderr, "child exit:", err)
			os.Exit(1)
		}
	}
	record("complete")
}
