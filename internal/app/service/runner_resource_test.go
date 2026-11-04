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
		t.Run(fmt.Sprintf("subDir=%t", recursive), func(t *testing.T) {
			path := t.TempDir()
			if err := os.Mkdir(filepath.Join(path, "child"), 0700); err != nil {
				t.Fatal(err)
			}
			r := NewWatchTargetRunner(domain.GlobalCommandSet{}, domain.WatchTargetsCommandSet{Path: domain.Path(path), Option: watchTargetOption(domain.WatchOption{SubDir: recursive, FileEvent: domain.WatchEvent{Create: true}})}, newMockLogger())
			assertWatchDirectoriesRemovable(t, r)
			changed, result := startRunner(t, r, nil)
			// finish hook은 Stop을 부른 고루틴이 실행하므로, 감시를 시작한 뒤에 바꿔도 Run과 겹치지 않는다.
			finished := make(chan struct{})
			r.runFinishHook = func() { close(finished) }
			if err := os.WriteFile(filepath.Join(path, "actual.txt"), []byte("event"), 0600); err != nil {
				t.Fatal(err)
			}
			select {
			case <-changed:
			case <-time.After(5 * time.Second):
				t.Fatal("native filesystem event did not reach the hook")
			}
			var wg sync.WaitGroup
			wg.Add(1)
			r.Stop(&wg)
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

func TestSubDirControlsExistingSubdirectoryEvents(t *testing.T) {
	for _, subDir := range []bool{false, true} {
		t.Run(fmt.Sprintf("subDir=%t", subDir), func(t *testing.T) {
			path := t.TempDir()
			child := filepath.Join(path, "child")
			if err := os.Mkdir(child, 0700); err != nil {
				t.Fatal(err)
			}
			r := NewWatchTargetRunner(domain.GlobalCommandSet{}, domain.WatchTargetsCommandSet{Path: domain.Path(path), Option: watchTargetOption(domain.WatchOption{SubDir: subDir, FileEvent: domain.WatchEvent{Create: true}})}, newMockLogger())
			assertWatchDirectoriesRemovable(t, r)
			changed, _ := startRunner(t, r, nil)

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
			for !sentinelSeen || (subDir && !nestedSeen) {
				select {
				case got := <-changed:
					nestedSeen = nestedSeen || got.Path.String() == nested
					sentinelSeen = sentinelSeen || got.Path.String() == sentinel
				case <-time.After(5 * time.Second):
					t.Fatalf("events did not reach the hook: nested=%t sentinel=%t", nestedSeen, sentinelSeen)
				}
			}
			if !subDir && nestedSeen {
				t.Fatal("change in existing subdirectory ran the hook with subDir=false")
			}
		})
	}
}
