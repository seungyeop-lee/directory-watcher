package service

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/seungyeop-lee/directory-watcher/v2/internal/app/domain"
)

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
				original, err := os.Getwd()
				if err != nil {
					t.Fatal(err)
				}
				if err := os.Chdir(work); err != nil {
					t.Fatal(err)
				}
				// Windows는 작업 디렉터리로 쓰는 폴더를 지우지 못하므로, TempDir 정리보다 먼저 되돌린다.
				t.Cleanup(func() {
					if err := os.Chdir(original); err != nil {
						t.Fatal(err)
					}
				})
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
