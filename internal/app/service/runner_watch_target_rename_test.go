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

	"github.com/seungyeop-lee/directory-watcher/v3/internal/app/domain"
)

// renameFixture는 감시 대상 target 안에 src와 dst 폴더를, target 밖에 outside 폴더를 둔다.
type renameFixture struct {
	t       *testing.T
	target  string
	outside string
	runner  *watchTargetRunner
	changed <-chan domain.Event
	result  <-chan error
	phase   int
}

func newRenameFixture(t *testing.T) *renameFixture {
	base := t.TempDir()
	f := &renameFixture{t: t, target: filepath.Join(base, "target"), outside: filepath.Join(base, "outside")}
	for _, dir := range []string{"target/src", "target/dst", "outside"} {
		if err := os.MkdirAll(filepath.Join(base, filepath.FromSlash(dir)), 0700); err != nil {
			t.Fatal(err)
		}
	}
	return f
}

// path는 감시 대상 기준 상대 경로 rel을 경로로 바꾼다. "../outside/"로 시작하면 감시 대상 밖 경로다.
func (f *renameFixture) path(rel string) string {
	return filepath.Join(f.target, filepath.FromSlash(rel))
}

func (f *renameFixture) write(rel string) {
	f.t.Helper()
	if err := os.MkdirAll(filepath.Dir(f.path(rel)), 0700); err != nil {
		f.t.Fatal(err)
	}
	if err := os.WriteFile(f.path(rel), []byte("event"), 0600); err != nil {
		f.t.Fatal(err)
	}
}

func (f *renameFixture) move(from, to string) {
	f.t.Helper()
	if err := os.Rename(f.path(from), f.path(to)); err != nil {
		f.t.Fatal(err)
	}
}

func (f *renameFixture) start(exclude domain.ExcludePatterns) {
	f.runner, f.changed, f.result = startExcludeRunner(f.t, f.target, domain.WatchTargetsOption{
		NoWait:      true,
		WatchSubDir: true,
		WatchEvent:  domain.WatchAllEvent(),
		Exclude:     exclude,
	}, newMockLogger(), nil)
}

// changes는 감시 대상의 모든 감시 폴더에 표지 파일을 쓰고, 그 표지들의 hook이 모두 실행될 때까지 hook이 받은
// 이벤트를 "D src/a.txt"처럼 돌려준다. 표지 파일의 이벤트는 빼고 돌려준다.
//
// Windows는 폴더마다 따로 이벤트를 받으므로, 한 폴더의 표지만 기다리면 다른 폴더의 이벤트가 그 뒤에 올 수 있다.
func (f *renameFixture) changes() []string {
	f.t.Helper()
	f.phase++
	sentinel := fmt.Sprintf("zz-sentinel-%d.txt", f.phase)
	waiting := map[string]bool{}
	for _, dir := range []string{"", "src", "dst"} {
		rel := filepath.ToSlash(filepath.Join(dir, sentinel))
		f.write(rel)
		waiting[rel] = true
	}
	var got []string
	for len(waiting) > 0 {
		select {
		case ev := <-f.changed:
			rel, err := filepath.Rel(f.target, ev.Path.String())
			if err != nil {
				f.t.Fatal(err)
			}
			rel = filepath.ToSlash(rel)
			if strings.HasPrefix(filepath.Base(rel), "zz-sentinel-") {
				delete(waiting, rel)
				continue
			}
			got = append(got, ev.Operation.String()+" "+rel)
		case err := <-f.result:
			f.t.Fatalf("run stopped: %v", err)
		case <-time.After(5 * time.Second):
			f.t.Fatalf("sentinels %v did not reach the hook (got %v)", waiting, got)
		}
	}
	return got
}

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

// 폴더의 이름을 바꾸거나 옮기면 hook이 옛 경로의 삭제(D)를 한 번 받고, 옮긴 폴더 안의 변경은 새 경로로만 받는지
// 확인한다. 감시 대상 밖으로 옮긴 폴더는 그 아래 폴더까지 감시가 풀려 옮겨 간 곳의 변경으로 hook이 실행되지 않는다.
func TestRenamedOrMovedFolderNotifiesOldPathAsDelete(t *testing.T) {
	cases := []struct {
		name     string
		from, to string
		want     []string
	}{
		{name: "rename in the same folder", from: "src/d", to: "src/e", want: []string{"D src/d"}},
		{name: "move to another folder in the target", from: "src/d", to: "dst/d", want: []string{"D src/d"}},
		{name: "move out of the target", from: "src/d", to: "../outside/d", want: []string{"D src/d"}},
		{name: "move into the target", from: "../outside/d", to: "src/d"},
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

// assertNoWatchUnder는 감시 목록에 rel과 그 아래 경로가 없는지 확인한다.
//
// Windows에서는 옮겨 나간 폴더의 감시를 leakCheckDelay 뒤에 watcher를 새로 만들어 버리므로, 그보다 오래 기다린
// 뒤 표지 파일의 hook이 실행될 때까지 기다린다. 감시 루프는 이벤트와 watcher 교체를 차례로 처리하므로, 표지
// 파일의 hook이 실행됐다면 교체도 끝났고 runner.watcher를 읽어도 된다.
func (f *renameFixture) assertNoWatchUnder(rel string) {
	f.t.Helper()
	time.Sleep(3 * leakCheckDelay)
	f.changes()
	for _, p := range f.runner.watcher.WatchList() {
		if domain.Path(f.path(rel)).Contains(domain.Path(p)) {
			f.t.Errorf("watch list still holds %s under the old path %s", p, rel)
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
	if got, want := f.changes(), []string{"D src/d"}; !slices.Equal(got, want) {
		t.Fatalf("hook events = %v, want %v", got, want)
	}
	if err := os.Mkdir(f.path("src/d"), 0700); err != nil {
		t.Fatal(err)
	}
	f.changes()
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
	// 옛 폴더의 D는 같은 경로에 폴더가 이미 다시 생겼으면 오지 않는다. IsListening이 그 경로를 폴더로 보고 버린다.
	f.changes()
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
	for _, p := range f.runner.watcher.WatchList() {
		if filepath.Clean(p) == f.path("src/d") {
			count++
		}
	}
	if count != 1 {
		t.Errorf("watch list holds src/d %d times, want once (%v)", count, f.runner.watcher.WatchList())
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
