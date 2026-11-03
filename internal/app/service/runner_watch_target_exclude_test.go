package service

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/seungyeop-lee/directory-watcher/v3/internal/app/domain"
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
				Option: domain.WatchTargetsOption{NoWait: true, WatchSubDir: true, WatchEvent: domain.WatchEvent{Create: true}},
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
					switch got.FileName() {
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

// 빈 경로나 감시 대상 밖 경로의 이벤트는 hook을 실행하지 않고, 대상 밖 폴더를 감시 목록에 넣지도 않는지 확인한다.
func TestEventOutsideWatchTargetIsDropped(t *testing.T) {
	target, outside := t.TempDir(), t.TempDir()
	outsideDir := filepath.Join(outside, "dir")
	if err := os.Mkdir(outsideDir, 0700); err != nil {
		t.Fatal(err)
	}
	r := NewWatchTargetRunner(domain.GlobalCommandSet{}, domain.WatchTargetsCommandSet{
		Path:   domain.Path(target),
		Option: domain.WatchTargetsOption{NoWait: true, WatchSubDir: true, WatchEvent: domain.WatchEvent{Create: true, Update: true, Delete: true}},
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
