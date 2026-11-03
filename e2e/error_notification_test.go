package e2e

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/fsnotify/fsnotify"
)

func TestEventLossNotificationCLI(t *testing.T) {
	for _, mode := range []string{"success", "absent", "missing-command", "failure", "timeout", "success-tree"} {
		t.Run(mode, func(t *testing.T) {
			s := newScenario(t)
			notifyDir := filepath.Join(s.work, "notification-workdir")
			treeMode := mode == "failure" || mode == "timeout" || mode == "success-tree"
			var guard *scenario
			if treeMode {
				guard = newScenario(t)
				guard.startProcess(exec.Command(hookrecBin, "--file="+guard.records, "--depth=2"))
				guard.assertRunning("protected process", guard.awaitReadyGenerations("protected parent, child and grandchild ready", 1))
				guard.snapshotProcesses("protected-before")
			}
			other := filepath.Join(s.work, "other")
			for _, dir := range []string{notifyDir, other} {
				if err := os.Mkdir(dir, 0755); err != nil {
					t.Fatal(err)
				}
			}
			notification := notificationConfig(s, notifyDir, mode)
			recordsNotification := mode != "absent" && mode != "missing-command"
			s.launch(watcherBin, fmt.Sprintf(`global:
  lifeCycle:
%s    onBeforeChange:
      - %s
    onFinishWatch:
      - %s
watchTargets:
  - path: '%s'
    lifeCycle:
      onStartWatch:
        - %s
      onChange:
        - %s
      onFinishWatch:
        - %s
    option:
      noWait: true
      watchEvent: C
      excludeSuffix: [.probe]
  - path: '%s'
    lifeCycle:
      onStartWatch:
        - %s
      onChange:
        - %s
      onFinishWatch:
        - %s
    option:
      noWait: true
      watchEvent: C
`, notification, s.hook("--line=BEFORE"), s.hook("--line=GLOBAL:FINISH"),
				s.watch, s.hook("--delay=30s", "--line=START"), s.hook("--line=CHANGE"), s.hook("--line=TARGET:FINISH"),
				other, s.hook("--line=OTHER:READY"), s.hook("--line=OTHER:CHANGE"), s.hook("--line=OTHER:FINISH")))
			s.await("startup hooks ready at default ERROR level", 10*time.Second, func() bool {
				lines := s.lines()
				return delayedHookBegun(lines, "START") && countRecord(lines, "OTHER:READY") > 0
			})
			started := time.Now()
			producerDone := make(chan error, 1)
			go func() {
				for n := range 1000 {
					select {
					case <-s.done:
						producerDone <- nil
						return
					default:
					}
					if err := os.WriteFile(filepath.Join(s.watch, fmt.Sprintf("load-%04d.txt", n)), nil, 0644); err != nil {
						producerDone <- err
						return
					}
					time.Sleep(time.Millisecond)
				}
				producerDone <- nil
			}()
			if recordsNotification {
				s.await("one notification starts despite normal-hook cancellation", 10*time.Second, func() bool {
					return len(errorContexts(t, s.records)) > 0
				})
				if treeMode {
					s.assertRunning("notification process", s.awaitReadyGenerations("notification parent, child and grandchild ready", 1))
					s.snapshotProcesses("notification-before")
					if mode != "timeout" {
						s.write(filepath.Join(s.work, "release-notification"), nil)
					}
				}
				for n := range 20 {
					s.write(filepath.Join(other, fmt.Sprintf("after-fatal-%02d.txt", n)), nil)
				}
			}
			s.await("fatal CLI exit including bounded notification cleanup", 28*time.Second, s.exited)
			if err := <-producerDone; err != nil {
				t.Fatal(err)
			}
			assertLossExit(s, "dispatch queue full (capacity 256): "+fsnotify.ErrEventOverflow.Error())
			if !strings.Contains(s.log.String(), "possible event loss") {
				t.Fatalf("default ERROR diagnostics omitted possible loss: %s", s.log.String())
			}
			if (mode == "missing-command" || mode == "failure" || mode == "timeout") && !strings.Contains(s.log.String(), "onError notification failed") {
				t.Fatalf("notification failure diagnostic missing for %s: %s", mode, s.log.String())
			}
			if mode == "failure" && !strings.Contains(s.log.String(), "exit status 7") {
				t.Errorf("parent exit 7 was not preserved: %s", s.log.String())
			}
			if mode == "success-tree" && strings.Contains(s.log.String(), "onError notification failed") {
				t.Errorf("successful parent reported notification failure: %s", s.log.String())
			}
			contexts := errorContexts(t, s.records)
			if !recordsNotification {
				if len(contexts) != 0 {
					t.Fatalf("notification recorded without a runnable onError: %v", contexts)
				}
			} else {
				if len(contexts) != 1 {
					t.Fatalf("notification count = %d, want 1", len(contexts))
				}
				expectedDir := notifyDir
				if mode == "success" || mode == "failure" {
					expectedDir = s.work
				}
				assertErrorContext(t, contexts[0], s.watch, "dispatch queue full (capacity 256): "+fsnotify.ErrEventOverflow.Error(), expectedDir)
			}
			lines := s.lines()
			for _, forbidden := range []string{"CHANGE", "OTHER:CHANGE", "BEFORE", "START"} {
				if countRecord(lines, forbidden) != 0 {
					t.Errorf("normal hook approved or completed after fatal: %s\n%v", forbidden, lines)
				}
			}
			for _, finish := range []string{"TARGET:FINISH", "OTHER:FINISH", "GLOBAL:FINISH"} {
				if countRecord(lines, finish) != 1 {
					t.Errorf("finish hook %s count != 1: %v", finish, lines)
				}
			}
			if mode == "timeout" {
				if elapsed := time.Since(started); elapsed < 5*time.Second || elapsed > 27*time.Second {
					t.Errorf("notification timeout outside 5s + 20s cleanup bound: %s", elapsed)
				}
				if countRecord(lines, "NOTIFY") != 0 {
					t.Error("timed-out notification completed instead of being canceled")
				}
			} else if mode == "success" && countRecord(lines, "NOTIFY") != 1 {
				t.Errorf("notification did not finish: %v", lines)
			}
			s.awaitDelayedHooksGone()
			tree := s.processRecords()
			if treeMode {
				s.snapshotProcesses("notification-after")
				if !s.processesGone(tree) {
					t.Error("notification descendants alive after CLI cleanup completed")
				}
				for _, record := range guard.processRecords() {
					if record.state == "start" && !guard.procs.alive(record.pid) {
						t.Errorf("notification cleanup killed protected process: %+v", record)
					}
				}
				guard.snapshotProcesses("protected-after")
			}
			t.Logf("ACTUAL application dispatch saturation, not OS overflow: mode=%s duration=%s exit=1 notifications=%d normal-hook-PIDs-alive=0", mode, time.Since(started), len(contexts))
			t.Logf("original fatal cause preserved: dispatch queue full (capacity 256): %s", fsnotify.ErrEventOverflow)
			s.awaitProcessesGone("notification descendants disappear even when parent fails", tree)
		})
	}
}

func notificationConfig(s *scenario, dir, mode string) string {
	if mode == "absent" {
		return ""
	}
	if mode == "missing-command" {
		return fmt.Sprintf("    onError:\n      - '%s'\n", filepath.Join(s.work, "missing-notification-executable"))
	}
	arguments := []string{"--error-context", "--line=NOTIFY"}
	if mode == "failure" || mode == "success-tree" {
		arguments = append(arguments, "--depth=2", "--release-file="+filepath.Join(s.work, "release-notification"))
		if mode == "failure" {
			arguments = append(arguments, "--exit-code=7")
		}
	}
	if mode == "timeout" {
		arguments = append(arguments, "--depth=2")
	}
	command := s.hook(arguments...)
	switch mode {
	case "success", "failure":
		return fmt.Sprintf("    onError:\n      - %s\n", command)
	case "success-tree":
		return fmt.Sprintf("    onError:\n      - dir: %q\n        cmd:\n          - %s\n", dir, command)
	default:
		return fmt.Sprintf("    onError:\n      - dir: %q\n        cmd: %s\n", dir, command)
	}
}

func errorContexts(t *testing.T, file string) []map[string]string {
	t.Helper()
	var contexts []map[string]string
	for _, line := range readLinesLenient(file) {
		if strings.HasPrefix(line, "ERROR|") {
			var context map[string]string
			if err := json.Unmarshal([]byte(strings.TrimPrefix(line, "ERROR|")), &context); err != nil {
				t.Fatalf("notification JSON: %v: %s", err, line)
			}
			contexts = append(contexts, context)
		}
	}
	return contexts
}

func assertErrorContext(t *testing.T, context map[string]string, target, cause, workdir string) {
	t.Helper()
	want := map[string]string{
		"DW_ERROR_TARGET":        target,
		"DW_ERROR_KIND":          "event_loss",
		"DW_ERROR_CAUSE":         cause,
		"DW_ERROR_MESSAGE":       fmt.Sprintf("watch target %q: %s", target, cause),
		"DW_ERROR_POSSIBLE_LOSS": "true",
		"workdir":                filepath.Clean(workdir),
	}
	if len(context) != len(want) {
		t.Errorf("unexpected error context fields: %v", context)
	}
	for key, value := range want {
		if context[key] != value {
			t.Errorf("%s = %q, want %q", key, context[key], value)
		}
	}
}

func assertLossExit(s *scenario, cause string) {
	s.t.Helper()
	var exit *exec.ExitError
	if !errors.As(s.exit, &exit) || exit.ExitCode() != 1 {
		s.t.Fatalf("fatal exit = %v, want status 1\n%s", s.exit, s.log.String())
	}
	log := s.log.String()
	if !strings.Contains(log, fmt.Sprintf("watch target %q: %s", s.watch, cause)) {
		s.t.Fatalf("original fatal target/cause missing at default ERROR level: %s", log)
	}
	if strings.Contains(log, "event:") {
		s.t.Fatalf("default ERROR test unexpectedly enabled DEBUG logging: %s", log)
	}
}

func TestNormalChangesDoNotNotify(t *testing.T) {
	for _, noWait := range []bool{false, true} {
		t.Run(fmt.Sprintf("noWait=%t", noWait), func(t *testing.T) {
			s := newScenario(t)
			s.start(fmt.Sprintf(`global:
  lifeCycle:
    onError:
      - %s
watchTargets:
  - path: '%s'
    lifeCycle:
      onChange:
        - %s
      onFinishWatch:
        - %s
    option:
      noWait: %t
      waitMillisecond: 50
      watchEvent: C
      excludeSuffix: [.probe]
`, s.hook("--error-context", "--line=NOTIFY"), s.watch, s.hook("--line=CHANGE:{{.Path}}"), s.hook("--line=FINISH"), noWait))
			var paths []string
			for n := range 20 {
				path := filepath.Join(s.watch, fmt.Sprintf("normal-%02d.txt", n))
				paths = append(paths, path)
				s.write(path, nil)
			}
			deadline := time.Now().Add(10 * time.Second)
			s.awaitChangesComplete("CHANGE:", paths, fsnotify.Create, noWait, time.Until(deadline))
			last := filepath.Join(s.watch, "last.txt")
			s.write(last, nil)
			s.awaitLine("isolated normal/debounced event completes", time.Until(deadline), "CHANGE:"+last)
			s.stop()
			if got := errorContexts(t, s.records); len(got) != 0 {
				t.Fatalf("normal/debounced events invoked onError: %v", got)
			}
			if countRecord(s.lines(), "FINISH") != 1 {
				t.Fatal("normal finish hook missing")
			}
		})
	}
}
