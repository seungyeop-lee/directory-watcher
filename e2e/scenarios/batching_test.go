package scenarios

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/seungyeop-lee/directory-watcher/v3/e2e/harness"
)

// TestBatchingOptions는 maxWait를 기다리는 동안 받은 이벤트 수십 개의 hook이 10초 안에 모두 끝나기를 요구한다.
// Windows에서는 하위 테스트끼리만 함께 실행해도 이 한도를 넘었으므로, 하위 테스트까지 병렬로 실행하지 않는다.
func TestBatchingOptions(t *testing.T) {
	t.Run("lastPerPath", func(t *testing.T) {
		s := harness.NewScenario(t)
		a, b := filepath.Join(s.Watch, "a.txt"), filepath.Join(s.Watch, "b.txt")
		transient := filepath.Join(s.Watch, "transient.txt")
		dir := filepath.Join(s.Watch, "removed")
		child := filepath.Join(dir, "child.txt")
		mkdirAll(t, dir)
		s.Write(child, nil)
		s.Write(b, nil)
		s.Start(batchConfig(s, 0, 0, 1500, 10000, "lastPerPath"))
		paths := []string{a, b, transient, child, dir}
		received := func() []string {
			return batchReceipts(s.Log(), paths, fsnotify.Create|fsnotify.Write|fsnotify.Remove|fsnotify.Rename)
		}
		act := func(path string, action func()) {
			t.Helper()
			before := len(received())
			action()
			s.Await("received change of "+path, 5*time.Second, func() bool {
				return slices.Contains(received()[before:], path)
			})
		}
		act(a, func() { s.Write(a, []byte("created")) })
		act(b, func() { s.Write(b, []byte("updated")) })
		act(a, func() { s.Write(a, []byte("updated again")) })
		act(transient, func() { createEmpty(t, transient) })
		for _, path := range []string{transient, child, dir} {
			act(path, func() {
				if err := os.Remove(path); err != nil {
					t.Fatal(err)
				}
			})
		}
		finals := map[string]string{a: "CHANGE:C,false," + a, b: "CHANGE:U,false," + b, dir: "CHANGE:D,true," + dir}
		var want []string
		quietSince, previous := time.Now(), ""
		s.Await("final per-path changes in received order", 15*time.Second, func() bool {
			snapshot := s.Log() + strings.Join(s.Lines(), "\n")
			if snapshot != previous {
				quietSince, previous = time.Now(), snapshot
			}
			want = nil
			order := received()
			for i, path := range order {
				if line, ok := finals[path]; ok && !slices.Contains(order[i+1:], path) {
					want = append(want, line)
				}
			}
			return len(want) == 3 && slices.Equal(s.Lines(), want) && time.Since(quietSince) >= 120*time.Millisecond
		})
		s.Stop()
		if !slices.Equal(s.Lines(), want) {
			t.Fatalf("final changes = %v, want %v", s.Lines(), want)
		}
		t.Logf("lastPerPath: received=%v final=%v; transient and deleted descendant absent", received(), want)
	})

	t.Run("dirBatchWaitZero", func(t *testing.T) {
		s := harness.NewScenario(t)
		s.Start(batchConfig(s, 0, 0, 0, 0, "each"))
		dir := filepath.Join(s.Watch, "created")
		mkdirAll(t, dir)
		probe := filepath.Join(dir, "nested.probe")
		s.Await("new folder watch receives excluded probe", 5*time.Second, func() bool {
			s.Write(probe, nil)
			return strings.Contains(s.Log(), strconv.Quote(probe))
		})
		paths := []string{dir}
		for _, name := range []string{"one.txt", "two.txt"} {
			path := filepath.Join(dir, name)
			paths = append(paths, path)
			createEmpty(t, path)
		}
		// each의 완료 경계는 파일 조작 수가 아니라 DEBUG에서 받은 CREATE 수를 쓴다.
		s.AwaitChangesComplete("CHANGE:", paths, fsnotify.Create, true, 10*time.Second)
		s.Stop()
		for _, path := range paths {
			if harness.CountRecord(s.Lines(), "CHANGE:"+path) == 0 {
				t.Fatalf("folder or contained file not forwarded: %s", path)
			}
		}
		if got, want := len(s.Lines()), len(batchReceipts(s.Log(), paths, fsnotify.Create)); got != want {
			t.Fatalf("forwarded creates = %d, received %d", got, want)
		}
	})

	for _, kind := range []string{"hookBatch", "dirBatch"} {
		t.Run(kind+"MaxWait", func(t *testing.T) {
			const wait, maxWait = 500 * time.Millisecond, 1800 * time.Millisecond
			s := harness.NewScenario(t)
			dirWait, dirMax, hookWait, hookMax, deliver := 0, 0, int(wait/time.Millisecond), int(maxWait/time.Millisecond), "last"
			if kind == "dirBatch" {
				dirWait, dirMax, hookWait, hookMax, deliver = hookWait, hookMax, 0, 0, "each"
			}
			s.Start(batchConfig(s, dirWait, dirMax, hookWait, hookMax, deliver))
			initial := filepath.Join(s.Watch, "initial.txt")
			if kind == "dirBatch" {
				initial = filepath.Join(s.Watch, "created")
			}
			started := time.Now()
			if kind == "dirBatch" {
				mkdirAll(t, initial)
			} else {
				createEmpty(t, initial)
			}
			paths := []string{initial}
			lastReceipt, firstReceipt := time.Time{}, time.Time{}
			seen, inputs := 0, 0
			var flushedAt time.Time
			s.Await("maxWait flush while short-gap inflow continues", maxWait+2*time.Second, func() bool {
				now := time.Now()
				path := filepath.Join(s.Watch, fmt.Sprintf("inflow-%04d.txt", inputs))
				paths = append(paths, path)
				createEmpty(t, path)
				inputs++
				received := batchReceipts(s.Log(), paths, fsnotify.Create)
				if len(received) > seen {
					if !lastReceipt.IsZero() && now.Sub(lastReceipt) >= wait {
						t.Fatalf("received inflow gap %s reached wait %s", now.Sub(lastReceipt), wait)
					}
					if firstReceipt.IsZero() {
						firstReceipt = now
					}
					lastReceipt, seen = now, len(received)
				}
				lines := s.Lines()
				flushed := len(lines) > 0
				if kind == "dirBatch" {
					flushed = harness.CountRecord(lines, "CHANGE:"+initial) > 0
				}
				if !flushed {
					return false
				}
				flushedAt = now
				if firstReceipt.IsZero() || now.Sub(lastReceipt) >= wait {
					t.Fatal("flush observed without continued received inflow")
				}
				if elapsed := now.Sub(started); elapsed < maxWait-100*time.Millisecond || elapsed > maxWait+1500*time.Millisecond {
					t.Fatalf("flush after %s, want %s with -100ms/+1500ms scheduling allowance", elapsed, maxWait)
				}
				return true
			})
			// flush가 입력 중단보다 먼저였고, 그 뒤 이벤트도 독립적으로 실행되는지 확인한다.
			tail := filepath.Join(s.Watch, "tail.txt")
			if kind == "dirBatch" {
				tail = filepath.Join(initial, "tail.txt")
			}
			createEmpty(t, tail)
			paths = append(paths, tail)
			s.AwaitChangesComplete("CHANGE:", paths, fsnotify.Create, kind == "dirBatch", 10*time.Second)
			s.AwaitLine("post-flush event forwarded", 5*time.Second, "CHANGE:"+tail)
			s.Stop()
			t.Logf("%s: wait=%s maxWait=%s flush=%s inputs-at-flush=%d received-at-flush=%d last-receipt-age=%s", kind, wait, maxWait, flushedAt.Sub(started), inputs, seen, flushedAt.Sub(lastReceipt))
		})
	}
}

func batchConfig(s *harness.Scenario, dirWait, dirMax, hookWait, hookMax int, deliver string) string {
	line := "CHANGE:{{.Path}}"
	if deliver == "lastPerPath" {
		line = "CHANGE:{{.Event}},{{.IsDir}},{{.Path}}"
	}
	return fmt.Sprintf(`watchTargets:
  - path: '%s'
    lifeCycle:
      onChange:
        - %s
    option:
      watch:
        exclude: ['**/*.probe']
        fileEvent: CUD
      dirBatch:
        waitMillisecond: %d
        maxWaitMillisecond: %d
      hookBatch:
        waitMillisecond: %d
        maxWaitMillisecond: %d
        deliver: %s
`, s.Watch, s.Hook("--line="+line), dirWait, dirMax, hookWait, hookMax, deliver)
}

// batchReceipts는 완료 경계와 같은 DEBUG event 형식에서 지정한 경로의 실제 수신 순서를 읽는다.
// 삭제도 포함하되 Windows의 폴더 WRITE나 CHMOD는 최종 변경 순서에 넣지 않는다.
func batchReceipts(log string, paths []string, operations fsnotify.Op) []string {
	var received []string
	for _, line := range strings.Split(log, "\n") {
		_, event, ok := strings.Cut(line, "event: ")
		if !ok {
			continue
		}
		quote := strings.IndexByte(event, '"')
		if quote < 0 {
			continue
		}
		path, err := strconv.Unquote(event[quote:])
		if err != nil || !slices.Contains(paths, path) {
			continue
		}
		op := event[:quote]
		for _, candidate := range []struct {
			op   fsnotify.Op
			name string
		}{{fsnotify.Create, "CREATE"}, {fsnotify.Write, "WRITE"}, {fsnotify.Remove, "REMOVE"}, {fsnotify.Rename, "RENAME"}} {
			if operations.Has(candidate.op) && strings.Contains(op, candidate.name) {
				if candidate.op == fsnotify.Write {
					if info, err := os.Stat(path); err == nil && info.IsDir() {
						continue
					}
				}
				received = append(received, path)
				break
			}
		}
	}
	return received
}
