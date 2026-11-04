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
	s := harness.NewScenario(t)
	outside := filepath.Join(s.Work, "outside")
	for _, dir := range []string{filepath.Join(s.Watch, "src"), filepath.Join(s.Watch, "dst"), outside} {
		mkdirAll(t, dir)
	}
	// Windows는 감시 중인 하위 폴더가 있는 폴더의 이름을 바꾸거나 옮기지 못하므로, 옮기는 폴더에는 파일만 둔다.
	fill(t, filepath.Join(outside, "x"), 3)
	fill(t, filepath.Join(outside, "big"), 300)
	s.Start(fmt.Sprintf(`watchTargets:
  - path: '%s'
    lifeCycle:
      onChange:
        - %s
    option:
      watch:
        exclude: ['*.probe']
      hookBatch:
        waitMillisecond: 0
        deliver: each
`, s.Watch, s.Hook("--line=CHANGE:{{.Event}},{{.IsDir}},{{.Path}}")))

	watch := func(rel string) string { return filepath.Join(s.Watch, filepath.FromSlash(rel)) }
	move := func(from, to string) {
		t.Helper()
		if err := os.Rename(from, to); err != nil {
			t.Fatal(err)
		}
	}
	type step struct {
		name string
		act  func()
		want []string
		// folder는 act가 파일을 만드는 감시 대상 안의 폴더다. 이 폴더에도 표지 파일을 만든다.
		folder string
	}
	// createLater는 앞 단계에서 폴더 이벤트를 받은 폴더 안에 빈 파일 rel을 만들고, hook이 그 파일의 C를 받기를
	// 기대하는 단계를 만든다.
	createLater := func(name, rel string) step {
		return step{name, func() { createEmpty(t, watch(rel)) }, []string{"C,false," + rel}, path.Dir(rel)}
	}
	steps := []step{
		{"mkdir", func() { mkdirAll(t, watch("src/pkg")) }, []string{"C,true,src/pkg"}, ""},
		createLater("create a file in a created folder", "src/pkg/after-mkdir.txt"),
		{"mkdir -p", func() { mkdirAll(t, watch("src/a/b/c")) }, []string{"C,true,src/a"}, ""},
		createLater("create a file in the deepest folder of mkdir -p", "src/a/b/c/after-mkdir-p.txt"),
		{"move in a folder with files", func() { move(filepath.Join(outside, "x"), watch("src/x")) }, []string{"C,true,src/x"}, ""},
		createLater("create a file in a folder moved in", "src/x/after-move-in.txt"),
		{"mkdir and write a file in it", func() {
			mkdirAll(t, watch("src/n"))
			s.Write(watch("src/n/a.txt"), []byte("hi"))
		}, []string{"C,true,src/n"}, ""},
		{"move in a folder with 300 files", func() { move(filepath.Join(outside, "big"), watch("src/big")) }, []string{"C,true,src/big"}, ""},
		{"rm -rf a folder with 300 files", func() {
			if err := os.RemoveAll(watch("src/big")); err != nil {
				t.Fatal(err)
			}
		}, []string{"D,true,src/big"}, ""},
		{"move a folder out of the target", func() {
			move(watch("src/x"), filepath.Join(outside, "x"))
			if runtime.GOOS == "windows" {
				// 감시 대상 밖으로 옮긴 뒤 감시를 다시 만드는 순간의 중복은 사용 안내가 허용한다.
				// 다음 동작과 표지는 재생성 로그를 받은 뒤 만들어 모든 단계의 기록을 그대로 비교한다.
				s.Await("watcher rebuilt after moving a folder out", 10*time.Second, func() bool {
					return strings.Contains(s.Log(), "rebuilt watcher to release watches of folders moved out")
				})
			}
		}, []string{"D,true,src/x"}, ""},
		{"create a file in a folder moved out", func() { createEmpty(t, filepath.Join(outside, "x", "after-move-out.txt")) }, nil, ""},
		{"rename a folder", func() { move(watch("src/n"), watch("src/m")) }, []string{"D,true,src/n", "C,true,src/m"}, ""},
		createLater("create a file in a renamed folder", "src/m/after-rename.txt"),
		{"move a folder under another folder", func() { move(watch("src/m"), watch("dst/m")) }, []string{"D,true,src/m", "C,true,dst/m"}, ""},
		createLater("create a file in a folder moved under another folder", "dst/m/after-move.txt"),
		{"create a file", func() { createEmpty(t, watch("src/f.txt")) }, []string{"C,false,src/f.txt"}, ""},
		{"rename a file", func() { move(watch("src/f.txt"), watch("src/g.txt")) }, []string{"D,false,src/f.txt", "C,false,src/g.txt"}, ""},
		{"delete a file", func() {
			if err := os.Remove(watch("src/g.txt")); err != nil {
				t.Fatal(err)
			}
		}, []string{"D,false,src/g.txt"}, ""},
	}
	changes := &changeSteps{t: t, s: s, root: s.Watch}
	for _, step := range steps {
		step.act()
		// Windows는 폴더마다 이벤트를 따로 받으므로, 이벤트가 생기는 폴더마다 표지 파일을 만들어 모두 기다린다.
		dirs := []string{"", "src", "dst"}
		if step.folder != "" {
			dirs = append(dirs, step.folder)
		}
		got := changes.next(step.name, nil, dirs...)
		if !reflect.DeepEqual(got, step.want) {
			t.Errorf("%s: hook received %q, want %q", step.name, got, step.want)
		}
	}
	s.Stop()
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
