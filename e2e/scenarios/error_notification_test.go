package scenarios

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/seungyeop-lee/directory-watcher/v3/e2e/harness"
)

func TestEventLossNotificationCLI(t *testing.T) {
	t.Parallel()
	for _, mode := range []string{"success", "absent", "missing-command", "failure", "timeout", "success-tree"} {
		t.Run(mode, func(t *testing.T) {
			t.Parallel()
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
      watch:
        exclude: ['*.probe']
        fileEvent: C
      hookBatch:
        waitMillisecond: 0
        deliver: each
      queue:
        size: 64
  - path: '%s'
    lifeCycle:
      onStartWatch:
        - %s
      onChange:
        - %s
      onFinishWatch:
        - %s
    option:
      watch:
        fileEvent: C
      hookBatch:
        waitMillisecond: 0
        deliver: each
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
			assertLossExit(t, s, "dispatch queue full (capacity 64): "+fsnotify.ErrEventOverflow.Error())
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
				assertErrorContext(t, contexts[0], s.Watch, "dispatch queue full (capacity 64): "+fsnotify.ErrEventOverflow.Error(), expectedDir)
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
			t.Logf("original fatal cause preserved: dispatch queue full (capacity 64): %s", fsnotify.ErrEventOverflow)
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
	t.Parallel()
	for _, deliver := range []string{"last", "each"} {
		t.Run(fmt.Sprintf("deliver=%s", deliver), func(t *testing.T) {
			t.Parallel()
			// each는 기다리지 않고 받은 이벤트마다 실행하게 하고, last는 기다린 뒤 마지막 경로 하나만 실행하게 한다.
			each, wait := deliver == "each", 50
			if each {
				wait = 0
			}
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
      watch:
        exclude: ['*.probe']
        fileEvent: C
      hookBatch:
        waitMillisecond: %d
        deliver: %s
`, s.Hook("--error-context", "--line=NOTIFY"), s.Watch, s.Hook("--line=CHANGE:{{.Path}}"), s.Hook("--line=FINISH"), wait, deliver))
			var paths []string
			for n := range 20 {
				path := filepath.Join(s.Watch, fmt.Sprintf("normal-%02d.txt", n))
				paths = append(paths, path)
				s.Write(path, nil)
			}
			deadline := time.Now().Add(10 * time.Second)
			s.AwaitChangesComplete("CHANGE:", paths, fsnotify.Create, each, time.Until(deadline))
			last := filepath.Join(s.Watch, "last.txt")
			s.Write(last, nil)
			s.AwaitLine("isolated normal/merged event completes", time.Until(deadline), "CHANGE:"+last)
			s.Stop()
			if got := errorContexts(t, s.Records); len(got) != 0 {
				t.Fatalf("normal/merged events invoked onError: %v", got)
			}
			if harness.CountRecord(s.Lines(), "FINISH") != 1 {
				t.Fatal("normal finish hook missing")
			}
		})
	}
}

// injectMarker는 주입 tag watcher가 감시 대상 폴더에서 읽는 표지 파일이다. 이름과 내용("loss", "nonloss")은
// internal/app/service/runner_watch_target_tree_inject.go와 같아야 한다.
const injectMarker = "dw-e2e-error.inject"

// TestInjectedLossNotificationCLI는 주입 tag watcher의 watcher 오류 채널에 오류를 보내, 유실 오류는 onError 알림 한
// 번과 종료 코드 1로, 유실이 아닌 치명적 오류는 알림 없이 종료 코드 1로 이어지는지 확인한다. 실제 OS overflow를
// 관찰한 증거는 아니다.
func TestInjectedLossNotificationCLI(t *testing.T) {
	t.Parallel()
	for _, deliver := range []string{"last", "each"} {
		for _, interrupt := range []string{"none", "oldest"} {
			t.Run(fmt.Sprintf("deliver=%s/interrupt=%s", deliver, interrupt), func(t *testing.T) {
				t.Parallel()
				runInjectedNotification(t, deliver, interrupt, "loss")
			})
		}
	}
	t.Run("non-loss-fatal-does-not-notify", func(t *testing.T) {
		t.Parallel()
		runInjectedNotification(t, "each", "none", "nonloss")
	})
}

// runInjectedNotification은 시작 hook과 변경 hook의 프로세스 트리가 돌고 파일이 계속 생기는 동안 marker 내용의
// 오류를 주입하고, 종료 코드, 최초 원인, 알림, 종료 hook, 남은 프로세스를 확인한다.
func runInjectedNotification(t *testing.T, deliver, interrupt, marker string) {
	t.Helper()
	// each는 기다리지 않고 받은 이벤트마다 실행하게 하고, last는 기다린 뒤 마지막 경로 하나만 실행하게 한다.
	wait := 30
	if deliver == "each" {
		wait = 0
	}
	s := harness.NewScenario(t)
	s.Launch(harness.InjectWatcherBin(), fmt.Sprintf(`global:
  lifeCycle:
    onError:
      - dir: %q
        cmd: %s
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
      watch:
        fileEvent: C
        exclude: ['*.probe', '*.swp', '*.inject']
      hookBatch:
        waitMillisecond: %d
        deliver: %s
      run:
        interrupt: %s
`, s.Work, s.Hook("--error-context", "--line=NOTIFY"), s.Hook("--line=GLOBAL:FINISH"),
		s.Watch, s.Hook("--depth=2"), s.Hook("--depth=2"), s.Hook("--line=TARGET:FINISH"), wait, deliver, interrupt))
	s.AssertRunning("startup process", s.AwaitReadyGenerations("startup process tree ready", 1))

	var produced atomic.Int64
	var paused atomic.Bool
	producerDone := make(chan error, 1)
	go func() {
		for n := 0; ; {
			select {
			case <-s.Done():
				producerDone <- nil
				return
			default:
			}
			if !paused.Load() {
				if err := os.WriteFile(filepath.Join(s.Watch, fmt.Sprintf("inflow-%04d.txt", n)), nil, 0644); err != nil {
					producerDone <- err
					return
				}
				n++
				produced.Add(1)
			}
			time.Sleep(time.Millisecond)
		}
	}()
	s.Await("thirty files created during the slow hooks", 5*time.Second, func() bool { return produced.Load() >= 30 })
	// interrupt=oldest에서는 생성을 멈춘 뒤에도 받아 둔 이벤트가 돌고 있는 트리를 취소하므로, 프로세스 기록이 200ms 동안
	// 바뀌지 않고 취소되지 않은 트리가 돌 때 오류를 주입한다. 그러면 치명적 오류로 멈출 때 다 만들어진 트리만 취소된다.
	paused.Store(true)
	var running []harness.ProcessRecord
	recordCount, changed := -1, time.Now()
	s.Await("a hook process tree running uncanceled with records unchanged for 200ms", 10*time.Second, func() bool {
		records := s.ProcessRecords()
		if len(records) != recordCount {
			recordCount, changed = len(records), time.Now()
		}
		running = runningTree(s, records)
		return len(running) > 0 && time.Since(changed) >= 200*time.Millisecond
	})
	// 표지 파일은 내용을 다 쓴 뒤 이름을 바꿔 만들어, watcher가 쓰는 중인 내용을 읽지 않게 한다.
	pending := filepath.Join(s.Watch, "dw-e2e-error.swp")
	s.Write(pending, []byte(marker))
	if err := os.Rename(pending, filepath.Join(s.Watch, injectMarker)); err != nil {
		t.Fatal(err)
	}
	atNotification := produced.Load()
	if marker == "loss" {
		s.Await("notification recorded", 10*time.Second, func() bool { return len(errorContexts(t, s.Records)) > 0 })
		// 알림 뒤에도 이벤트가 들어오게 해, 새 일반 hook이 시작되지 않는지 확인한다.
		paused.Store(false)
	}
	s.Await("injected fatal error exits the CLI", 10*time.Second, s.Exited)
	if err := <-producerDone; err != nil {
		t.Fatal(err)
	}

	cause := "injected 'cause' {{.Path}} $HOME: " + fsnotify.ErrEventOverflow.Error()
	if marker == "nonloss" {
		cause = "injected non-loss failure"
	}
	assertLossExit(t, s, cause)
	if strings.Contains(s.Log(), "later injected error") {
		t.Errorf("second injected error replaced or joined the first: %s", s.Log())
	}
	contexts := errorContexts(t, s.Records)
	if marker == "nonloss" {
		if len(contexts) != 0 {
			t.Fatalf("non-loss fatal error ran onError: %v", contexts)
		}
	} else {
		if len(contexts) != 1 {
			t.Fatalf("notification count = %d, want 1", len(contexts))
		}
		assertErrorContext(t, contexts[0], s.Watch, cause, s.Work)
	}
	lines := s.Lines()
	notified := false
	for _, line := range lines {
		if strings.HasPrefix(line, "ERROR|") {
			notified = true
			continue
		}
		// 프로세스 트리의 루트 부모는 generation이 자기 PID인 start를 남긴다.
		fields := strings.Split(line, "|")
		if notified && len(fields) == 4 && fields[0] == fields[1] && fields[3] == "start" {
			t.Errorf("new normal hook started after the notification: %s", line)
		}
	}
	for _, finish := range []string{"TARGET:FINISH", "GLOBAL:FINISH"} {
		if harness.CountRecord(lines, finish) != 1 {
			t.Errorf("finish hook %s count != 1: %v", finish, lines)
		}
	}
	records := s.ProcessRecords()
	s.AwaitProcessesGone("all hook descendants gone", records)
	t.Logf("INJECTED watcher error, not OS overflow evidence: deliver=%s interrupt=%s marker=%s notifications=%d exit=1 created-before-injection=%d created-after-notification=%d running-tree-at-injection=%s", deliver, interrupt, marker, len(contexts), atNotification, produced.Load()-atNotification, running[0].Generation)
}

// runningTree는 records에서 준비를 마쳤고 종료 신호를 받지 않았으며 루트 부모가 살아 있는 프로세스 트리 하나의 기록을
// 돌려준다. 그런 트리가 없으면 nil이다.
func runningTree(s *harness.Scenario, records []harness.ProcessRecord) []harness.ProcessRecord {
	byGeneration := map[string][]harness.ProcessRecord{}
	var order []string
	for _, r := range records {
		if _, seen := byGeneration[r.Generation]; !seen {
			order = append(order, r.Generation)
		}
		byGeneration[r.Generation] = append(byGeneration[r.Generation], r)
	}
	for _, generation := range order {
		ready, signaled, rootPID := false, false, 0
		for _, r := range byGeneration[generation] {
			if strconv.Itoa(r.PID) != r.Generation {
				continue
			}
			rootPID = r.PID
			ready = ready || r.State == "ready"
			signaled = signaled || r.State == "signal"
		}
		if ready && !signaled && s.Alive(rootPID) {
			return byGeneration[generation]
		}
	}
	return nil
}
