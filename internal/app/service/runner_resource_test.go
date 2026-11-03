package service

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/seungyeop-lee/directory-watcher/v3/internal/app/domain"
)

func TestNativeWatcherStopReleasesDirectory(t *testing.T) {
	for _, recursive := range []bool{false, true} {
		t.Run(fmt.Sprintf("watchSubDir=%t", recursive), func(t *testing.T) {
			path := t.TempDir()
			if err := os.Mkdir(filepath.Join(path, "child"), 0700); err != nil {
				t.Fatal(err)
			}
			r := NewWatchTargetRunner(domain.GlobalCommandSet{}, domain.WatchTargetsCommandSet{
				Path:   domain.Path(path),
				Option: domain.WatchTargetsOption{NoWait: true, WatchSubDir: recursive, WatchEvent: domain.WatchEvent{Create: true}},
			}, newMockLogger())
			assertWatchDirectoriesRemovable(t, r)
			stop := func() {
				var wg sync.WaitGroup
				wg.Add(1)
				r.Stop(&wg)
			}
			t.Cleanup(stop)
			ready, changed, finished := make(chan struct{}), make(chan struct{}, 8), make(chan struct{})
			r.runStartHook = func() { close(ready) }
			r.runChangeHook = func(domain.Event) { changed <- struct{}{} }
			r.runFinishHook = func() { close(finished) }
			result := make(chan error, 1)
			go func() { result <- r.Run() }()
			select {
			case <-ready:
			case err := <-result:
				t.Fatalf("run before registration: %v", err)
			case <-time.After(5 * time.Second):
				t.Fatal("native watcher did not become ready")
			}
			if err := os.WriteFile(filepath.Join(path, "actual.txt"), []byte("event"), 0600); err != nil {
				t.Fatal(err)
			}
			select {
			case <-changed:
			case <-time.After(5 * time.Second):
				t.Fatal("native filesystem event did not reach the hook")
			}
			stop()
			if err := <-result; err != nil {
				t.Fatalf("normal stop: %v", err)
			}
			select {
			case <-finished:
			default:
				t.Fatal("Stop returned before finish hook")
			}
		})
	}
}

func TestWatchSubDirControlsExistingSubdirectoryEvents(t *testing.T) {
	for _, watchSubDir := range []bool{false, true} {
		t.Run(fmt.Sprintf("watchSubDir=%t", watchSubDir), func(t *testing.T) {
			path := t.TempDir()
			child := filepath.Join(path, "child")
			if err := os.Mkdir(child, 0700); err != nil {
				t.Fatal(err)
			}
			r := NewWatchTargetRunner(domain.GlobalCommandSet{}, domain.WatchTargetsCommandSet{
				Path:   domain.Path(path),
				Option: domain.WatchTargetsOption{NoWait: true, WatchSubDir: watchSubDir, WatchEvent: domain.WatchEvent{Create: true}},
			}, newMockLogger())
			assertWatchDirectoriesRemovable(t, r)
			stop := func() {
				var wg sync.WaitGroup
				wg.Add(1)
				r.Stop(&wg)
			}
			t.Cleanup(stop)
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

			nested := filepath.Join(child, "nested.txt")
			if err := os.WriteFile(nested, []byte("event"), 0600); err != nil {
				t.Fatal(err)
			}
			// 하위 디렉터리 이벤트가 오지 않았음을 시간 초과만으로 판단하지 않도록,
			// 이어서 루트에 파일을 만들고 그 이벤트가 도착할 때까지 받은 경로를 확인한다.
			sentinel := filepath.Join(path, "sentinel.txt")
			if err := os.WriteFile(sentinel, []byte("event"), 0600); err != nil {
				t.Fatal(err)
			}
			nestedSeen, sentinelSeen := false, false
			for !sentinelSeen || (watchSubDir && !nestedSeen) {
				select {
				case got := <-changed:
					nestedSeen = nestedSeen || got.String() == nested
					sentinelSeen = sentinelSeen || got.String() == sentinel
				case <-time.After(5 * time.Second):
					t.Fatalf("events did not reach the hook: nested=%t sentinel=%t", nestedSeen, sentinelSeen)
				}
			}
			if !watchSubDir && nestedSeen {
				t.Fatal("change in existing subdirectory ran the hook with watchSubDir=false")
			}
		})
	}
}
