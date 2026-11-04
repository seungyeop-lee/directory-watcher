package service

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/seungyeop-lee/directory-watcher/v3/internal/app/domain"
	"github.com/seungyeop-lee/directory-watcher/v3/internal/helper"
)

// mockLogger는 테스트용 모의 Logger다.
type mockLogger struct{}

func (m *mockLogger) Info(msg string)  {}
func (m *mockLogger) Error(msg string) {}
func (m *mockLogger) Debug(msg string) {}

func newMockLogger() helper.Logger {
	return &mockLogger{}
}

// testQueueSize는 대기열을 채우지 않는 테스트의 대기열 용량이다. hook이 받은 실행을 모으는 채널도 이만큼 받아 둔다.
const testQueueSize = 2048

// hookBatchOption은 설정 파일에 option을 적지 않았을 때의 감시 대상 옵션에서 hookBatch 값만 바꾼다.
func hookBatchOption(wait, maxWait domain.Millisecond, deliver domain.Deliver) domain.WatchTargetsOption {
	return domain.WatchTargetsOption{
		WaitMillisecond:    wait,
		MaxWaitMillisecond: maxWait,
		Deliver:            deliver,
		WatchSubDir:        true,
		WatchFileEvent:     domain.WatchAllEvent(),
		WatchDirEvent:      domain.WatchEvent{Create: true, Delete: true},
		DirEventWait:       100,
		DirEventMaxWait:    1000,
		QueueSize:          testQueueSize,
		Run:                domain.RunPolicy{Concurrency: 1},
	}
}

// hookBatchFixture는 fsnotify 이벤트를 직접 넣는 감시 대상 runner와, hook이 받은 실행을 둔다. 실제 watcher의
// 이벤트는 받지 않으므로, 디스크를 바꾼 뒤 그 이벤트를 send로 넣는다.
type hookBatchFixture struct {
	t      *testing.T
	root   string
	events chan<- fsnotify.Event
	runs   chan domain.Event
	result chan error
}

// startHookBatchRunner는 root를 option으로 감시하는 runner를 시작한다. onChange는 hook이 실행을 받을 때 먼저 실행한다.
func startHookBatchRunner(t *testing.T, root string, option domain.WatchTargetsOption, onChange func(domain.Event)) *hookBatchFixture {
	t.Helper()
	r := NewWatchTargetRunner(domain.GlobalCommandSet{}, domain.WatchTargetsCommandSet{Path: domain.Path(root), Option: option}, newMockLogger())
	events, _ := controlledSource(r)
	f := &hookBatchFixture{t: t, root: root, events: events, runs: make(chan domain.Event, 64), result: make(chan error, 1)}
	ready := make(chan struct{})
	r.runStartHook = func() { close(ready) }
	r.runChangeHook = func(event domain.Event) {
		if onChange != nil {
			onChange(event)
		}
		f.runs <- event
	}
	r.runFinishHook = func() {}
	t.Cleanup(func() {
		var wg sync.WaitGroup
		wg.Add(1)
		r.Stop(&wg)
	})
	go func() { f.result <- r.Run() }()
	select {
	case <-ready:
	case err := <-f.result:
		t.Fatalf("run before registration: %v", err)
	case <-time.After(5 * time.Second):
		t.Fatal("watcher did not become ready")
	}
	return f
}

func (f *hookBatchFixture) path(rel string) string {
	return filepath.Join(f.root, filepath.FromSlash(rel))
}

// send는 감시 대상 기준 상대 경로 rel의 fsnotify 이벤트 op를 감시 루프에 넣는다.
func (f *hookBatchFixture) send(op fsnotify.Op, rel string) {
	f.t.Helper()
	select {
	case f.events <- fsnotify.Event{Name: f.path(rel), Op: op}:
	case err := <-f.result:
		f.t.Fatalf("run stopped: %v", err)
	}
}

// collect는 hook이 quiet 동안 실행을 더 받지 않을 때까지 받은 실행을 "C a.txt"처럼 돌려준다. 폴더는 "C d/"처럼 쓴다.
func (f *hookBatchFixture) collect(quiet time.Duration) []string {
	f.t.Helper()
	got := []string{}
	for {
		select {
		case ev := <-f.runs:
			rel, err := filepath.Rel(f.root, ev.Path.String())
			if err != nil {
				f.t.Fatal(err)
			}
			rel = filepath.ToSlash(rel)
			if ev.IsDir {
				rel += "/"
			}
			got = append(got, ev.Operation.String()+" "+rel)
		case err := <-f.result:
			f.t.Fatalf("run stopped: %v", err)
		case <-time.After(quiet):
			return got
		}
	}
}

func writeFile(t *testing.T, path string) {
	t.Helper()
	if err := os.WriteFile(path, []byte("event"), 0600); err != nil {
		t.Fatal(err)
	}
}

// 한 번 기다리는 동안 모인 이벤트를 deliver 값마다 정해진 실행으로 넘기는지 확인한다. x.swp는 생겼다가 사라져
// 최종 변화가 없다.
func TestDeliverTurnsOneWaitIntoRuns(t *testing.T) {
	cases := []struct {
		deliver domain.Deliver
		name    string
		want    []string
	}{
		{domain.DeliverLast, "last", []string{"U b.txt"}},
		{domain.DeliverLastPerPath, "lastPerPath", []string{"C a.txt", "U b.txt"}},
		{domain.DeliverEach, "each", []string{"C a.txt", "U a.txt", "U b.txt", "C x.swp", "D x.swp"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			writeFile(t, filepath.Join(root, "b.txt"))
			f := startHookBatchRunner(t, root, hookBatchOption(50, 1000, tc.deliver), nil)
			writeFile(f.t, f.path("a.txt"))
			f.send(fsnotify.Create, "a.txt")
			f.send(fsnotify.Write, "a.txt")
			f.send(fsnotify.Write, "b.txt")
			f.send(fsnotify.Create, "x.swp")
			f.send(fsnotify.Remove, "x.swp")
			if got := f.collect(300 * time.Millisecond); !slices.Equal(got, tc.want) {
				t.Fatalf("runs = %v, want %v", got, tc.want)
			}
		})
	}
}

// last와 lastPerPath가 경로의 최종 변화를 이전 기록과 지금 디스크 상태로 정하는지 확인한다.
func TestFinalChangeComparesRecordWithDisk(t *testing.T) {
	cases := []struct {
		name  string
		setup func(t *testing.T, root string)
		act   func(f *hookBatchFixture)
		want  []string
	}{
		{
			name: "file created",
			act:  func(f *hookBatchFixture) { writeFile(f.t, f.path("f.txt")); f.send(fsnotify.Create, "f.txt") },
			want: []string{"C f.txt"},
		},
		{
			name: "file created without its create event",
			act:  func(f *hookBatchFixture) { writeFile(f.t, f.path("f.txt")); f.send(fsnotify.Write, "f.txt") },
			want: []string{"C f.txt"},
		},
		{
			name:  "file modified",
			setup: func(t *testing.T, root string) { writeFile(t, filepath.Join(root, "f.txt")) },
			act:   func(f *hookBatchFixture) { writeFile(f.t, f.path("f.txt")); f.send(fsnotify.Write, "f.txt") },
			want:  []string{"U f.txt"},
		},
		{
			name:  "file deleted",
			setup: func(t *testing.T, root string) { writeFile(t, filepath.Join(root, "f.txt")) },
			act: func(f *hookBatchFixture) {
				removeAll(f.t, f.path("f.txt"))
				f.send(fsnotify.Remove, "f.txt")
			},
			want: []string{"D f.txt"},
		},
		{
			name: "file created and deleted",
			act: func(f *hookBatchFixture) {
				writeFile(f.t, f.path("f.txt"))
				f.send(fsnotify.Create, "f.txt")
				removeAll(f.t, f.path("f.txt"))
				f.send(fsnotify.Remove, "f.txt")
			},
			want: []string{},
		},
		{
			name: "folder created",
			act:  func(f *hookBatchFixture) { mkdir(f.t, f.path("d")); f.send(fsnotify.Create, "d") },
			want: []string{"C d/"},
		},
		{
			name:  "folder deleted and created again",
			setup: func(t *testing.T, root string) { mkdir(t, filepath.Join(root, "d")) },
			act: func(f *hookBatchFixture) {
				removeAll(f.t, f.path("d"))
				f.send(fsnotify.Remove, "d")
				mkdir(f.t, f.path("d"))
				f.send(fsnotify.Create, "d")
			},
			want: []string{},
		},
		{
			name:  "folder deleted",
			setup: func(t *testing.T, root string) { mkdir(t, filepath.Join(root, "d")) },
			act: func(f *hookBatchFixture) {
				removeAll(f.t, f.path("d"))
				f.send(fsnotify.Remove, "d")
			},
			want: []string{"D d/"},
		},
		{
			name: "folder created and deleted",
			act: func(f *hookBatchFixture) {
				mkdir(f.t, f.path("d"))
				f.send(fsnotify.Create, "d")
				removeAll(f.t, f.path("d"))
				f.send(fsnotify.Remove, "d")
			},
			want: []string{},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			if tc.setup != nil {
				tc.setup(t, root)
			}
			f := startHookBatchRunner(t, root, hookBatchOption(50, 1000, domain.DeliverLastPerPath), nil)
			tc.act(f)
			if got := f.collect(400 * time.Millisecond); !slices.Equal(got, tc.want) {
				t.Fatalf("runs = %v, want %v", got, tc.want)
			}
		})
	}
}

// 생성 이벤트를 빠뜨린 파일은 처음 수정 이벤트에서 기록에 들어가, 다음에 기다리는 동안의 수정은 수정으로 나가는지 확인한다.
func TestFileWithoutCreateEventIsRecordedOnWrite(t *testing.T) {
	f := startHookBatchRunner(t, t.TempDir(), hookBatchOption(50, 1000, domain.DeliverLast), nil)
	writeFile(f.t, f.path("f.txt"))
	f.send(fsnotify.Write, "f.txt")
	if got, want := f.collect(300*time.Millisecond), []string{"C f.txt"}; !slices.Equal(got, want) {
		t.Fatalf("runs after the first write = %v, want %v", got, want)
	}
	f.send(fsnotify.Write, "f.txt")
	if got, want := f.collect(300*time.Millisecond), []string{"U f.txt"}; !slices.Equal(got, want) {
		t.Fatalf("runs after the second write = %v, want %v", got, want)
	}
}

// waitMillisecond보다 짧은 간격으로 파일을 계속 써도, 첫 이벤트부터 maxWaitMillisecond 안에 hook이 실행되는지 확인한다.
func TestMaxWaitRunsHookDuringContinuousWrites(t *testing.T) {
	const maxWait = 500 * time.Millisecond
	root := t.TempDir()
	_, changed, result := startExcludeRunner(t, root, hookBatchOption(100, domain.Millisecond(maxWait/time.Millisecond), domain.DeliverLast), newMockLogger(), nil)
	stop, writing := make(chan struct{}), make(chan struct{})
	defer func() { close(stop); <-writing }()
	start := time.Now()
	go func() {
		defer close(writing)
		for n := 0; ; n++ {
			_ = os.WriteFile(filepath.Join(root, "a.txt"), []byte(fmt.Sprint(n)), 0600)
			select {
			case <-stop:
				return
			case <-time.After(50 * time.Millisecond):
			}
		}
	}()
	select {
	case <-changed:
		// 감시 루프가 이벤트를 받기까지의 지연을 생각해 여유를 둔다.
		if elapsed := time.Since(start); elapsed > maxWait+300*time.Millisecond {
			t.Errorf("hook ran after %v, want within %v", elapsed, maxWait)
		}
	case err := <-result:
		t.Fatalf("run stopped: %v", err)
	case <-time.After(5 * time.Second):
		t.Fatal("hook did not run while the file kept changing")
	}
}

// hook이 도는 동안 기다리는 시간이 여러 번 끝나면, 그때마다 만든 실행이 대기열에 따로 쌓여 하나로 합쳐지지 않는지 확인한다.
func TestRunsFromSeparateWaitsAreNotMerged(t *testing.T) {
	entered, release := make(chan struct{}), make(chan struct{})
	var once sync.Once
	f := startHookBatchRunner(t, t.TempDir(), hookBatchOption(20, 1000, domain.DeliverLast), func(domain.Event) {
		once.Do(func() { close(entered); <-release })
	})
	writeFile(f.t, f.path("a.txt"))
	f.send(fsnotify.Create, "a.txt")
	<-entered
	for _, name := range []string{"b.txt", "c.txt"} {
		writeFile(f.t, f.path(name))
		f.send(fsnotify.Create, name)
		time.Sleep(100 * time.Millisecond)
	}
	close(release)
	if got, want := f.collect(300*time.Millisecond), []string{"C a.txt", "C b.txt", "C c.txt"}; !slices.Equal(got, want) {
		t.Fatalf("runs = %v, want %v", got, want)
	}
}

// 폴더 삭제가 폴더 이벤트 묶음 두 개로 나뉘어 한 번 기다리는 동안 모이면, lastPerPath는 최종 변화가 D인 폴더
// 아래의 D를 넘기지 않는지 확인한다. last와 each의 결과는 hookBatch가 폴더 이벤트를 흡수하지 않을 때와 같다.
func TestLastPerPathDropsDeletesUnderDeletedFolder(t *testing.T) {
	cases := []struct {
		deliver  domain.Deliver
		name     string
		dirEvent domain.WatchEvent
		want     []string
	}{
		{domain.DeliverLastPerPath, "lastPerPath", domain.WatchEvent{Create: true, Delete: true}, []string{"D d/"}},
		{domain.DeliverLastPerPath, "lastPerPath with dirEvent none", domain.WatchEvent{}, []string{}},
		{domain.DeliverLast, "last", domain.WatchEvent{Create: true, Delete: true}, []string{"D d/"}},
		{domain.DeliverEach, "each", domain.WatchEvent{Create: true, Delete: true}, []string{"D d/sub/g.txt", "D d/f.txt", "D d/sub/", "D d/"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			mkdir(t, filepath.Join(root, "d"))
			mkdir(t, filepath.Join(root, "d", "sub"))
			writeFile(t, filepath.Join(root, "d", "f.txt"))
			writeFile(t, filepath.Join(root, "d", "sub", "g.txt"))
			option := hookBatchOption(300, 2000, tc.deliver)
			option.DirEventWait, option.DirEventMaxWait = 20, 20
			option.WatchDirEvent = tc.dirEvent
			f := startHookBatchRunner(t, root, option, nil)
			removeAll(f.t, f.path("d"))
			// 파일 삭제와 폴더 삭제 사이에 dirBatch.waitMillisecond보다 긴 틈을 두어 폴더 이벤트 묶음을 나눈다.
			f.send(fsnotify.Remove, "d/sub/g.txt")
			f.send(fsnotify.Remove, "d/f.txt")
			time.Sleep(100 * time.Millisecond)
			f.send(fsnotify.Remove, "d/sub")
			time.Sleep(100 * time.Millisecond)
			f.send(fsnotify.Remove, "d")
			if got := f.collect(600 * time.Millisecond); !slices.Equal(got, tc.want) {
				t.Fatalf("runs = %v, want %v", got, tc.want)
			}
		})
	}
}

// 폴더 생성과 그 안의 파일 생성이 폴더 이벤트 묶음 두 개로 나뉘어 한 번 기다리는 동안 모이면, lastPerPath가
// 파일의 C도 넘기는지 확인한다. 폴더의 C를 받은 hook이 그 뒤에 생긴 파일을 놓치지 않아야 한다.
func TestLastPerPathKeepsCreatesUnderCreatedFolder(t *testing.T) {
	root := t.TempDir()
	option := hookBatchOption(300, 2000, domain.DeliverLastPerPath)
	option.DirEventWait, option.DirEventMaxWait = 20, 20
	f := startHookBatchRunner(t, root, option, nil)
	mkdir(f.t, f.path("d"))
	f.send(fsnotify.Create, "d")
	time.Sleep(100 * time.Millisecond)
	writeFile(f.t, f.path("d/f.txt"))
	f.send(fsnotify.Create, "d/f.txt")
	if got, want := f.collect(600*time.Millisecond), []string{"C d/", "C d/f.txt"}; !slices.Equal(got, want) {
		t.Fatalf("runs = %v, want %v", got, want)
	}
}

func mkdir(t *testing.T, path string) {
	t.Helper()
	if err := os.Mkdir(path, 0700); err != nil {
		t.Fatal(err)
	}
}

func removeAll(t *testing.T, path string) {
	t.Helper()
	if err := os.RemoveAll(path); err != nil {
		t.Fatal(err)
	}
}
