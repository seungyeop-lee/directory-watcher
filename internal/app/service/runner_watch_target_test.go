package service

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/seungyeop-lee/directory-watcher/v3/internal/app/domain"
)

// controlledFixture는 fsnotify 이벤트를 직접 넣는 감시 대상 runner와, hook이 받은 실행을 둔다. 실제 watcher의
// 이벤트는 받지 않으므로, 디스크를 바꾼 뒤 그 이벤트를 send로 넣는다.
type controlledFixture struct {
	t      *testing.T
	root   string
	events chan<- fsnotify.Event
	runs   <-chan domain.Event
	result <-chan error
}

// startControlledRunner는 root를 option으로 감시하는 runner를 시작한다. onChange는 hook이 실행을 받을 때 먼저 실행한다.
func startControlledRunner(t *testing.T, root string, option domain.WatchTargetsOption, onChange func(domain.Event)) *controlledFixture {
	t.Helper()
	r := NewWatchTargetRunner(domain.GlobalCommandSet{}, domain.WatchTargetsCommandSet{Path: domain.Path(root), Option: option}, newMockLogger())
	events, _ := controlledSource(r)
	runs, result := startRunner(t, r, onChange)
	return &controlledFixture{t: t, root: root, events: events, runs: runs, result: result}
}

func (f *controlledFixture) path(rel string) string {
	return filepath.Join(f.root, filepath.FromSlash(rel))
}

// send는 감시 대상 기준 상대 경로 rel의 fsnotify 이벤트 op를 감시 루프에 넣는다.
func (f *controlledFixture) send(op fsnotify.Op, rel string) {
	f.t.Helper()
	select {
	case f.events <- fsnotify.Event{Name: f.path(rel), Op: op}:
	case err := <-f.result:
		f.t.Fatalf("run stopped: %v", err)
	}
}

// collect는 hook이 quiet 동안 실행을 더 받지 않을 때까지 받은 실행을 describeRun의 꼴로 돌려준다.
func (f *controlledFixture) collect(quiet time.Duration) []string {
	f.t.Helper()
	got := []string{}
	for {
		select {
		case run := <-f.runs:
			got = append(got, describeRun(f.t, f.root, run))
		case err := <-f.result:
			f.t.Fatalf("run stopped: %v", err)
		case <-time.After(quiet):
			return got
		}
	}
}

// 생성 이벤트를 빠뜨린 파일은 처음 수정 이벤트에서 기록에 들어가, 다음에 기다리는 동안의 수정은 수정으로 나가는지 확인한다.
func TestFileWithoutCreateEventIsRecordedOnWrite(t *testing.T) {
	f := startControlledRunner(t, t.TempDir(), hookBatchOption(50, 1000, domain.DeliverLast), nil)
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

// hook이 도는 동안 기다리는 시간이 여러 번 끝나면, 그때마다 만든 실행이 대기열에 따로 쌓여 하나로 합쳐지지 않는지 확인한다.
func TestRunsFromSeparateWaitsAreNotMerged(t *testing.T) {
	entered, release := make(chan struct{}), make(chan struct{})
	var once sync.Once
	f := startControlledRunner(t, t.TempDir(), hookBatchOption(20, 1000, domain.DeliverLast), func(domain.Event) {
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

// 폴더가 안의 내용과 함께 생기거나 사라지면 hook이 맨 위 폴더의 이벤트 하나만 받는지 확인한다. 폴더 안의
// 파일 300개를 옮겨 오거나 지워도 그 파일들의 이벤트는 대기열에 들어가지 않는다.
func TestFolderChangeNotifiesTopFolderOnce(t *testing.T) {
	cases := []struct {
		name  string
		setup func(f *renameFixture)
		act   func(f *renameFixture)
		want  []string
	}{
		{name: "mkdir", act: func(f *renameFixture) { f.mkdirAll("src/pkg") }, want: []string{"C src/pkg/"}},
		{name: "mkdir -p", act: func(f *renameFixture) { f.mkdirAll("src/a/b/c") }, want: []string{"C src/a/"}},
		{
			name:  "move in a folder with files",
			setup: func(f *renameFixture) { f.fill("../outside/x", 3); f.write("../outside/x/sub/g.txt") },
			act:   func(f *renameFixture) { f.move("../outside/x", "src/x") },
			want:  []string{"C src/x/"},
		},
		{
			name:  "move in a folder with 300 files",
			setup: func(f *renameFixture) { f.fill("../outside/x", 300) },
			act:   func(f *renameFixture) { f.move("../outside/x", "src/x") },
			want:  []string{"C src/x/"},
		},
		{
			name: "mkdir and write a file in it",
			act: func(f *renameFixture) {
				f.mkdirAll("src/n")
				f.write("src/n/a.txt")
			},
			want: []string{"C src/n/"},
		},
		{
			name:  "rm -rf a folder with 300 files",
			setup: func(f *renameFixture) { f.fill("src/pkg", 300) },
			act:   func(f *renameFixture) { f.removeAll("src/pkg") },
			want:  []string{"D src/pkg/"},
		},
		{
			name: "rm -rf nested folders",
			setup: func(f *renameFixture) {
				f.fill("src/pkg", 5)
				f.fill("src/pkg/sub", 5)
				f.write("src/pkg/sub/deep/g.txt")
			},
			act:  func(f *renameFixture) { f.removeAll("src/pkg") },
			want: []string{"D src/pkg/"},
		},
		{
			name:  "move out of the target",
			setup: func(f *renameFixture) { f.fill("src/pkg", 3) },
			act:   func(f *renameFixture) { f.move("src/pkg", "../outside/pkg") },
			want:  []string{"D src/pkg/"},
		},
		{
			name:  "rename",
			setup: func(f *renameFixture) { f.fill("src/d", 3) },
			act:   func(f *renameFixture) { f.move("src/d", "src/e") },
			want:  []string{"D src/d/", "C src/e/"},
		},
		{
			name:  "move under another folder",
			setup: func(f *renameFixture) { f.fill("src/d", 3) },
			act:   func(f *renameFixture) { f.move("src/d", "dst/d") },
			want:  []string{"D src/d/", "C dst/d/"},
		},
		{
			name:  "rm -rf and mkdir again",
			setup: func(f *renameFixture) { f.fill("src/d", 3) },
			act: func(f *renameFixture) {
				f.removeAll("src/d")
				f.mkdirAll("src/d")
			},
			want: []string{"D src/d/", "C src/d/"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newRenameFixture(t)
			if tc.setup != nil {
				tc.setup(f)
			}
			f.start(nil)
			tc.act(f)
			if got := f.changes(); !slices.Equal(got, tc.want) {
				t.Fatalf("hook events = %v, want %v", got, tc.want)
			}
		})
	}
}

// dirEvent가 none이면 폴더 이벤트도, 폴더 이벤트에 흡수된 파일 이벤트도 hook에 가지 않는지 확인한다.
func TestNoFolderEventStillAbsorbsFileEvents(t *testing.T) {
	f := newRenameFixture(t)
	f.fill("src/pkg", 5)
	f.fill("../outside/x", 3)
	f.startWith(watchTargetOption(domain.WatchOption{SubDir: true, FileEvent: domain.WatchAllEvent()}))
	f.mkdirAll("src/a/b")
	f.write("src/a/b/c.txt")
	f.move("../outside/x", "src/x")
	f.removeAll("src/pkg")
	if got := f.changes(); len(got) != 0 {
		t.Fatalf("hook events = %v, want none", got)
	}
	f.write("src/a/b/later.txt")
	if got := f.changes(); !slices.Contains(got, "C src/a/b/later.txt") {
		t.Fatalf("creation in the new folder did not reach the hook (got %v)", got)
	}
}

// subDir가 false이면 감시하지 않는 대상 바로 아래 폴더도 만들거나 지우면 폴더 이벤트로 알리는지 확인한다.
func TestFolderEventWithoutSubDir(t *testing.T) {
	f := newRenameFixture(t)
	f.sentinelDirs = []string{""}
	f.fill("src", 3)
	f.startWith(watchTargetOption(domain.WatchOption{FileEvent: domain.WatchAllEvent(), DirEvent: domain.WatchEvent{Create: true, Delete: true}}))
	f.removeAll("src")
	if got, want := f.changes(), []string{"D src/"}; !slices.Equal(got, want) {
		t.Fatalf("hook events after removing an existing folder = %v, want %v", got, want)
	}
	f.mkdirAll("new")
	f.fill("new", 3)
	if got, want := f.changes(), []string{"C new/"}; !slices.Equal(got, want) {
		t.Fatalf("hook events after making a folder = %v, want %v", got, want)
	}
	f.removeAll("new")
	if got, want := f.changes(), []string{"D new/"}; !slices.Equal(got, want) {
		t.Fatalf("hook events after removing a new folder = %v, want %v", got, want)
	}
}

// 폴더를 만들고 곧바로 그 안에 파일을 써도 hook이 폴더 C를 받고, 그 뒤 그 폴더 안에 만든 파일의 이벤트도 받는지
// 확인한다. macOS에서는 fsnotify가 새 폴더의 감시 등록을 덮어써 그 폴더 안의 이벤트를 놓치기도 했다. 덮어쓴
// 등록은 다음 이벤트를 받을 때 되돌리므로, 폴더 C를 받기 전에는 다른 이벤트를 만들지 않는다.
func TestFolderCreatedWithFileIsWatchedAfterFolderEvent(t *testing.T) {
	f := newRenameFixture(t)
	f.start(nil)
	f.mkdirAll("src/n")
	f.write("src/n/a.txt")
	if got, want := f.next(), "C src/n/"; got != want {
		t.Fatalf("hook event = %q, want %q", got, want)
	}
	f.write("src/n/b.txt")
	if got := f.changes(); !slices.Contains(got, "C src/n/b.txt") {
		t.Fatalf("creation in the new folder did not reach the hook (got %v)", got)
	}
}

// 폴더 이벤트를 모으는 동안 쌓인 이벤트로 만든 실행이 대기열 용량보다 많으면, 감시 루프는 대기열에 자리가 나기를
// 기다리지 않고 dispatch queue full로 멈춘다. 폴더 안의 파일만 지우면 폴더 이벤트가 없어 파일 이벤트가 모두 실행이 된다.
func TestBatchLargerThanQueueOverflows(t *testing.T) {
	const queueSize, files = 50, 150
	f := newRenameFixture(t)
	f.fill("src", files)
	release := make(chan struct{})
	option := watchTargetOption(domain.WatchOption{SubDir: true, FileEvent: domain.WatchAllEvent(), DirEvent: domain.WatchEvent{Create: true, Delete: true}})
	option.Queue.Size = queueSize
	r := NewWatchTargetRunner(domain.GlobalCommandSet{}, domain.WatchTargetsCommandSet{Path: domain.Path(f.target), Option: option}, newMockLogger())
	f.changed, f.result = startRunner(t, r, func(domain.Event) { <-release })
	t.Cleanup(func() { close(release) })
	for i := 0; i < files; i++ {
		if err := os.Remove(f.path(fmt.Sprintf("src/f%d.txt", i))); err != nil {
			t.Fatal(err)
		}
	}
	select {
	case err := <-f.result:
		if !errors.Is(err, fsnotify.ErrEventOverflow) || !strings.Contains(err.Error(), "dispatch queue full (capacity 50)") {
			t.Fatalf("run stopped with %v, want dispatch queue full (capacity 50)", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("watch loop waited for room in the queue instead of stopping")
	}
}

// 폴더 이벤트는 exclude에 맞는 폴더에는 오지 않는다. dist/**는 dist 폴더 자신도 빼고, dist/*는 dist 안의 항목만 뺀다.
func TestExcludedFolderSendsNoFolderEvent(t *testing.T) {
	f := newRenameFixture(t)
	f.start(domain.ExcludePatterns{"src/dist/**", "dst/out/*"})
	f.mkdirAll("src/dist/sub")
	f.mkdirAll("dst/out/sub")
	got := f.changes()
	if slices.ContainsFunc(got, func(ev string) bool { return strings.Contains(ev, "src/dist") }) {
		t.Errorf("excluded folder src/dist reached the hook (got %v)", got)
	}
	if want := []string{"C dst/out/"}; !slices.Equal(got, want) {
		t.Errorf("hook events = %v, want %v", got, want)
	}
}

// 감시를 시작할 때와 항목을 만들거나 지우거나 옮긴 뒤에, 기록이 감시 대상 안의 실제 파일과 폴더와 같은지
// 확인한다.
func TestRecordMatchesDiskAfterEachChange(t *testing.T) {
	cases := []struct {
		name  string
		setup func(f *renameFixture)
		act   func(f *renameFixture)
		gone  []string
	}{
		{
			name:  "existing at start",
			setup: func(f *renameFixture) { f.fill("src/a/b", 3); f.write("top.txt") },
		},
		{name: "create a file", act: func(f *renameFixture) { f.write("src/new.txt") }},
		{name: "create folders with files", act: func(f *renameFixture) { f.fill("src/n/m", 3) }},
		{
			name:  "remove a file",
			setup: func(f *renameFixture) { f.write("src/a.txt") },
			act:   func(f *renameFixture) { f.removeAll("src/a.txt") },
			gone:  []string{"src/a.txt"},
		},
		{
			name:  "rename a file",
			setup: func(f *renameFixture) { f.write("src/a.txt") },
			act:   func(f *renameFixture) { f.move("src/a.txt", "src/b.txt") },
			gone:  []string{"src/a.txt"},
		},
		{
			name:  "rename a folder",
			setup: func(f *renameFixture) { f.fillMovable("src/d") },
			act:   func(f *renameFixture) { f.move("src/d", "src/e") },
			gone:  []string{"src/d", "src/d/sub", "src/d/sub/f0.txt"},
		},
		{
			name:  "move a folder under another folder",
			setup: func(f *renameFixture) { f.fill("src/d", 3) },
			act:   func(f *renameFixture) { f.move("src/d", "dst/d") },
			gone:  []string{"src/d", "src/d/f0.txt"},
		},
		{
			name:  "move a file in",
			setup: func(f *renameFixture) { f.write("../outside/a.txt") },
			act:   func(f *renameFixture) { f.move("../outside/a.txt", "src/a.txt") },
		},
		{
			name:  "move a folder in",
			setup: func(f *renameFixture) { f.fill("../outside/x/sub", 3) },
			act:   func(f *renameFixture) { f.move("../outside/x", "src/x") },
		},
		{
			name:  "move a file out",
			setup: func(f *renameFixture) { f.write("src/a.txt") },
			act:   func(f *renameFixture) { f.move("src/a.txt", "../outside/a.txt") },
			gone:  []string{"src/a.txt"},
		},
		{
			name:  "move a folder out",
			setup: func(f *renameFixture) { f.fillMovable("src/pkg") },
			act:   func(f *renameFixture) { f.move("src/pkg", "../outside/pkg") },
			gone:  []string{"src/pkg", "src/pkg/sub", "src/pkg/sub/f0.txt"},
		},
		{
			name:  "rm -rf a folder",
			setup: func(f *renameFixture) { f.fill("src/pkg/sub", 3) },
			act:   func(f *renameFixture) { f.removeAll("src/pkg") },
			gone:  []string{"src/pkg", "src/pkg/sub", "src/pkg/sub/f0.txt"},
		},
		{
			name:  "rm -rf and make the folder again",
			setup: func(f *renameFixture) { f.fill("src/d", 3) },
			act: func(f *renameFixture) {
				f.removeAll("src/d")
				f.write("src/d/new.txt")
			},
			gone: []string{"src/d/f0.txt"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newRenameFixture(t)
			if tc.setup != nil {
				tc.setup(f)
			}
			f.start(nil)
			if tc.act != nil {
				tc.act(f)
			}
			f.changes()
			f.stop()
			f.assertRecordMatchesDisk(tc.gone...)
		})
	}
}

// exclude와 기본 제외에 걸린 경로는 감시를 시작할 때 있었든 감시 중에 생겼든 기록에 없는지 확인한다.
func TestRecordSkipsExcludedPaths(t *testing.T) {
	f := newRenameFixture(t)
	f.write("dist/old.txt")
	f.write("src/old.tmp")
	f.write("src/.hidden/old.txt")
	f.write("src/keep.txt")
	f.start(domain.ExcludePatterns{"dist/**", "**/*.tmp"})
	f.write("dist/new.txt")
	f.write("src/new.tmp")
	f.write("src/_new.txt")
	f.write("src/added.txt")
	f.changes()
	f.stop()
	for _, rel := range []string{"dist", "dist/old.txt", "dist/new.txt", "src/old.tmp", "src/new.tmp", "src/.hidden", "src/.hidden/old.txt", "src/_new.txt"} {
		f.assertRecorded(rel, false, false)
	}
	f.assertRecorded("src", true, true)
	f.assertRecorded("src/keep.txt", true, false)
	f.assertRecorded("src/added.txt", true, false)
}

// subDir가 false이면 감시 대상 바로 아래 항목만 기록하고, 그 아래 폴더 안의 항목은 기록하지 않는지 확인한다.
func TestRecordWithoutSubDirHoldsOnlyDirectChildren(t *testing.T) {
	f := newRenameFixture(t)
	f.sentinelDirs = []string{""}
	f.write("top.txt")
	f.fill("src/sub", 2)
	f.fill("gone", 2)
	f.startWith(watchTargetOption(domain.WatchOption{FileEvent: domain.WatchAllEvent(), DirEvent: domain.WatchEvent{Create: true, Delete: true}}))
	f.fill("new", 2)
	f.write("added.txt")
	f.removeAll("gone")
	f.changes()
	f.stop()
	f.assertRecorded("", true, true)
	f.assertRecorded("top.txt", true, false)
	f.assertRecorded("added.txt", true, false)
	f.assertRecorded("src", true, true)
	f.assertRecorded("dst", true, true)
	f.assertRecorded("new", true, true)
	for _, rel := range []string{"src/sub", "src/sub/f0.txt", "new/f0.txt", "gone"} {
		f.assertRecorded(rel, false, false)
	}
}
