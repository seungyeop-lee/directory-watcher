package service

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/seungyeop-lee/directory-watcher/v3/internal/app/domain"
)

// fill은 감시 대상 기준 상대 경로 dir 안에 파일 n개를 만든다.
func (f *renameFixture) fill(dir string, n int) {
	f.t.Helper()
	for i := 0; i < n; i++ {
		f.write(fmt.Sprintf("%s/f%d.txt", dir, i))
	}
}

func (f *renameFixture) mkdirAll(rel string) {
	f.t.Helper()
	if err := os.MkdirAll(f.path(rel), 0700); err != nil {
		f.t.Fatal(err)
	}
}

func (f *renameFixture) removeAll(rel string) {
	f.t.Helper()
	if err := os.RemoveAll(f.path(rel)); err != nil {
		f.t.Fatal(err)
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

// deliver: each에서 폴더 이벤트를 모으는 동안 생긴 다른 파일의 이벤트는 폴더 이벤트와 생긴 순서대로 hook에
// 가는지 확인한다.
func TestFolderEventKeepsOrderWithFileEvents(t *testing.T) {
	f := newRenameFixture(t)
	f.start(nil)
	f.write("first.txt")
	f.mkdirAll("src/n")
	f.write("src/n/in.txt")
	f.write("last.txt")
	var order []string
	for _, ev := range f.changes() {
		// 파일을 쓰면 운영체제에 따라 C 다음에 U가 오기도 하므로 경로마다 처음 받은 이벤트만 본다.
		if path := ev[2:]; !slices.Contains(order, path) {
			order = append(order, path)
		}
	}
	if want := []string{"first.txt", "src/n/", "last.txt"}; !slices.Equal(order, want) {
		t.Fatalf("hook received paths in order %v, want %v", order, want)
	}
}

// 새 폴더에 파일을 계속 써서 폴더 이벤트를 모으는 시간이 끝나지 않아도, dirEventMaxWait 안에 폴더 C가 나가고
// 그 뒤의 쓰기는 따로 hook에 가는지 확인한다.
func TestFolderEventIsSentWithinMaxWaitDuringContinuousWrites(t *testing.T) {
	const maxWait = 500 * time.Millisecond
	f := newRenameFixture(t)
	f.startWith(domain.WatchTargetsOption{
		Deliver:         domain.DeliverEach,
		WatchSubDir:     true,
		WatchFileEvent:  domain.WatchAllEvent(),
		WatchDirEvent:   domain.WatchEvent{Create: true, Delete: true},
		DirEventWait:    100,
		DirEventMaxWait: domain.Millisecond(maxWait / time.Millisecond),
	})
	stop, writing := make(chan struct{}), make(chan struct{})
	defer func() { close(stop); <-writing }()
	start := time.Now()
	f.mkdirAll("src/n")
	go func() {
		defer close(writing)
		for n := 0; ; n++ {
			select {
			case <-stop:
				return
			case <-time.After(50 * time.Millisecond):
			}
			_ = os.WriteFile(f.path("src/n/a.txt"), []byte(fmt.Sprint(n)), 0600)
		}
	}()
	await := func(want func(domain.Event) bool, label string) time.Duration {
		t.Helper()
		for {
			select {
			case ev := <-f.changed:
				if want(ev) {
					return time.Since(start)
				}
			case err := <-f.result:
				t.Fatalf("run stopped: %v", err)
			case <-time.After(5 * time.Second):
				t.Fatalf("%s did not reach the hook", label)
			}
		}
	}
	folder := await(func(ev domain.Event) bool { return ev.IsDir && ev.Path.String() == f.path("src/n") }, "C of src/n")
	// 감시 루프가 이벤트를 받기까지의 지연을 생각해 여유를 둔다.
	if folder > maxWait+300*time.Millisecond {
		t.Errorf("C of src/n reached the hook after %v, want within %v", folder, maxWait)
	}
	await(func(ev domain.Event) bool { return ev.Path.String() == f.path("src/n/a.txt") }, "write in src/n after the folder event")
	f.write("other.txt")
	await(func(ev domain.Event) bool { return ev.Path.String() == f.path("other.txt") }, "write of other.txt")
}

// watchDirEvent가 none이면 폴더 이벤트도, 폴더 이벤트에 흡수된 파일 이벤트도 hook에 가지 않는지 확인한다.
func TestNoFolderEventStillAbsorbsFileEvents(t *testing.T) {
	f := newRenameFixture(t)
	f.fill("src/pkg", 5)
	f.fill("../outside/x", 3)
	f.startWith(domain.WatchTargetsOption{Deliver: domain.DeliverEach, DirEventWait: 100, DirEventMaxWait: 1000, WatchSubDir: true, WatchFileEvent: domain.WatchAllEvent()})
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

// watchSubDir가 false이면 감시하지 않는 대상 바로 아래 폴더도 만들거나 지우면 폴더 이벤트로 알리는지 확인한다.
func TestFolderEventWithoutWatchSubDir(t *testing.T) {
	f := newRenameFixture(t)
	f.sentinelDirs = []string{""}
	f.fill("src", 3)
	f.startWith(domain.WatchTargetsOption{
		Deliver:         domain.DeliverEach,
		DirEventWait:    100,
		DirEventMaxWait: 1000,
		WatchFileEvent:  domain.WatchAllEvent(),
		WatchDirEvent:   domain.WatchEvent{Create: true, Delete: true},
	})
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

// next는 hook이 받은 다음 이벤트를 changes와 같은 꼴로 돌려준다.
func (f *renameFixture) next() string {
	f.t.Helper()
	select {
	case ev := <-f.changed:
		rel, err := filepath.Rel(f.target, ev.Path.String())
		if err != nil {
			f.t.Fatal(err)
		}
		rel = filepath.ToSlash(rel)
		if ev.IsDir {
			rel += "/"
		}
		return ev.Operation.String() + " " + rel
	case err := <-f.result:
		f.t.Fatalf("run stopped: %v", err)
	case <-time.After(5 * time.Second):
		f.t.Fatal("no event reached the hook")
	}
	return ""
}

// 폴더 이벤트를 모으는 동안 쌓인 이벤트가 대기열 용량보다 많아도, 내보낼 때 대기열이 넘쳐 감시가 멈추지 않는지
// 확인한다. 폴더 안의 파일만 지우면 폴더 이벤트가 없어 파일 이벤트가 모두 hook에 간다.
func TestBatchLargerThanQueueDoesNotOverflow(t *testing.T) {
	const files = dispatchCapacity + 100
	f := newRenameFixture(t)
	f.fill("src", files)
	var mu sync.Mutex
	deleted := 0
	f.runner, f.changed, f.result = startExcludeRunner(t, f.target, domain.WatchTargetsOption{
		Deliver:         domain.DeliverEach,
		DirEventWait:    100,
		DirEventMaxWait: 1000,
		WatchSubDir:     true,
		WatchFileEvent:  domain.WatchAllEvent(),
		WatchDirEvent:   domain.WatchEvent{Create: true, Delete: true},
	}, newMockLogger(), func(ev domain.Event) {
		if ev.Operation.String() == "D" {
			mu.Lock()
			deleted++
			mu.Unlock()
		}
		// hook이 대기열을 비우는 속도를 늦춘다.
		time.Sleep(time.Millisecond)
	})
	// startExcludeRunner의 hook 쪽 채널은 dispatchCapacity만큼만 받아 두므로 계속 비운다.
	go func() {
		for range f.changed {
		}
	}()
	for i := 0; i < files; i++ {
		if err := os.Remove(f.path(fmt.Sprintf("src/f%d.txt", i))); err != nil {
			t.Fatal(err)
		}
	}
	deadline := time.After(10 * time.Second)
	for {
		mu.Lock()
		n := deleted
		mu.Unlock()
		if n == files {
			return
		}
		select {
		case err := <-f.result:
			t.Fatalf("run stopped after %d deletions reached the hook: %v", n, err)
		case <-deadline:
			t.Fatalf("%d of %d deletions reached the hook", n, files)
		case <-time.After(10 * time.Millisecond):
		}
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
