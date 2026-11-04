package service

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/seungyeop-lee/directory-watcher/v3/internal/app/domain"
	"github.com/seungyeop-lee/directory-watcher/v3/internal/helper"
)

// chdir는 작업 디렉터리를 dir로 옮기고 테스트가 끝나면 되돌린다. Windows는 작업 디렉터리로 쓰는 폴더를
// 지우지 못하므로, dir은 이 함수를 부르기 전에 만든 TempDir 안에 있어야 TempDir 정리보다 먼저 되돌려진다.
func chdir(t *testing.T, dir string) {
	original, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(original); err != nil {
			t.Fatal(err)
		}
	})
}

func TestDefaultExcludeChecksCreatedFileName(t *testing.T) {
	tests := []struct {
		name      string
		watchPath func(t *testing.T) string
	}{
		{
			name:      "absolute path",
			watchPath: func(t *testing.T) string { return t.TempDir() },
		},
		{
			name: "relative path starting with ..",
			watchPath: func(t *testing.T) string {
				base := t.TempDir()
				work, src := filepath.Join(base, "work"), filepath.Join(base, "src")
				for _, dir := range []string{work, src} {
					if err := os.Mkdir(dir, 0700); err != nil {
						t.Fatal(err)
					}
				}
				chdir(t, work)
				return filepath.Join("..", "src")
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := tt.watchPath(t)
			r := NewWatchTargetRunner(domain.GlobalCommandSet{}, domain.WatchTargetsCommandSet{
				Path:   domain.Path(path),
				Option: domain.WatchTargetsOption{Deliver: domain.DeliverEach, DirEventWait: 100, DirEventMaxWait: 1000, WatchSubDir: true, WatchFileEvent: domain.WatchEvent{Create: true}},
			}, newMockLogger())
			t.Cleanup(func() {
				var wg sync.WaitGroup
				wg.Add(1)
				r.Stop(&wg)
			})
			ready, changed := make(chan struct{}), make(chan domain.Path, 8)
			r.runStartHook = func() { close(ready) }
			r.runChangeHook = func(event domain.Event) { changed <- event.Path }
			r.runFinishHook = func() {}
			result := make(chan error, 1)
			go func() { result <- r.Run() }()
			select {
			case <-ready:
			case err := <-result:
				t.Fatalf("run before registration: %v", err)
			case <-time.After(5 * time.Second):
				t.Fatal("native watcher did not become ready")
			}

			// 제외된 이벤트가 오지 않았음을 시간 초과만으로 판단하지 않도록,
			// 제외 대상을 먼저 만들고 일반 파일의 이벤트가 도착할 때까지 받은 경로를 확인한다.
			for _, name := range []string{".hidden.txt", "_draft.txt", "plain.txt"} {
				if err := os.WriteFile(filepath.Join(path, name), []byte("event"), 0600); err != nil {
					t.Fatal(err)
				}
			}
			for {
				select {
				case got := <-changed:
					switch got.Name() {
					case "plain.txt":
						return
					case ".hidden.txt", "_draft.txt":
						t.Fatalf("create of default-excluded file ran the hook: %s", got)
					}
				case <-time.After(5 * time.Second):
					t.Fatal("create of plain.txt did not reach the hook")
				}
			}
		})
	}
}

func TestDefaultExcludeChecksModifiedAndRemovedName(t *testing.T) {
	tests := []struct {
		name      string
		watchPath func(t *testing.T) string
	}{
		{
			name:      "plain watch path",
			watchPath: func(t *testing.T) string { return t.TempDir() },
		},
		{
			name: "watch path named with a dot",
			watchPath: func(t *testing.T) string {
				path := filepath.Join(t.TempDir(), ".watched")
				if err := os.Mkdir(path, 0700); err != nil {
					t.Fatal(err)
				}
				return path
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := tt.watchPath(t)
			for _, name := range []string{".env", "_draft.txt", "plain.txt"} {
				if err := os.WriteFile(filepath.Join(path, name), []byte("event"), 0600); err != nil {
					t.Fatal(err)
				}
			}
			if err := os.Mkdir(filepath.Join(path, ".cache"), 0700); err != nil {
				t.Fatal(err)
			}
			r := NewWatchTargetRunner(domain.GlobalCommandSet{}, domain.WatchTargetsCommandSet{
				Path:   domain.Path(path),
				Option: domain.WatchTargetsOption{Deliver: domain.DeliverEach, DirEventWait: 100, DirEventMaxWait: 1000, WatchSubDir: true, WatchFileEvent: domain.WatchAllEvent()},
			}, newMockLogger())
			t.Cleanup(func() {
				var wg sync.WaitGroup
				wg.Add(1)
				r.Stop(&wg)
			})
			ready, changed := make(chan struct{}), make(chan domain.Event, 32)
			r.runStartHook = func() { close(ready) }
			r.runChangeHook = func(event domain.Event) { changed <- event }
			r.runFinishHook = func() {}
			result := make(chan error, 1)
			go func() { result <- r.Run() }()
			select {
			case <-ready:
			case err := <-result:
				t.Fatalf("run before registration: %v", err)
			case <-time.After(5 * time.Second):
				t.Fatal("native watcher did not become ready")
			}

			// 제외된 이벤트가 오지 않았음을 시간 초과만으로 판단하지 않도록, 제외 대상을 먼저 바꾸고
			// 일반 파일의 수정과 삭제가 hook에 도착할 때까지 받은 이벤트를 확인한다.
			for _, name := range []string{".env", "_draft.txt"} {
				appendFile(t, filepath.Join(path, name))
			}
			for _, name := range []string{".env", "_draft.txt", ".cache"} {
				if err := os.RemoveAll(filepath.Join(path, name)); err != nil {
					t.Fatal(err)
				}
			}
			plain := filepath.Join(path, "plain.txt")
			appendFile(t, plain)
			waitPlainEvent(t, changed, fsnotify.Write)
			if err := os.Remove(plain); err != nil {
				t.Fatal(err)
			}
			waitPlainEvent(t, changed, fsnotify.Remove)
		})
	}
}

func appendFile(t *testing.T, name string) {
	t.Helper()
	f, err := os.OpenFile(name, os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if _, err := f.WriteString("y\n"); err != nil {
		t.Fatal(err)
	}
}

// waitPlainEvent는 plain.txt에서 op 이벤트가 hook에 도착할 때까지 기다리며, 그 사이에 기본 제외 대상의
// 이벤트로 hook이 실행되면 실패한다.
func waitPlainEvent(t *testing.T, changed <-chan domain.Event, op fsnotify.Op) {
	t.Helper()
	for {
		select {
		case got := <-changed:
			if got.Path.IsDefaultExcludeFile() {
				t.Fatalf("%s of default-excluded name ran the hook: %s", got.Operation, got.Path)
			}
			if got.Path.Name() == "plain.txt" && fsnotify.Op(got.Operation).Has(op) {
				return
			}
		case <-time.After(5 * time.Second):
			t.Fatalf("%s of plain.txt did not reach the hook", op)
		}
	}
}

// 빈 경로나 감시 대상 밖 경로의 이벤트는 hook을 실행하지 않고, 대상 밖 폴더를 감시 목록에 넣지도 않는지 확인한다.
func TestEventOutsideWatchTargetIsDropped(t *testing.T) {
	target, outside := t.TempDir(), t.TempDir()
	outsideDir := filepath.Join(outside, "dir")
	if err := os.Mkdir(outsideDir, 0700); err != nil {
		t.Fatal(err)
	}
	r := NewWatchTargetRunner(domain.GlobalCommandSet{}, domain.WatchTargetsCommandSet{
		Path:   domain.Path(target),
		Option: domain.WatchTargetsOption{Deliver: domain.DeliverEach, DirEventWait: 100, DirEventMaxWait: 1000, WatchSubDir: true, WatchFileEvent: domain.WatchEvent{Create: true, Update: true, Delete: true}},
	}, newMockLogger())
	t.Cleanup(func() {
		var wg sync.WaitGroup
		wg.Add(1)
		r.Stop(&wg)
	})
	events, _ := controlledSource(r)
	changed := make(chan domain.Path, 8)
	r.runStartHook = func() {}
	r.runChangeHook = func(event domain.Event) { changed <- event.Path }
	r.runFinishHook = func() {}
	result := make(chan error, 1)
	go func() { result <- r.Run() }()

	for _, ev := range []fsnotify.Event{
		{Name: "", Op: fsnotify.Write},
		{Name: "", Op: fsnotify.Create},
		{Name: outsideDir, Op: fsnotify.Create},
		{Name: filepath.Join(outside, "a.txt"), Op: fsnotify.Write},
		{Name: filepath.Join(outside, "b.txt"), Op: fsnotify.Remove},
		{Name: filepath.Join("..", "outside", "a.txt"), Op: fsnotify.Write},
	} {
		events <- ev
	}
	// 감시 루프와 hook은 이벤트를 받은 순서대로 처리하므로, 뒤이어 넣은 대상 안 이벤트의 hook이
	// 실행됐다면 앞서 넣은 이벤트의 처리도 끝났다.
	inside := filepath.Join(target, "in.txt")
	events <- fsnotify.Event{Name: inside, Op: fsnotify.Create}
	select {
	case got := <-changed:
		if got.String() != inside {
			t.Fatalf("event outside the watch target ran the hook: %q", got)
		}
	case err := <-result:
		t.Fatalf("run stopped: %v", err)
	case <-time.After(5 * time.Second):
		t.Fatal("event inside the watch target did not reach the hook")
	}
	if got := r.watcher.WatchList(); len(got) != 1 || got[0] != target {
		t.Fatalf("watch list changed: %v", got)
	}
}

// startExcludeRunner는 path를 감시하는 runner를 시작하고, 감시 목록 등록이 끝나면 돌려준다.
// changed는 hook이 받은 이벤트를, result는 Run의 결과를 받는다.
func startExcludeRunner(t *testing.T, path string, option domain.WatchTargetsOption, logger helper.Logger, onChange func(domain.Event)) (r *watchTargetRunner, changed <-chan domain.Event, result <-chan error) {
	t.Helper()
	r = NewWatchTargetRunner(domain.GlobalCommandSet{}, domain.WatchTargetsCommandSet{Path: domain.Path(path), Option: option}, logger)
	t.Cleanup(func() {
		var wg sync.WaitGroup
		wg.Add(1)
		r.Stop(&wg)
	})
	ready, events, done := make(chan struct{}), make(chan domain.Event, dispatchCapacity), make(chan error, 1)
	r.runStartHook = func() { close(ready) }
	r.runChangeHook = func(event domain.Event) {
		if onChange != nil {
			onChange(event)
		}
		events <- event
	}
	r.runFinishHook = func() {}
	go func() { done <- r.Run() }()
	select {
	case <-ready:
	case err := <-done:
		t.Fatalf("run before registration: %v", err)
	case <-time.After(5 * time.Second):
		t.Fatal("native watcher did not become ready")
	}
	return r, events, done
}

// awaitHookPath는 hook이 path의 이벤트를 받을 때까지 기다리며, 그 사이에 받은 이벤트의 경로를 돌려준다.
func awaitHookPath(t *testing.T, changed <-chan domain.Event, result <-chan error, path string) []string {
	t.Helper()
	var seen []string
	for {
		select {
		case got := <-changed:
			seen = append(seen, got.Path.String())
			if got.Path.String() == path {
				return seen
			}
		case err := <-result:
			t.Fatalf("run stopped: %v", err)
		case <-time.After(5 * time.Second):
			t.Fatalf("change of %s did not reach the hook (seen %v)", path, seen)
		}
	}
}

// exclude에 맞는 폴더와 그 아래 폴더는 감시 시작 때 있었든 감시 중에 생겼든 감시 목록에 들어가지 않는지 확인한다.
func TestExcludedFolderStaysOutOfWatchList(t *testing.T) {
	exclude := domain.ExcludePatterns{"dist/**", "**/node_modules/**"}
	excludedDirs := []string{"dist", "dist/sub", "app/node_modules", "app/node_modules/pkg"}
	watchedDirs := []string{"keep", "keep/sub", "app"}
	for _, atStart := range []bool{true, false} {
		name := "created while watching"
		if atStart {
			name = "existing at start"
		}
		t.Run(name, func(t *testing.T) {
			target := t.TempDir()
			makeDirs := func() {
				for _, dir := range []string{"dist/sub", "app/node_modules/pkg", "keep/sub"} {
					if err := os.MkdirAll(filepath.Join(target, dir), 0700); err != nil {
						t.Fatal(err)
					}
				}
			}
			if atStart {
				makeDirs()
			}
			r, changed, result := startExcludeRunner(t, target, domain.WatchTargetsOption{
				Deliver:         domain.DeliverEach,
				DirEventWait:    100,
				DirEventMaxWait: 1000,
				WatchSubDir:     true,
				WatchFileEvent:  domain.WatchEvent{Create: true},
				Exclude:         exclude,
			}, newMockLogger(), nil)
			if !atStart {
				makeDirs()
			}
			// 감시 루프는 이벤트를 받은 순서대로 처리하므로, 뒤이어 만든 파일의 hook이 실행됐다면 앞서 생긴 폴더의
			// 감시 목록 반영도 끝났다. 가장 깊은 감시 폴더 안의 파일 hook까지 실행되면 감시 목록 반영이 모두 끝났다.
			for _, sentinel := range []string{filepath.Join(target, "zz-registered.txt"), filepath.Join(target, "keep", "sub", "zz-registered.txt")} {
				if err := os.WriteFile(sentinel, []byte("event"), 0600); err != nil {
					t.Fatal(err)
				}
				awaitHookPath(t, changed, result, sentinel)
			}

			watchList := map[string]bool{}
			for _, p := range r.watcher.WatchList() {
				watchList[p] = true
			}
			for _, dir := range excludedDirs {
				if watchList[filepath.Join(target, dir)] {
					t.Errorf("excluded folder %s is in the watch list", dir)
				}
			}
			for _, dir := range watchedDirs {
				if !watchList[filepath.Join(target, dir)] {
					t.Errorf("folder %s is not in the watch list", dir)
				}
			}
		})
	}
}

// exclude에 맞는 파일은 생성, 수정, 삭제 모두 hook을 실행하지 않고, 맞지 않는 파일은 hook을 실행하는지 확인한다.
func TestExcludePatternDropsFileEvents(t *testing.T) {
	cases := []struct {
		name     string
		exclude  domain.ExcludePatterns
		excluded []string
		watched  []string
	}{
		{name: "file name", exclude: domain.ExcludePatterns{"out.txt"}, excluded: []string{"out.txt"}, watched: []string{"layout.txt", "sub/out.txt"}},
		{name: "extension", exclude: domain.ExcludePatterns{"**/*.log"}, excluded: []string{"a.log", "sub/b.log"}, watched: []string{"a.txt"}},
		{name: "pattern with a sub path", exclude: domain.ExcludePatterns{"build/**/*.o"}, excluded: []string{"build/x.o", "build/sub/y.o"}, watched: []string{"build/x.c", "src/x.o"}},
		{name: "negation", exclude: domain.ExcludePatterns{"**/*.log", "!important.log"}, excluded: []string{"a.log"}, watched: []string{"important.log"}},
		{name: "negation inside a watched folder", exclude: domain.ExcludePatterns{"dist/*", "!dist/keep.txt"}, excluded: []string{"dist/a.txt"}, watched: []string{"dist/keep.txt"}},
		{name: "negation inside an excluded folder", exclude: domain.ExcludePatterns{"dist/**", "!dist/keep.txt"}, excluded: []string{"dist/a.txt", "dist/keep.txt"}},
	}
	ops := []fsnotify.Op{fsnotify.Create, fsnotify.Write, fsnotify.Remove}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			target := t.TempDir()
			r := NewWatchTargetRunner(domain.GlobalCommandSet{}, domain.WatchTargetsCommandSet{
				Path:   domain.Path(target),
				Option: domain.WatchTargetsOption{Deliver: domain.DeliverEach, DirEventWait: 100, DirEventMaxWait: 1000, WatchSubDir: true, WatchFileEvent: domain.WatchAllEvent(), Exclude: tc.exclude},
			}, newMockLogger())
			t.Cleanup(func() {
				var wg sync.WaitGroup
				wg.Add(1)
				r.Stop(&wg)
			})
			events, _ := controlledSource(r)
			changed := make(chan domain.Event, dispatchCapacity)
			r.runStartHook = func() {}
			r.runChangeHook = func(event domain.Event) { changed <- event }
			r.runFinishHook = func() {}
			result := make(chan error, 1)
			go func() { result <- r.Run() }()

			type change struct {
				rel string
				op  fsnotify.Op
			}
			var want []change
			for _, rel := range append(append([]string{}, tc.excluded...), tc.watched...) {
				for _, op := range ops {
					events <- fsnotify.Event{Name: filepath.Join(target, filepath.FromSlash(rel)), Op: op}
				}
			}
			for _, rel := range tc.watched {
				for _, op := range ops {
					want = append(want, change{rel, op})
				}
			}
			// 감시 루프와 hook은 이벤트를 받은 순서대로 처리하므로, 뒤이어 넣은 이벤트의 hook이 실행됐다면
			// 앞서 넣은 이벤트의 처리도 끝났다.
			sentinel := filepath.Join(target, "zz-sentinel.txt")
			events <- fsnotify.Event{Name: sentinel, Op: fsnotify.Create}
			var got []change
			for done := false; !done; {
				select {
				case event := <-changed:
					if event.Path.String() == sentinel {
						done = true
						break
					}
					rel, _ := filepath.Rel(target, event.Path.String())
					got = append(got, change{filepath.ToSlash(rel), fsnotify.Op(event.Operation)})
				case err := <-result:
					t.Fatalf("run stopped: %v", err)
				case <-time.After(5 * time.Second):
					t.Fatal("sentinel event did not reach the hook")
				}
			}
			if !slices.Equal(got, want) {
				t.Fatalf("hook ran for %v, want %v", got, want)
			}
		})
	}
}

// hook이 감시 대상 안의 out.txt에 써도 exclude에 out.txt를 적었으면 그 쓰기로 hook이 다시 실행되지 않고,
// hook 실행 중에 바뀐 다른 파일은 hook이 끝난 뒤 처리되는지 확인한다.
func TestHookOutputInExcludedPathDoesNotRerunHook(t *testing.T) {
	target := t.TempDir()
	out, a, b, c := filepath.Join(target, "out.txt"), filepath.Join(target, "a.txt"), filepath.Join(target, "b.txt"), filepath.Join(target, "c.txt")
	logger := &eventLogger{lines: make(chan string, 64)}
	release := make(chan struct{})
	var once sync.Once
	var mu sync.Mutex
	var runs []string // hook이 시작하고 끝난 순서
	_, changed, result := startExcludeRunner(t, target, domain.WatchTargetsOption{
		Deliver:         domain.DeliverEach,
		DirEventWait:    100,
		DirEventMaxWait: 1000,
		WatchSubDir:     true,
		WatchFileEvent:  domain.WatchAllEvent(),
		Exclude:         domain.ExcludePatterns{"out.txt"},
	}, logger, func(event domain.Event) {
		mu.Lock()
		runs = append(runs, "start "+event.Path.Name())
		mu.Unlock()
		f, err := os.OpenFile(out, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
		if err == nil {
			_, err = f.WriteString(event.Path.Name() + "\n")
			f.Close()
		}
		if err != nil {
			t.Error(err)
		}
		once.Do(func() { <-release })
		mu.Lock()
		runs = append(runs, "end "+event.Path.Name())
		mu.Unlock()
	})

	if err := os.WriteFile(a, []byte("event"), 0600); err != nil {
		t.Fatal(err)
	}
	// 첫 hook이 끝나지 않은 동안 b.txt를 바꾸고, 감시 루프가 b.txt의 이벤트를 받은 뒤에 hook을 끝낸다.
	logger.await(t, "out.txt")
	if err := os.WriteFile(b, []byte("event"), 0600); err != nil {
		t.Fatal(err)
	}
	logger.await(t, "b.txt")
	close(release)
	seen := awaitHookPath(t, changed, result, b)
	if err := os.WriteFile(c, []byte("event"), 0600); err != nil {
		t.Fatal(err)
	}
	seen = append(seen, awaitHookPath(t, changed, result, c)...)

	for _, p := range seen {
		if p == out {
			t.Fatalf("hook output in out.txt ran the hook (hook paths %v)", seen)
		}
	}
	mu.Lock()
	defer mu.Unlock()
	firstEnd, bStart := slices.Index(runs, "end a.txt"), slices.Index(runs, "start b.txt")
	if firstEnd < 0 || bStart < firstEnd {
		t.Fatalf("change of b.txt was not handled after the running hook ended: %v", runs)
	}
}

// eventLogger는 감시 루프가 받은 이벤트의 DEBUG 로그를 lines로 보낸다.
type eventLogger struct{ lines chan string }

func (l *eventLogger) Info(string)  {}
func (l *eventLogger) Error(string) {}
func (l *eventLogger) Debug(msg string) {
	if strings.HasPrefix(msg, "event: ") {
		l.lines <- msg
	}
}

// await는 name이 든 이벤트 로그가 올 때까지 기다린다.
func (l *eventLogger) await(t *testing.T, name string) {
	t.Helper()
	for {
		select {
		case line := <-l.lines:
			if strings.Contains(line, name) {
				return
			}
		case <-time.After(5 * time.Second):
			t.Fatalf("watcher did not receive an event of %s", name)
		}
	}
}
