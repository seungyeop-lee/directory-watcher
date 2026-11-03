package service

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/seungyeop-lee/directory-watcher/v2/internal/app/domain"
)

// 폴더가 감시 시작 전에 있었는지, 감시 중에 만들어졌는지, 감시 중에 통째로 옮겨 왔는지와
// 관계없이 같은 규칙으로 감시되는지 확인한다.
func TestSubDirWatchRuleIsSameAtStartAndWhileWatching(t *testing.T) {
	cases := []struct {
		name       string
		dirs       []string // 감시 대상 안에 만들 폴더
		link       string   // 감시 대상 밖 폴더를 가리키는 링크의 이름
		excludeDir []string
		probe      string // 내용을 쓸 파일
		want       bool   // probe의 변경으로 hook이 실행되어야 하는지
	}{
		{name: "plain nested folder", dirs: []string{"plain/sub"}, probe: "plain/sub/a.txt", want: true},
		{name: "default-excluded folder", dirs: []string{".cache/sub"}, probe: ".cache/sub/a.txt", want: false},
		{name: "default-excluded nested folder", dirs: []string{"plain/_tmp"}, probe: "plain/_tmp/a.txt", want: false},
		{name: "excludeDir sibling with same prefix", dirs: []string{"dir1", "dir10"}, excludeDir: []string{"dir1"}, probe: "dir10/a.txt", want: true},
		{name: "excludeDir subfolder", dirs: []string{"parent/sub/deep"}, excludeDir: []string{"parent/sub"}, probe: "parent/sub/deep/a.txt", want: false},
		{name: "symlink to folder", link: "link", probe: "link/a.txt", want: false},
	}
	timings := []struct {
		name string
		// 폴더를 만드는 시점과 방법이다. start는 감시 시작 전에, 나머지는 감시 중에 만든다.
		start bool
		moved bool
	}{
		{name: "existing at start", start: true},
		{name: "created while watching"},
		{name: "moved in while watching", moved: true},
	}
	for _, tc := range cases {
		for _, timing := range timings {
			t.Run(tc.name+"/"+timing.name, func(t *testing.T) {
				base := t.TempDir()
				target, staging, outside := filepath.Join(base, "target"), filepath.Join(base, "staging"), filepath.Join(base, "outside")
				for _, dir := range []string{target, staging, outside} {
					if err := os.Mkdir(dir, 0700); err != nil {
						t.Fatal(err)
					}
				}
				build := func(root string) {
					for _, dir := range tc.dirs {
						if err := os.MkdirAll(filepath.Join(root, dir), 0700); err != nil {
							t.Fatal(err)
						}
					}
					if tc.link != "" {
						if err := os.Symlink(outside, filepath.Join(root, tc.link)); err != nil {
							t.Skipf("symlink not available: %v", err)
						}
					}
				}
				if timing.start {
					build(target)
				}

				var excludeDir domain.Paths
				for _, dir := range tc.excludeDir {
					excludeDir = append(excludeDir, domain.Path(filepath.Join(target, dir)))
				}
				r := NewWatchTargetRunner(domain.GlobalCommandSet{}, domain.WatchTargetsCommandSet{
					Path: domain.Path(target),
					Option: domain.WatchTargetsOption{
						NoWait:      true,
						WatchSubDir: true,
						ExcludeDir:  excludeDir,
						WatchEvent:  domain.WatchEvent{Create: true, Update: true},
					},
				}, newMockLogger())
				t.Cleanup(func() {
					var wg sync.WaitGroup
					wg.Add(1)
					r.Stop(&wg)
				})
				ready, changed := make(chan struct{}), make(chan domain.Path, dispatchCapacity)
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

				seen := map[string]bool{}
				write := func(rel string) {
					if err := os.WriteFile(filepath.Join(target, rel), []byte("event"), 0600); err != nil {
						t.Fatal(err)
					}
				}
				await := func(rel string) {
					for !seen[rel] {
						select {
						case got := <-changed:
							gotRel, err := filepath.Rel(target, got.String())
							if err != nil {
								t.Fatal(err)
							}
							// fsnotify의 kqueue 구현(macOS)은 감시 목록과 관계없이 감시 중인 폴더 안의 링크를
							// 스스로 따라가, 링크가 가리키는 감시 대상 밖 경로로 이벤트를 알린다. 이 이벤트는
							// 감시 목록 규칙의 결과가 아니므로 비교에서 뺀다.
							if gotRel == ".." || strings.HasPrefix(gotRel, ".."+string(filepath.Separator)) {
								continue
							}
							seen[filepath.ToSlash(gotRel)] = true
						case err := <-result:
							t.Fatalf("run stopped: %v", err)
						case <-time.After(5 * time.Second):
							t.Fatalf("change of %s did not reach the hook", rel)
						}
					}
				}

				if !timing.start {
					if timing.moved {
						build(staging)
						entries, err := os.ReadDir(staging)
						if err != nil {
							t.Fatal(err)
						}
						for _, entry := range entries {
							if err := os.Rename(filepath.Join(staging, entry.Name()), filepath.Join(target, entry.Name())); err != nil {
								t.Fatal(err)
							}
						}
					} else {
						build(target)
					}
				}
				// 감시 루프는 이벤트를 받은 순서대로 처리하므로, 뒤이어 만든 파일의 hook이 실행됐다면
				// 앞서 생긴 폴더의 감시 목록 반영도 끝났다. kqueue는 새 항목을 이름 순으로 알리므로
				// 다른 항목보다 뒤에 정렬되는 이름을 쓴다.
				write("zz-registered.txt")
				await("zz-registered.txt")

				write(tc.probe)
				if tc.want {
					await(tc.probe)
					return
				}
				// 제외된 변경이 오지 않았음을 시간 초과만으로 판단하지 않도록, 뒤이어 만든 파일의 hook이
				// 실행될 때까지 받은 경로를 확인한다.
				write("zz-after-probe.txt")
				await("zz-after-probe.txt")
				if seen[tc.probe] {
					t.Fatalf("change of %s ran the hook", tc.probe)
				}
			})
		}
	}
}
