package scenarios

import (
	"fmt"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/seungyeop-lee/directory-watcher/v3/e2e/harness"
)

func TestFastInflowSlowHook(t *testing.T) {
	t.Parallel()
	for _, deliver := range []string{"last", "each"} {
		for _, interrupt := range []string{"none", "oldest"} {
			t.Run(fmt.Sprintf("deliver=%s/interrupt=%s", deliver, interrupt), func(t *testing.T) {
				t.Parallel()
				// each는 기다리지 않고 받은 이벤트마다 실행하게 하고, last는 기다린 뒤 마지막 경로 하나만 실행하게 한다.
				each, wait := deliver == "each", 30
				sequential := each && interrupt == "none"
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
        exclude: ['*.probe', '*.swp']
        fileEvent: CU
      hookBatch:
        waitMillisecond: %d
        deliver: %s
      run:
        interrupt: %s
`, s.Hook("--error-context", "--line=NOTIFY"), s.Watch, s.Hook("--delay=80ms", "--line=DONE:{{.Path}}"), s.Hook("--line=FINISH"), wait, deliver, interrupt))
				initial := filepath.Join(s.Watch, "initial.txt")
				s.Write(initial, []byte("initial"))
				s.Await("slow hook starts", 5*time.Second, func() bool { return harness.DelayedHookBegun(s.Lines(), "DONE:"+initial) })
				started := time.Now()
				var paths []string
				shared := filepath.Join(s.Watch, "shared.txt")
				for n := range 24 {
					path := filepath.Join(s.Watch, fmt.Sprintf("%02d.txt", n))
					paths = append(paths, path)
					s.Write(path, []byte(fmt.Sprint(n)))
					s.Write(shared, []byte(fmt.Sprint(n)))
					s.Write(filepath.Join(s.Watch, "excluded.swp"), []byte(fmt.Sprint(n)))
					time.Sleep(10 * time.Millisecond)
				}
				elapsed := time.Since(started)
				deadline := time.Now().Add(30 * time.Second)
				inflowPaths := append([]string{initial, shared}, paths...)
				s.AwaitChangesComplete("DONE:", inflowPaths, fsnotify.Create|fsnotify.Write, sequential, time.Until(deadline))
				last := filepath.Join(s.Watch, "last.txt")
				s.Write(last, []byte("last"))
				s.Write(filepath.Join(s.Watch, "excluded.swp"), []byte("after isolated valid event"))
				s.AwaitLine("isolated valid change completes after inflow", time.Until(deadline), "DONE:"+last)
				lines := s.Lines()
				if sequential {
					for _, path := range paths {
						if harness.CountRecord(lines, "DONE:"+path) == 0 {
							t.Errorf("sequential accepted change not completed: %s", path)
						}
					}
				}
				for _, line := range lines {
					if strings.Contains(line, "excluded.swp") || strings.Contains(line, "readiness.probe") {
						t.Errorf("excluded hook: %s", line)
					}
				}
				// 이벤트가 더 들어오고 느린 명령이 실행되는 동안 정지한다.
				s.Write(filepath.Join(s.Watch, "shutdown.txt"), []byte("stop"))
				s.Await("shutdown event received", 5*time.Second, func() bool { return strings.Contains(s.Log(), "shutdown.txt") })
				s.Stop()
				if got := errorContexts(t, s.Records); len(got) != 0 {
					t.Fatalf("normal/merged events invoked onError: %v", got)
				}
				lines = s.Lines()
				if n := harness.CountRecord(lines, "FINISH"); n != 1 {
					t.Fatalf("finish hook count = %d, want 1", n)
				}
				finish := slices.Index(lines, "FINISH")
				for _, line := range lines[finish+1:] {
					if strings.HasPrefix(line, "BEGIN|") {
						t.Fatalf("hook after finish: %s", line)
					}
				}
				s.AwaitDelayedHooksGone()
				t.Logf("load: 24 unique creates + 24 shared writes + 25 excluded writes; interval=10ms duration=%s hook=80ms wait=%dms records=%d", elapsed, wait, len(lines))
			})
		}
	}
}
