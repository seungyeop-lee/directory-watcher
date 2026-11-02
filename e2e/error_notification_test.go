//go:build !windows

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
	for _, mode := range []string{"success", "absent", "missing-command", "failure", "timeout"} {
		t.Run(mode, func(t *testing.T) {
			s := newScenario(t)
			notifyDir := filepath.Join(s.work, "notification-workdir")
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
      - '%s --file=%s --line=BEFORE'
    onFinishWatch:
      - '%s --file=%s --line=GLOBAL:FINISH'
watchTargets:
  - path: '%s'
    lifeCycle:
      onStartWatch:
        - '%s --file=%s --delay=30s --line=START'
      onChange:
        - '%s --file=%s --line=CHANGE'
      onFinishWatch:
        - '%s --file=%s --line=TARGET:FINISH'
    option:
      noWait: true
      watchEvent: C
      excludeSuffix: [.probe]
  - path: '%s'
    lifeCycle:
      onStartWatch:
        - '%s --file=%s --line=OTHER:READY'
      onChange:
        - '%s --file=%s --line=OTHER:CHANGE'
      onFinishWatch:
        - '%s --file=%s --line=OTHER:FINISH'
    option:
      noWait: true
      watchEvent: C
`, notification, hookrecBin, s.records, hookrecBin, s.records, s.watch, hookrecBin, s.records, hookrecBin, s.records, hookrecBin, s.records, other, hookrecBin, s.records, hookrecBin, s.records, hookrecBin, s.records))
			s.await("startup hooks ready at default ERROR level", 10*time.Second, func() bool {
				lines := strings.Join(readLinesLenient(s.records), "\n")
				return strings.Contains(lines, "|START") && strings.Contains(lines, "OTHER:READY")
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
				for n := range 20 {
					s.write(filepath.Join(other, fmt.Sprintf("after-fatal-%02d.txt", n)), nil)
				}
			}
			s.await("fatal CLI exit including bounded notification cleanup", 28*time.Second, func() bool {
				select {
				case <-s.done:
					return true
				default:
					return false
				}
			})
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
			contexts := errorContexts(t, s.records)
			if !recordsNotification {
				if len(contexts) != 0 {
					t.Fatalf("notification recorded without a runnable onError: %v", contexts)
				}
			} else {
				if len(contexts) != 1 {
					t.Fatalf("notification count = %d, want 1", len(contexts))
				}
				assertErrorContext(t, contexts[0], s.watch, "dispatch queue full (capacity 256): "+fsnotify.ErrEventOverflow.Error(), notifyDir)
			}
			lines := readLinesLenient(s.records)
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
			assertLineProcessesGone(s)
			tree := processRecords(s.records)
			s.await("notification descendants disappear even when parent fails", 5*time.Second, func() bool { return recordsGone(tree) })
			if (mode == "failure" || mode == "timeout") && len(tree) == 0 {
				t.Fatal("notification process tree was not exercised")
			}
			t.Logf("ACTUAL application dispatch saturation, not OS overflow: mode=%s duration=%s exit=1 notifications=%d normal-hook-PIDs-alive=0", mode, time.Since(started), len(contexts))
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
	extra := ""
	if mode == "failure" {
		extra = " --depth=2 --exit-code=7"
	}
	if mode == "timeout" {
		extra = " --depth=2"
	}
	return fmt.Sprintf("    onError:\n      - dir: %q\n        cmd: '%s --file=%s --error-context --line=NOTIFY%s'\n", dir, hookrecBin, s.records, extra)
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
		"workdir":                workdir,
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
	if !strings.Contains(log, s.watch) || !strings.Contains(log, cause) {
		s.t.Fatalf("original fatal target/cause missing at default ERROR level: %s", log)
	}
	if strings.Contains(log, "event:") {
		s.t.Fatalf("default ERROR test unexpectedly enabled DEBUG logging: %s", log)
	}
}

func countRecord(lines []string, value string) int {
	count := 0
	for _, line := range lines {
		if line == value {
			count++
		}
	}
	return count
}

func TestNormalChangesDoNotNotify(t *testing.T) {
	for _, noWait := range []bool{false, true} {
		t.Run(fmt.Sprintf("noWait=%t", noWait), func(t *testing.T) {
			s := newScenario(t)
			s.start(fmt.Sprintf(`global:
  lifeCycle:
    onError:
      - '%s --file=%s --error-context --line=NOTIFY'
watchTargets:
  - path: '%s'
    lifeCycle:
      onChange:
        - '%s --file=%s --line=CHANGE:{{.Path}}'
      onFinishWatch:
        - '%s --file=%s --line=FINISH'
    option:
      noWait: %t
      waitMillisecond: 50
      watchEvent: C
      excludeSuffix: [.probe]
`, hookrecBin, s.records, s.watch, hookrecBin, s.records, hookrecBin, s.records, noWait))
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
			s.await("isolated normal/debounced event completes", time.Until(deadline), func() bool {
				return countRecord(readLinesLenient(s.records), "CHANGE:"+last) > 0
			})
			s.stop()
			if got := errorContexts(t, s.records); len(got) != 0 {
				t.Fatalf("normal/debounced events invoked onError: %v", got)
			}
			if countRecord(readLinesLenient(s.records), "FINISH") != 1 {
				t.Fatal("normal finish hook missing")
			}
		})
	}
}
