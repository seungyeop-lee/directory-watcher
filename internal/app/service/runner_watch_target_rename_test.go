package service

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/seungyeop-lee/directory-watcher/v3/internal/app/domain"
)

// 파일의 이름을 바꾸거나 옮기면 hook이 옛 경로의 삭제(D)를 먼저, 감시 대상 안의 새 경로의 생성(C)을 그다음에
// 받는지 확인한다.
func TestRenamedOrMovedFileNotifiesOldPathAsDelete(t *testing.T) {
	cases := []struct {
		name     string
		from, to string
		want     []string
	}{
		{name: "rename in the same folder", from: "src/a.txt", to: "src/b.txt", want: []string{"D src/a.txt", "C src/b.txt"}},
		{name: "move to another folder in the target", from: "src/a.txt", to: "dst/a.txt", want: []string{"D src/a.txt", "C dst/a.txt"}},
		{name: "move out of the target", from: "src/a.txt", to: "../outside/a.txt", want: []string{"D src/a.txt"}},
		{name: "move into the target", from: "../outside/a.txt", to: "src/a.txt", want: []string{"C src/a.txt"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newRenameFixture(t)
			f.write(tc.from)
			f.start(nil)
			f.move(tc.from, tc.to)
			if got := f.changes(); !slices.Equal(got, tc.want) {
				t.Fatalf("hook events = %v, want %v", got, tc.want)
			}
		})
	}
}

// 폴더의 이름을 바꾸거나 옮기면 hook이 옛 경로의 폴더 삭제(D)를 한 번 받고 그다음에 새 경로의 폴더 생성(C)을 받으며,
// 옮긴 폴더 안의 변경은 새 경로로만 받는지 확인한다. 감시 대상 밖으로 옮긴 폴더는 그 아래 폴더까지 감시가 풀려 옮겨
// 간 곳의 변경으로 hook이 실행되지 않는다.
func TestRenamedOrMovedFolderNotifiesOldPathAsDelete(t *testing.T) {
	cases := []struct {
		name     string
		from, to string
		want     []string
	}{
		{name: "rename in the same folder", from: "src/d", to: "src/e", want: []string{"D src/d/", "C src/e/"}},
		{name: "move to another folder in the target", from: "src/d", to: "dst/d", want: []string{"D src/d/", "C dst/d/"}},
		{name: "move out of the target", from: "src/d", to: "../outside/d", want: []string{"D src/d/"}},
		{name: "move into the target", from: "../outside/d", to: "src/d", want: []string{"C src/d/"}},
	}
	for _, nested := range []bool{false, true} {
		for _, tc := range cases {
			name := tc.name
			if nested {
				name += " with a subfolder"
			}
			t.Run(name, func(t *testing.T) {
				f := newRenameFixture(t)
				files := []string{"f.txt"}
				if nested {
					files = append(files, "sub/g.txt")
				}
				for _, file := range files {
					f.write(tc.from + "/" + file)
				}
				f.start(nil)
				// Windows는 하위 폴더에 감시 핸들이 열린 폴더의 이름 바꾸기와 옮기기를 거부한다. fsnotify는 Windows에서
				// 하위 폴더까지 한 번에 감시하는 방법을 열어 두지 않아, 하위 폴더마다 핸들을 연다.
				if nested && runtime.GOOS == "windows" && !strings.HasPrefix(tc.from, "../") {
					err := os.Rename(f.path(tc.from), f.path(tc.to))
					if !errors.Is(err, fs.ErrPermission) {
						t.Fatalf("rename of a folder that holds a watched folder = %v, want access denied", err)
					}
					return
				}
				f.move(tc.from, tc.to)
				if got := f.changes(); !slices.Equal(got, tc.want) {
					t.Fatalf("hook events = %v, want %v", got, tc.want)
				}

				for _, file := range files {
					f.write(tc.to + "/" + file)
				}
				got := f.changes()
				movedIn := !strings.HasPrefix(tc.to, "../")
				for _, file := range files {
					want := "U " + tc.to + "/" + file
					if movedIn && !slices.Contains(got, want) {
						t.Errorf("change in the moved folder did not reach the hook as %q (got %v)", want, got)
					}
				}
				for _, ev := range got {
					if !movedIn || strings.Contains(ev, " "+tc.from+"/") {
						t.Errorf("change in the moved folder reached the hook as %q", ev)
					}
				}

				if !movedIn {
					f.assertNoWatchUnder(tc.from)
				}
			})
		}
	}
}

// 감시 대상 밖으로 옮긴 폴더와 같은 경로에 폴더를 다시 만들면, 그 안에 새로 생긴 파일의 이벤트를 받는지 확인한다.
// fsnotify의 kqueue 구현에 옛 폴더 안 항목의 감시가 남으면 같은 이름의 새 파일은 생성 이벤트가 오지 않는다.
func TestFolderRecreatedAfterMoveOutIsWatched(t *testing.T) {
	f := newRenameFixture(t)
	f.write("src/d/f.txt")
	f.start(nil)
	f.move("src/d", "../outside/d")
	if got, want := f.changes(), []string{"D src/d/"}; !slices.Equal(got, want) {
		t.Fatalf("hook events = %v, want %v", got, want)
	}
	if err := os.Mkdir(f.path("src/d"), 0700); err != nil {
		t.Fatal(err)
	}
	if got, want := f.changes(), []string{"C src/d/"}; !slices.Equal(got, want) {
		t.Fatalf("hook events = %v, want %v", got, want)
	}
	f.write("src/d/f.txt")
	if got := f.changes(); !slices.Contains(got, "C src/d/f.txt") {
		t.Fatalf("creation in the recreated folder did not reach the hook (got %v)", got)
	}
}

// 옛 경로가 exclude에 맞으면 이름을 바꾸거나 옮겨도 hook이 삭제(D)를 받지 않는지 확인한다.
func TestRenameFromExcludedPathDoesNotNotifyDelete(t *testing.T) {
	f := newRenameFixture(t)
	f.write("src/a.tmp")
	f.write("src/b.tmp")
	f.start(domain.ExcludePatterns{"**/*.tmp"})
	f.move("src/a.tmp", "src/a.txt")
	f.move("src/b.tmp", "../outside/b.tmp")
	if got, want := f.changes(), []string{"C src/a.txt"}; !slices.Equal(got, want) {
		t.Fatalf("hook events = %v, want %v", got, want)
	}
}

// 감시 대상 밖으로 옮긴 폴더의 옛 경로에 곧바로 폴더를 다시 만들어도, 옮겨 간 폴더 안의 변경은 hook을 실행하지
// 않고 새 폴더 안의 변경만 hook을 실행하는지 확인한다. Windows에서 옮겨 간 폴더의 감시가 남으면 두 폴더의
// 변경이 모두 옛 경로로 온다.
func TestFolderRecreatedRightAfterMoveOutIgnoresMovedFolder(t *testing.T) {
	f := newRenameFixture(t)
	f.write("src/d/moved.txt")
	f.start(nil)
	f.move("src/d", "../outside/d")
	if err := os.Mkdir(f.path("src/d"), 0700); err != nil {
		t.Fatal(err)
	}
	// 같은 경로에 폴더가 이미 다시 생겼어도, 옛 폴더를 감시 목록에 적어 두었으므로 옛 폴더의 D가 온다.
	if got, want := f.changes(), []string{"D src/d/", "C src/d/"}; !slices.Equal(got, want) {
		t.Fatalf("hook events = %v, want %v", got, want)
	}
	f.write("../outside/d/moved.txt")
	f.write("../outside/d/other.txt")
	f.write("src/d/new.txt")
	got := f.changes()
	if !slices.Contains(got, "C src/d/new.txt") {
		t.Errorf("creation in the recreated folder did not reach the hook (got %v)", got)
	}
	for _, ev := range got {
		if strings.HasSuffix(ev, "/moved.txt") || strings.HasSuffix(ev, "/other.txt") {
			t.Errorf("change in the moved folder reached the hook as %q", ev)
		}
	}
	// 옮겨 간 폴더의 감시가 남으면 감시 목록에 옛 경로가 새 폴더와 함께 두 번 나온다.
	time.Sleep(3 * leakCheckDelay)
	f.changes()
	count := 0
	for _, p := range f.runner.tree.watcher.WatchList() {
		if filepath.Clean(p) == f.path("src/d") {
			count++
		}
	}
	if count != 1 {
		t.Errorf("watch list holds src/d %d times, want once (%v)", count, f.runner.tree.watcher.WatchList())
	}
}

// 이름 바꾸기가 다른 파일 쓰기와 함께 빠르게 이어져도, 이름을 바꾼 파일마다 hook이 옛 경로의 삭제(D)를 새 경로의
// 생성(C)보다 먼저 받는지 확인한다. kqueue(macOS)는 새 경로의 Create를 옛 경로의 Rename보다 수십 ms 먼저
// 알리기도 한다.
func TestManyRenamesNotifyDeleteBeforeCreate(t *testing.T) {
	const renamers, renamesEach = 4, 50
	f := newRenameFixture(t)
	for k := 0; k < renamers; k++ {
		for i := 0; i < renamesEach; i++ {
			f.write(fmt.Sprintf("src/a-%d-%d.txt", k, i))
		}
	}
	f.start(nil)
	// 받은 이벤트가 hook 쪽 채널을 채워 감시 루프의 대기열이 넘치지 않도록 계속 받아 둔다.
	var mu sync.Mutex
	var got []string
	go func() {
		for ev := range f.changed {
			rel, err := filepath.Rel(f.target, ev.Path.String())
			if err != nil {
				continue
			}
			mu.Lock()
			got = append(got, ev.Operation.String()+" "+filepath.ToSlash(rel))
			mu.Unlock()
		}
	}()
	seen := func(want string) bool {
		mu.Lock()
		defer mu.Unlock()
		return slices.Contains(got, want)
	}

	stopWriting := make(chan struct{})
	writing := make(chan struct{})
	go func() {
		defer close(writing)
		for n := 0; ; n++ {
			select {
			case <-stopWriting:
				return
			default:
			}
			_ = os.WriteFile(f.path("dst/noise.txt"), []byte(fmt.Sprint(n)), 0600)
			time.Sleep(time.Millisecond)
		}
	}()
	var renaming sync.WaitGroup
	for k := 0; k < renamers; k++ {
		renaming.Add(1)
		go func(k int) {
			defer renaming.Done()
			for i := 0; i < renamesEach; i++ {
				if err := os.Rename(f.path(fmt.Sprintf("src/a-%d-%d.txt", k, i)), f.path(fmt.Sprintf("src/b-%d-%d.txt", k, i))); err != nil {
					t.Error(err)
				}
			}
		}(k)
	}
	renaming.Wait()
	close(stopWriting)
	<-writing
	f.write("src/zz-sentinel.txt")
	deadline := time.After(10 * time.Second)
	for !seen("C src/zz-sentinel.txt") {
		select {
		case err := <-f.result:
			t.Fatalf("run stopped: %v", err)
		case <-deadline:
			t.Fatal("sentinel did not reach the hook")
		case <-time.After(10 * time.Millisecond):
		}
	}

	mu.Lock()
	defer mu.Unlock()
	position := map[string]int{}
	for i, ev := range got {
		position[ev] = i
	}
	for k := 0; k < renamers; k++ {
		for i := 0; i < renamesEach; i++ {
			d, okD := position[fmt.Sprintf("D src/a-%d-%d.txt", k, i)]
			c, okC := position[fmt.Sprintf("C src/b-%d-%d.txt", k, i)]
			if !okD || !okC || d > c {
				t.Errorf("rename %d-%d: D at %d (%v), C at %d (%v)", k, i, d, okD, c, okC)
			}
		}
	}
}

// 옛 경로의 Rename을 기다리다 보류 시간이 끝나 넘긴 Create가 watcher를 다시 만들어도, 예전 watcher가 닫히기 전에
// 받은 이벤트가 새 watcher가 받은 다음 이벤트보다 먼저 hook에 가는지 확인한다.
//
// 보류는 kqueue에서만, 감시를 풀지 못한 폴더는 주로 Windows에서 생기므로 두 상태를 직접 만든다. 감시 대상 밖
// 폴더의 감시를 남겨, 풀지 못한 폴더 d의 Create가 rebuildIfLeaked에서 watcher를 다시 만들게 한다.
func TestRetiredWatcherEventsPrecedeNextEventAfterHoldTimeout(t *testing.T) {
	target, outside := t.TempDir(), t.TempDir()
	dir := filepath.Join(target, "d")
	if err := os.Mkdir(dir, 0700); err != nil {
		t.Fatal(err)
	}
	option := watchTargetOption(domain.WatchOption{SubDir: true, FileEvent: domain.WatchEvent{Create: true}, DirEvent: domain.WatchEvent{Create: true}})
	// 폴더 이벤트를 모으지 않아, 보류를 푼 d의 C가 묶음을 기다리지 않고 곧바로 hook에 간다.
	option.DirBatch = domain.DirBatchOption{}
	r := NewWatchTargetRunner(domain.GlobalCommandSet{}, domain.WatchTargetsCommandSet{Path: domain.Path(target), Option: option}, newMockLogger())
	if err := r.tree.watcher.Add(outside); err != nil {
		t.Fatal(err)
	}
	retired, retiredErrors := controlledSource(r)
	holdTimeout := make(chan time.Time)
	r.held = []fsnotify.Event{{Name: dir, Op: fsnotify.Create}}
	r.renameSource = filepath.Join(target, "old")
	r.holdTimeout = holdTimeout
	r.tree.unreleasedDirs[dir] = struct{}{}
	changed, result := startRunner(t, r, nil)

	var got []string
	await := func(want string) {
		t.Helper()
		for !slices.Contains(got, want) {
			select {
			case ev := <-changed:
				if rel, err := filepath.Rel(target, ev.Path.String()); err == nil {
					got = append(got, filepath.ToSlash(rel))
				}
			case err := <-result:
				t.Fatalf("run stopped: %v", err)
			case <-time.After(5 * time.Second):
				t.Fatalf("%s did not reach the hook (got %v)", want, got)
			}
		}
	}
	holdTimeout <- time.Now()
	await("d")
	// 감시 루프가 예전 watcher의 이벤트를 새 watcher의 다음 이벤트 뒤에 받으면, 그때까지 보내기가 멈추므로 따로 보낸다.
	go func() {
		retired <- fsnotify.Event{Name: filepath.Join(target, "retired.txt"), Op: fsnotify.Create}
		close(retired)
		close(retiredErrors)
	}()
	if err := os.WriteFile(filepath.Join(target, "next.txt"), []byte("event"), 0600); err != nil {
		t.Fatal(err)
	}
	await("retired.txt")
	await("next.txt")
	if slices.Index(got, "retired.txt") > slices.Index(got, "next.txt") {
		t.Fatalf("event of the retired watcher reached the hook after the next event: %v", got)
	}
}
