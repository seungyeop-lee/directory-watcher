package service

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/seungyeop-lee/directory-watcher/v2/internal/app/domain"
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
