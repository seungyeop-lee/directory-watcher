package scenarios

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
	"github.com/seungyeop-lee/directory-watcher/v2/e2e/harness"
)

func TestEventLossNotificationCLI(t *testing.T) {
	for _, mode := range []string{"success", "absent", "missing-command", "failure", "timeout", "success-tree"} {
		t.Run(mode, func(t *testing.T) {
			s := harness.NewScenario(t)
			notifyDir := filepath.Join(s.Work, "notification-workdir")
			treeMode := mode == "failure" || mode == "timeout" || mode == "success-tree"
			var guard *harness.Scenario
			if treeMode {
				guard = harness.NewScenario(t)
				guard.StartProcess(exec.Command(harness.HookrecBin(), "--file="+guard.Records, "--depth=2"))
				guard.AssertRunning("protected process", guard.AwaitReadyGenerations("protected parent, child and grandchild ready", 1))
				guard.SnapshotProcesses("protected-before")
			}
			other := filepath.Join(s.Work, "other")
			for _, dir := range []string{notifyDir, other} {
				if err := os.Mkdir(dir, 0755); err != nil {
					t.Fatal(err)
				}
			}
			notification := notificationConfig(s, notifyDir, mode)
			recordsNotification := mode != "absent" && mode != "missing-command"
			s.Launch(harness.WatcherBin(), fmt.Sprintf(`global:
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
`, notification, s.Hook("--line=BEFORE"), s.Hook("--line=GLOBAL:FINISH"),
				s.Watch, s.Hook("--delay=30s", "--line=START"), s.Hook("--line=CHANGE"), s.Hook("--line=TARGET:FINISH"),
				other, s.Hook("--line=OTHER:READY"), s.Hook("--line=OTHER:CHANGE"), s.Hook("--line=OTHER:FINISH")))
			s.Await("startup hooks ready at default ERROR level", 10*time.Second, func() bool {
				lines := s.Lines()
				return harness.DelayedHookBegun(lines, "START") && harness.CountRecord(lines, "OTHER:READY") > 0
			})
			started := time.Now()
			producerDone := make(chan error, 1)
			go func() {
				for n := range 1000 {
					select {
					case <-s.Done():
						producerDone <- nil
						return
					default:
					}
					if err := os.WriteFile(filepath.Join(s.Watch, fmt.Sprintf("load-%04d.txt", n)), nil, 0644); err != nil {
						producerDone <- err
						return
					}
					time.Sleep(time.Millisecond)
				}
				producerDone <- nil
			}()
			if recordsNotification {
				s.Await("one notification starts despite normal-hook cancellation", 10*time.Second, func() bool {
					return len(errorContexts(t, s.Records)) > 0
				})
				if treeMode {
					s.AssertRunning("notification process", s.AwaitReadyGenerations("notification parent, child and grandchild ready", 1))
					s.SnapshotProcesses("notification-before")
					if mode != "timeout" {
						s.Write(filepath.Join(s.Work, "release-notification"), nil)
					}
				}
				for n := range 20 {
					s.Write(filepath.Join(other, fmt.Sprintf("after-fatal-%02d.txt", n)), nil)
				}
			}
			s.Await("fatal CLI exit including bounded notification cleanup", 28*time.Second, s.Exited)
			if err := <-producerDone; err != nil {
				t.Fatal(err)
			}
			assertLossExit(t, s, "dispatch queue full (capacity 256): "+fsnotify.ErrEventOverflow.Error())
			if !strings.Contains(s.Log(), "possible event loss") {
				t.Fatalf("default ERROR diagnostics omitted possible loss: %s", s.Log())
			}
			if (mode == "missing-command" || mode == "failure" || mode == "timeout") && !strings.Contains(s.Log(), "onError notification failed") {
				t.Fatalf("notification failure diagnostic missing for %s: %s", mode, s.Log())
			}
			if mode == "failure" && !strings.Contains(s.Log(), "exit status 7") {
				t.Errorf("parent exit 7 was not preserved: %s", s.Log())
			}
			if mode == "success-tree" && strings.Contains(s.Log(), "onError notification failed") {
				t.Errorf("successful parent reported notification failure: %s", s.Log())
			}
			contexts := errorContexts(t, s.Records)
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
					expectedDir = s.Work
				}
				assertErrorContext(t, contexts[0], s.Watch, "dispatch queue full (capacity 256): "+fsnotify.ErrEventOverflow.Error(), expectedDir)
			}
			lines := s.Lines()
			for _, forbidden := range []string{"CHANGE", "OTHER:CHANGE", "BEFORE", "START"} {
				if harness.CountRecord(lines, forbidden) != 0 {
					t.Errorf("normal hook approved or completed after fatal: %s\n%v", forbidden, lines)
				}
			}
			for _, finish := range []string{"TARGET:FINISH", "OTHER:FINISH", "GLOBAL:FINISH"} {
				if harness.CountRecord(lines, finish) != 1 {
					t.Errorf("finish hook %s count != 1: %v", finish, lines)
				}
			}
			if mode == "timeout" {
				if elapsed := time.Since(started); elapsed < 5*time.Second || elapsed > 27*time.Second {
					t.Errorf("notification timeout outside 5s + 20s cleanup bound: %s", elapsed)
				}
				if harness.CountRecord(lines, "NOTIFY") != 0 {
					t.Error("timed-out notification completed instead of being canceled")
				}
			} else if mode == "success" && harness.CountRecord(lines, "NOTIFY") != 1 {
				t.Errorf("notification did not finish: %v", lines)
			}
			s.AwaitDelayedHooksGone()
			tree := s.ProcessRecords()
			if treeMode {
				s.SnapshotProcesses("notification-after")
				if !s.ProcessesGone(tree) {
					t.Error("notification descendants alive after CLI cleanup completed")
				}
				for _, record := range guard.ProcessRecords() {
					if record.State == "start" && !guard.Alive(record.PID) {
						t.Errorf("notification cleanup killed protected process: %+v", record)
					}
				}
				guard.SnapshotProcesses("protected-after")
			}
			t.Logf("ACTUAL application dispatch saturation, not OS overflow: mode=%s duration=%s exit=1 notifications=%d normal-hook-PIDs-alive=0", mode, time.Since(started), len(contexts))
			t.Logf("original fatal cause preserved: dispatch queue full (capacity 256): %s", fsnotify.ErrEventOverflow)
			s.AwaitProcessesGone("notification descendants disappear even when parent fails", tree)
		})
	}
}

func notificationConfig(s *harness.Scenario, dir, mode string) string {
	if mode == "absent" {
		return ""
	}
	if mode == "missing-command" {
		return fmt.Sprintf("    onError:\n      - '\"%s\"'\n", filepath.Join(s.Work, "missing-notification-executable"))
	}
	arguments := []string{"--error-context", "--line=NOTIFY"}
	if mode == "failure" || mode == "success-tree" {
		arguments = append(arguments, "--depth=2", "--release-file="+filepath.Join(s.Work, "release-notification"))
		if mode == "failure" {
			arguments = append(arguments, "--exit-code=7")
		}
	}
	if mode == "timeout" {
		arguments = append(arguments, "--depth=2")
	}
	command := s.Hook(arguments...)
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
	for _, line := range harness.ReadLinesLenient(file) {
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

func assertLossExit(t *testing.T, s *harness.Scenario, cause string) {
	t.Helper()
	var exit *exec.ExitError
	if !errors.As(s.ExitErr(), &exit) || exit.ExitCode() != 1 {
		t.Fatalf("fatal exit = %v, want status 1\n%s", s.ExitErr(), s.Log())
	}
	log := s.Log()
	if !strings.Contains(log, fmt.Sprintf("watch target %q: %s", s.Watch, cause)) {
		t.Fatalf("original fatal target/cause missing at default ERROR level: %s", log)
	}
	if strings.Contains(log, "event:") {
		t.Fatalf("default ERROR test unexpectedly enabled DEBUG logging: %s", log)
	}
}

func TestNormalChangesDoNotNotify(t *testing.T) {
	for _, noWait := range []bool{false, true} {
		t.Run(fmt.Sprintf("noWait=%t", noWait), func(t *testing.T) {
			s := harness.NewScenario(t)
			s.Start(fmt.Sprintf(`global:
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
`, s.Hook("--error-context", "--line=NOTIFY"), s.Watch, s.Hook("--line=CHANGE:{{.Path}}"), s.Hook("--line=FINISH"), noWait))
			var paths []string
			for n := range 20 {
				path := filepath.Join(s.Watch, fmt.Sprintf("normal-%02d.txt", n))
				paths = append(paths, path)
				s.Write(path, nil)
			}
			deadline := time.Now().Add(10 * time.Second)
			s.AwaitChangesComplete("CHANGE:", paths, fsnotify.Create, noWait, time.Until(deadline))
			last := filepath.Join(s.Watch, "last.txt")
			s.Write(last, nil)
			s.AwaitLine("isolated normal/debounced event completes", time.Until(deadline), "CHANGE:"+last)
			s.Stop()
			if got := errorContexts(t, s.Records); len(got) != 0 {
				t.Fatalf("normal/debounced events invoked onError: %v", got)
			}
			if harness.CountRecord(s.Lines(), "FINISH") != 1 {
				t.Fatal("normal finish hook missing")
			}
		})
	}
}
