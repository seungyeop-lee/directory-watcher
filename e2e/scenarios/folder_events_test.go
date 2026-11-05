package scenarios

import (
	"fmt"
	"os"
	"path"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/seungyeop-lee/directory-watcher/v3/e2e/harness"
)

// TestFolderEvents는 사용자 동작마다 hook이 받는 이벤트를 {{.Event}},{{.IsDir}},<감시 폴더 기준 경로>로 확인한다.
// 폴더가 안의 내용과 함께 생기거나 사라지면 맨 위 폴더의 이벤트 하나만 온다. 감시 중에 생기거나 옮겨 온 폴더
// 안에서 나중에 만든 파일은 hook을 실행하고, 감시 대상 밖으로 옮긴 폴더 안에서 만든 파일은 실행하지 않는다.
func TestFolderEvents(t *testing.T) {
	t.Parallel()
	t.Run("actions", func(t *testing.T) {
		t.Parallel()
		f := startFolderEvents(t, "")
		// Windows는 감시 중인 하위 폴더가 있는 폴더의 이름을 바꾸거나 옮기지 못하므로, 옮기는 폴더에는 파일만 둔다.
		fill(t, filepath.Join(f.outside, "x"), 3)
		// createLater는 앞 단계에서 폴더 이벤트를 받은 폴더 안에 빈 파일 rel을 만들고, hook이 그 파일의 C를 받기를
		// 기대하는 단계를 만든다.
		createLater := func(name, rel string) folderStep {
			return folderStep{name, func() { createEmpty(t, f.watch(rel)) }, []string{"C,false," + rel}, path.Dir(rel)}
		}
		f.run(&changeSteps{t: t, s: f.s, root: f.s.Watch}, []folderStep{
			{"mkdir", func() { mkdirAll(t, f.watch("src/pkg")) }, []string{"C,true,src/pkg"}, ""},
			createLater("create a file in a created folder", "src/pkg/after-mkdir.txt"),
			{"mkdir -p", func() { mkdirAll(t, f.watch("src/a/b/c")) }, []string{"C,true,src/a"}, ""},
			createLater("create a file in the deepest folder of mkdir -p", "src/a/b/c/after-mkdir-p.txt"),
			{"move in a folder with files", func() { f.move(filepath.Join(f.outside, "x"), f.watch("src/x")) }, []string{"C,true,src/x"}, ""},
			createLater("create a file in a folder moved in", "src/x/after-move-in.txt"),
			{"mkdir and write a file in it", func() {
				mkdirAll(t, f.watch("src/n"))
				f.s.Write(f.watch("src/n/a.txt"), []byte("hi"))
			}, []string{"C,true,src/n"}, ""},
			{"move a folder out of the target", func() {
				f.move(f.watch("src/x"), filepath.Join(f.outside, "x"))
				if runtime.GOOS == "windows" {
					// 감시 대상 밖으로 옮긴 뒤 감시를 다시 만드는 순간의 중복은 사용 안내가 허용한다.
					// 다음 동작과 표지는 재생성 로그를 받은 뒤 만들어 모든 단계의 기록을 그대로 비교한다.
					f.s.Await("watcher rebuilt after moving a folder out", 10*time.Second, func() bool {
						return strings.Contains(f.s.Log(), "rebuilt watcher to release watches of folders moved out")
					})
				}
			}, []string{"D,true,src/x"}, ""},
			{"create a file in a folder moved out", func() { createEmpty(t, filepath.Join(f.outside, "x", "after-move-out.txt")) }, nil, ""},
			{"rename a folder", func() { f.move(f.watch("src/n"), f.watch("src/m")) }, []string{"D,true,src/n", "C,true,src/m"}, ""},
			createLater("create a file in a renamed folder", "src/m/after-rename.txt"),
			{"move a folder under another folder", func() { f.move(f.watch("src/m"), f.watch("dst/m")) }, []string{"D,true,src/m", "C,true,dst/m"}, ""},
			createLater("create a file in a folder moved under another folder", "dst/m/after-move.txt"),
			{"create a file", func() { createEmpty(t, f.watch("src/f.txt")) }, []string{"C,false,src/f.txt"}, ""},
			{"rename a file", func() { f.move(f.watch("src/f.txt"), f.watch("src/g.txt")) }, []string{"D,false,src/f.txt", "C,false,src/g.txt"}, ""},
			{"delete a file", func() {
				if err := os.Remove(f.watch("src/g.txt")); err != nil {
					t.Fatal(err)
				}
			}, []string{"D,false,src/g.txt"}, ""},
		})
	})
	// 부하가 큰 Windows CI에서는 파일 300개가 든 폴더를 지우는 동안 이벤트 사이가 dirBatch 기본 대기 100ms를 넘어,
	// 먼저 받은 파일 삭제가 폴더 D에 흡수되지 않고 hook에 간 적이 있다. 그래서 이 하위 테스트만 대기를 1초, 상한을
	// 10초로 늘리고, 표지는 이 상한보다 길게 기다린다. 늘린 대기는 단계마다 걸리는 시간을 늘리므로 다른 동작과 나눈다.
	t.Run("folderWith300Files", func(t *testing.T) {
		t.Parallel()
		f := startFolderEvents(t, `
      dirBatch:
        waitMillisecond: 1000
        maxWaitMillisecond: 10000`)
		fill(t, filepath.Join(f.outside, "big"), 300)
		f.run(&changeSteps{t: t, s: f.s, root: f.s.Watch, timeout: 20 * time.Second}, []folderStep{
			{"move in a folder with 300 files", func() { f.move(filepath.Join(f.outside, "big"), f.watch("src/big")) }, []string{"C,true,src/big"}, ""},
			{"rm -rf a folder with 300 files", func() {
				if err := os.RemoveAll(f.watch("src/big")); err != nil {
					t.Fatal(err)
				}
			}, []string{"D,true,src/big"}, ""},
		})
	})
}

// folderStep은 TestFolderEvents의 사용자 동작 한 단계와 그 동작에서 hook이 받기를 기대하는 기록이다.
type folderStep struct {
	name string
	act  func()
	want []string
	// folder는 act가 파일을 만드는 감시 대상 안의 폴더다. 이 폴더에도 표지 파일을 만든다.
	folder string
}

// folderEvents는 TestFolderEvents의 하위 테스트 하나가 띄운 watcher와 감시 대상 밖의 폴더 outside다.
type folderEvents struct {
	t       *testing.T
	s       *harness.Scenario
	outside string
}

// startFolderEvents는 감시 폴더에 src와 dst를, 실행 폴더에 outside를 만들고 watcher를 띄운다. option은 대상
// option 아래에 덧붙일 설정이다.
func startFolderEvents(t *testing.T, option string) *folderEvents {
	s := harness.NewScenario(t)
	f := &folderEvents{t: t, s: s, outside: filepath.Join(s.Work, "outside")}
	for _, dir := range []string{f.watch("src"), f.watch("dst"), f.outside} {
		mkdirAll(t, dir)
	}
	s.Start(fmt.Sprintf(`watchTargets:
  - path: '%s'
    lifeCycle:
      onChange:
        - %s
    option:
      watch:
        exclude: ['*.probe']%s
      hookBatch:
        waitMillisecond: 0
        deliver: each
`, s.Watch, s.Hook("--line=CHANGE:{{.Event}},{{.IsDir}},{{.Path}}"), option))
	return f
}

// watch는 감시 폴더 기준 상대 경로 rel을 경로로 바꾼다.
func (f *folderEvents) watch(rel string) string {
	return filepath.Join(f.s.Watch, filepath.FromSlash(rel))
}

func (f *folderEvents) move(from, to string) {
	f.t.Helper()
	if err := os.Rename(from, to); err != nil {
		f.t.Fatal(err)
	}
}

// run은 steps를 차례로 실행하고, 단계마다 hook이 받은 기록이 기대 기록과 같은지 확인한 뒤 watcher를 멈춘다.
func (f *folderEvents) run(changes *changeSteps, steps []folderStep) {
	f.t.Helper()
	for _, step := range steps {
		step.act()
		// Windows는 폴더마다 이벤트를 따로 받으므로, 이벤트가 생기는 폴더마다 표지 파일을 만들어 모두 기다린다.
		dirs := []string{"", "src", "dst"}
		if step.folder != "" {
			dirs = append(dirs, step.folder)
		}
		got := changes.next(step.name, nil, dirs...)
		if !reflect.DeepEqual(got, step.want) {
			f.t.Errorf("%s: hook received %q, want %q", step.name, got, step.want)
		}
	}
	f.s.Stop()
}

// fill은 dir 안에 파일 n개를 만든다.
func fill(t *testing.T, dir string, n int) {
	t.Helper()
	mkdirAll(t, dir)
	for i := 0; i < n; i++ {
		if err := os.WriteFile(filepath.Join(dir, fmt.Sprintf("f%d.txt", i)), []byte("x"), 0644); err != nil {
			t.Fatal(err)
		}
	}
}
