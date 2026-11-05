package main

import (
	"encoding/json"
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
	delay := flag.Duration("delay", 0, "delay before completing a line record")
	errorContext := flag.Bool("error-context", false, "record error notification environment and working directory")
	queueContext := flag.Bool("queue-context", false, "record queue warning environment")
	workdir := flag.Bool("workdir", false, "record working directory before the line")
	exitCode := flag.Int("exit-code", 0, "exit status after recording")
	releaseFile := flag.String("release-file", "", "exit parent when this file appears, leaving descendants running")
	label := flag.String("label", "", "text recorded with the process tree generation")
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
	if *workdir {
		dir, err := os.Getwd()
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		appendLine("WORKDIR|" + dir)
	}
	if *errorContext {
		context := environWithPrefix("DW_ERROR_")
		context["workdir"], err = os.Getwd()
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		data, err := json.Marshal(context)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		appendLine("ERROR|" + string(data))
	}
	if *queueContext {
		context := environWithPrefix("DW_QUEUE_")
		data, err := json.Marshal(context)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		appendLine("QUEUE|" + string(data))
	}
	if *depth < 0 {
		if *delay > 0 {
			signals := make(chan os.Signal, 1)
			signal.Notify(signals, os.Interrupt, syscall.SIGTERM)
			defer signal.Stop(signals)
			appendLine(fmt.Sprintf("BEGIN|%d|%s", os.Getpid(), *line))
			select {
			case <-signals:
				appendLine(fmt.Sprintf("CANCEL|%d|%s", os.Getpid(), *line))
				return
			case <-time.After(*delay):
			}
		}
		appendLine(*line)
		if *exitCode != 0 {
			_ = f.Close()
			os.Exit(*exitCode)
		}
		return
	}
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(signals)
	if *run == "" {
		*run = strconv.Itoa(os.Getpid())
	}
	record := func(state string) { appendLine(fmt.Sprintf("%s|%d|%d|%s", *run, os.Getpid(), *depth, state)) }
	if *label != "" {
		appendLine(fmt.Sprintf("LABEL|%s|%s", *run, *label))
	}
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
	if *releaseFile != "" {
		released := func() bool {
			for {
				if _, err := os.Stat(*releaseFile); err == nil {
					return true
				}
				select {
				case <-signals:
					return false
				case <-time.After(10 * time.Millisecond):
				}
			}
		}()
		if released {
			record("parent-exit")
			_ = f.Close()
			os.Exit(*exitCode)
		}
	} else {
		if *exitCode != 0 {
			_ = f.Close()
			os.Exit(*exitCode)
		}
		<-signals
	}
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

// environWithPrefix는 이름이 prefix로 시작하는 환경 변수를 모두 모은다. 키를 따로 나열하지 않는다: 목록에 없는
// 키는 빈 값으로 기록되어, watcher에 키가 늘었을 때 e2e 실패의 원인이 이 목록이라는 점이 드러나지 않는다.
func environWithPrefix(prefix string) map[string]string {
	values := make(map[string]string)
	for _, entry := range os.Environ() {
		if key, value, _ := strings.Cut(entry, "="); strings.HasPrefix(key, prefix) {
			values[key] = value
		}
	}
	return values
}
