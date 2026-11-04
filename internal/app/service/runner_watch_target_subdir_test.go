package service

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/seungyeop-lee/directory-watcher/v3/internal/app/domain"
)

// 폴더가 감시 시작 전에 있었는지, 감시 중에 만들어졌는지, 감시 중에 통째로 옮겨 왔는지와
// 관계없이 같은 규칙으로 감시되는지 확인한다.
func TestSubDirWatchRuleIsSameAtStartAndWhileWatching(t *testing.T) {
	cases := []struct {
		name    string
		dirs    []string // 감시 대상 안에 만들 폴더
		absLink string   // 감시 대상 밖 폴더를 절대 경로로 가리키는 링크의 이름
		relLink string   // 감시 대상 밖 폴더를 상대 경로로 가리키는 링크의 이름
		exclude []string
		probe   string // 내용을 쓸 파일
		want    bool   // probe의 변경으로 hook이 실행되어야 하는지
	}{
		{name: "plain nested folder", dirs: []string{"plain/sub"}, probe: "plain/sub/a.txt", want: true},
		{name: "default-excluded folder", dirs: []string{".cache/sub"}, probe: ".cache/sub/a.txt", want: false},
		{name: "default-excluded nested folder", dirs: []string{"plain/_tmp"}, probe: "plain/_tmp/a.txt", want: false},
		{name: "exclude sibling with same prefix", dirs: []string{"dir1", "dir10"}, exclude: []string{"dir1/**"}, probe: "dir10/a.txt", want: true},
		{name: "exclude subfolder", dirs: []string{"parent/sub/deep"}, exclude: []string{"parent/sub/**"}, probe: "parent/sub/deep/a.txt", want: false},
		{name: "exclude folder at any depth", dirs: []string{"a/node_modules/pkg"}, exclude: []string{"**/node_modules/**"}, probe: "a/node_modules/pkg/a.txt", want: false},
		{name: "absolute symlink to folder", absLink: "link", probe: "link/a.txt", want: false},
		{name: "relative symlink to folder", relLink: "link", probe: "link/a.txt", want: false},
		{name: "absolute and relative symlinks to folder", absLink: "abs", relLink: "rel", probe: "abs/a.txt", want: false},
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
					for _, link := range []struct{ name, dest string }{
						{tc.absLink, outside},
						{tc.relLink, filepath.Join("..", "outside")},
					} {
						if link.name == "" {
							continue
						}
						if err := os.Symlink(link.dest, filepath.Join(root, link.name)); err != nil {
							t.Skipf("symlink not available: %v", err)
						}
					}
				}
				if tc.relLink != "" {
					// fsnotify의 kqueue 구현(macOS)은 상대 경로 링크를 작업 디렉터리 기준으로 풀어, 그 위치에
					// 폴더가 있을 때만 따라간다. 링크가 가리키는 폴더와 같은 폴더로 풀리도록 작업 디렉터리를 옮긴다.
					chdir(t, target)
				}
				if timing.start {
					build(target)
				}

				r := NewWatchTargetRunner(domain.GlobalCommandSet{}, domain.WatchTargetsCommandSet{
					Path: domain.Path(target),
					Option: domain.WatchTargetsOption{
						NoWait:      true,
						WatchSubDir: true,
						Exclude:     tc.exclude,
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
							gotAbs, err := filepath.Abs(got.String())
							if err != nil {
								t.Fatal(err)
							}
							gotRel, err := filepath.Rel(target, gotAbs)
							if got == "" || err != nil || gotRel == ".." || strings.HasPrefix(gotRel, ".."+string(filepath.Separator)) {
								t.Fatalf("event outside the watch target ran the hook: %q", got)
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

// 감시 대상 path가 폴더를 가리키는 링크이면 watchSubDir와 관계없이 링크가 가리키는 폴더를 감시하고,
// exclude 패턴도 그 폴더의 같은 위치를 빼는지 확인한다.
func TestLinkedTargetPathWatchesLinkedFolder(t *testing.T) {
	cases := []struct {
		name        string
		watchSubDir bool
		startDirs   []string // 감시 시작 전에 링크가 가리키는 폴더 안에 만들 폴더
		laterDirs   []string // 감시 중에 링크가 가리키는 폴더 안에 만들 폴더
		exclude     []string
		probe       string
		want        bool
	}{
		{name: "file in linked folder without watchSubDir", probe: "a.txt", want: true},
		{name: "file in linked folder with watchSubDir", watchSubDir: true, probe: "a.txt", want: true},
		{name: "existing subfolder", watchSubDir: true, startDirs: []string{"sub"}, probe: "sub/a.txt", want: true},
		{name: "subfolder created while watching", watchSubDir: true, laterDirs: []string{"sub"}, probe: "sub/a.txt", want: true},
		// exclude 패턴이 있으면 감시 루프가 새 폴더를 감시 목록에 넣기까지 시간이 늘어, macOS에서 reAddDirs가
		// 되돌리는 fsnotify의 등록 경합이 자주 일어난다.
		{name: "subfolder created while watching with exclude", watchSubDir: true, laterDirs: []string{"sub"}, exclude: []string{"other/**"}, probe: "sub/a.txt", want: true},
		{name: "existing excluded folder", watchSubDir: true, startDirs: []string{"ex"}, exclude: []string{"ex/**"}, probe: "ex/a.txt", want: false},
		{name: "excluded folder created while watching", watchSubDir: true, laterDirs: []string{"ex"}, exclude: []string{"ex/**"}, probe: "ex/a.txt", want: false},
		{name: "excluded file", watchSubDir: true, exclude: []string{"skip.txt"}, probe: "skip.txt", want: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			base := t.TempDir()
			linked, link := filepath.Join(base, "linked"), filepath.Join(base, "link")
			if err := os.Mkdir(linked, 0700); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(linked, link); err != nil {
				t.Skipf("symlink not available: %v", err)
			}
			// 이벤트 경로는 링크를 모두 푼 폴더 기준으로 온다. macOS의 TempDir처럼 상위 경로에 링크가 있어도 맞춘다.
			resolved, err := filepath.EvalSymlinks(linked)
			if err != nil {
				t.Fatal(err)
			}
			makeDirs := func(dirs []string) {
				for _, dir := range dirs {
					if err := os.MkdirAll(filepath.Join(linked, dir), 0700); err != nil {
						t.Fatal(err)
					}
				}
			}
			makeDirs(tc.startDirs)

			r := NewWatchTargetRunner(domain.GlobalCommandSet{}, domain.WatchTargetsCommandSet{
				Path: domain.Path(link),
				Option: domain.WatchTargetsOption{
					NoWait:      true,
					WatchSubDir: tc.watchSubDir,
					Exclude:     tc.exclude,
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
				if err := os.WriteFile(filepath.Join(linked, rel), []byte("event"), 0600); err != nil {
					t.Fatal(err)
				}
			}
			await := func(rel string) {
				for !seen[rel] {
					select {
					case got := <-changed:
						gotRel, err := filepath.Rel(resolved, got.String())
						if err != nil {
							t.Fatal(err)
						}
						seen[filepath.ToSlash(gotRel)] = true
					case err := <-result:
						t.Fatalf("run stopped: %v", err)
					case <-time.After(5 * time.Second):
						t.Fatalf("change of %s did not reach the hook (seen %v)", rel, seen)
					}
				}
			}

			makeDirs(tc.laterDirs)
			// 감시 루프는 이벤트를 받은 순서대로 처리하므로, 뒤이어 만든 파일의 hook이 실행됐다면
			// 앞서 생긴 폴더의 감시 목록 반영도 끝났다.
			write("zz-registered.txt")
			await("zz-registered.txt")

			write(tc.probe)
			if tc.want {
				await(tc.probe)
				return
			}
			write("zz-after-probe.txt")
			await("zz-after-probe.txt")
			if seen[tc.probe] {
				t.Fatalf("change of %s ran the hook", tc.probe)
			}
		})
	}
}
