package scenarios

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/seungyeop-lee/directory-watcher/v3/e2e/harness"
)

// fsAction은 감시 폴더 root 기준 경로 하나에 하는 사용자 동작이다.
type fsAction func(t *testing.T, root string)

func touch(rel string) fsAction {
	return func(t *testing.T, root string) { createEmpty(t, filepath.Join(root, filepath.FromSlash(rel))) }
}

func mkdir(rel string) fsAction {
	return func(t *testing.T, root string) { mkdirAll(t, filepath.Join(root, filepath.FromSlash(rel))) }
}

func update(rel string) fsAction {
	return func(t *testing.T, root string) {
		t.Helper()
		// WriteFile의 truncate와 쓰기는 Windows에서 각각 U가 되므로 truncate 없이 한 번만 쓴다.
		f, err := os.OpenFile(filepath.Join(root, filepath.FromSlash(rel)), os.O_WRONLY, 0)
		if err != nil {
			t.Fatal(err)
		}
		_, writeErr := f.WriteString("x")
		closeErr := f.Close()
		if writeErr != nil {
			t.Fatal(writeErr)
		}
		if closeErr != nil {
			t.Fatal(closeErr)
		}
	}
}

func remove(rel string) fsAction {
	return func(t *testing.T, root string) {
		t.Helper()
		if err := os.RemoveAll(filepath.Join(root, filepath.FromSlash(rel))); err != nil {
			t.Fatal(err)
		}
	}
}

type watchOptionStep struct {
	name string
	act  []fsAction
	want []string
	// dirs는 감시 폴더 말고도 표지를 만들 폴더다. 동작이 감시 중인 하위 폴더 안에서 일어나면 그 폴더를 넣는다.
	dirs []string
}

// TestWatchOptions는 option.watch의 subDir, exclude, fileEvent·dirEvent none과 기본 제외, 폴더 링크인 path가
// 사용 안내대로 hook을 실행하는지 확인한다. hook을 실행하지 않아야 하는 동작은, 뒤이어 만든 표지가 hook에 닿을
// 때까지 그 동작의 기록이 없는지로 판정한다.
func TestWatchOptions(t *testing.T) {
	cases := []struct {
		name    string
		exclude []string
		// watch는 option.watch 아래에 exclude와 함께 둘 YAML 줄이다.
		watch string
		// target은 감시 대상 path와, 이벤트 경로의 기준이자 동작을 할 감시 폴더를 정한다. nil이면 둘 다 s.Watch다.
		target       func(t *testing.T, s *harness.Scenario) (path, root string)
		before       []fsAction
		dirSentinels bool
		steps        []watchOptionStep
	}{
		{
			name:   "subDir false",
			watch:  "subDir: false",
			before: []fsAction{mkdir("sub")},
			steps: []watchOptionStep{
				{name: "create a file in the target folder", act: []fsAction{touch("a.txt")}, want: []string{"C,false,a.txt"}},
				{name: "update a file in the target folder", act: []fsAction{update("a.txt")}, want: []string{"U,false,a.txt"}},
				{name: "delete a file in the target folder", act: []fsAction{remove("a.txt")}, want: []string{"D,false,a.txt"}},
				{name: "create a file in an existing subfolder", act: []fsAction{touch("sub/b.txt")}},
				{name: "update a file in an existing subfolder", act: []fsAction{update("sub/b.txt")}},
				{name: "delete a file in an existing subfolder", act: []fsAction{remove("sub/b.txt")}},
				{name: "create a subfolder", act: []fsAction{mkdir("new")}, want: []string{"C,true,new"}},
				{name: "create a file in a subfolder created while watching", act: []fsAction{touch("new/c.txt")}},
			},
		},
		{
			name:    "exclude the dist folder",
			exclude: []string{"dist/**"},
			before:  []fsAction{mkdir("dist"), touch("dist/old.txt")},
			steps: []watchOptionStep{
				{name: "create a file in the excluded folder", act: []fsAction{touch("dist/a.txt")}},
				{name: "delete the excluded folder", act: []fsAction{remove("dist")}},
				{name: "create the excluded folder with a file in it", act: []fsAction{mkdir("dist"), touch("dist/b.txt")}},
				{name: "create a file in the excluded folder created while watching", act: []fsAction{touch("dist/c.txt")}},
			},
		},
		{
			name:    "exclude entries of dist except keep.txt",
			exclude: []string{"dist/*", "!dist/keep.txt"},
			steps: []watchOptionStep{
				{name: "create dist", act: []fsAction{mkdir("dist")}, want: []string{"C,true,dist"}},
				// dist 안의 표지는 dist/*에 걸리므로, 뒤에 만든 keep.txt가 drop.txt의 표지 노릇을 한다.
				{name: "create drop.txt and then keep.txt in dist", act: []fsAction{touch("dist/drop.txt"), touch("dist/keep.txt")}, want: []string{"C,false,dist/keep.txt"}},
				{name: "delete dist", act: []fsAction{remove("dist")}, want: []string{"D,true,dist"}},
			},
		},
		{
			name:    "exclude log files except important.log",
			exclude: []string{"**/*.log", "!important.log"},
			before:  []fsAction{mkdir("logs")},
			steps: []watchOptionStep{
				{name: "create a .log file in the target folder", act: []fsAction{touch("build.log")}},
				{name: "create a .log file in a subfolder", act: []fsAction{touch("logs/app.log")}, dirs: []string{"logs"}},
				{name: "create important.log in the target folder", act: []fsAction{touch("important.log")}, want: []string{"C,false,important.log"}},
				{name: "create important.log in a subfolder", act: []fsAction{touch("logs/important.log")}, dirs: []string{"logs"}},
			},
		},
		{
			name:         "fileEvent none",
			watch:        "fileEvent: none",
			dirSentinels: true,
			steps: []watchOptionStep{
				{name: "create a file", act: []fsAction{touch("a.txt")}},
				{name: "update a file", act: []fsAction{update("a.txt")}},
				{name: "delete a file", act: []fsAction{remove("a.txt")}},
				{name: "create a folder", act: []fsAction{mkdir("d")}, want: []string{"C,true,d"}},
				{name: "delete a folder", act: []fsAction{remove("d")}, want: []string{"D,true,d"}},
			},
		},
		{
			name:  "dirEvent none",
			watch: "dirEvent: none",
			steps: []watchOptionStep{
				{name: "create a folder", act: []fsAction{mkdir("d")}},
				{name: "create a file in the created folder", act: []fsAction{touch("d/a.txt")}, want: []string{"C,false,d/a.txt"}, dirs: []string{"d"}},
				{name: "create a folder with a file in it", act: []fsAction{mkdir("n"), touch("n/a.txt")}},
				{name: "delete a folder with files in it", act: []fsAction{remove("d")}},
			},
		},
		{
			name:   "default exclusion",
			before: []fsAction{mkdir(".git"), touch(".git/HEAD")},
			steps: []watchOptionStep{
				{name: "create .env", act: []fsAction{touch(".env")}},
				{name: "update .env", act: []fsAction{update(".env")}},
				{name: "delete .env", act: []fsAction{remove(".env")}},
				{name: "create a file in .git", act: []fsAction{touch(".git/index")}},
				{name: "create .idea with a file in it", act: []fsAction{mkdir(".idea"), touch(".idea/workspace.xml")}},
				{name: "create _draft.txt", act: []fsAction{touch("_draft.txt")}},
			},
		},
		{
			name:   "path named with a leading dot",
			target: dotNamedTarget,
			steps: []watchOptionStep{
				{name: "create a file", act: []fsAction{touch("a.txt")}, want: []string{"C,false,a.txt"}},
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := harness.NewScenario(t)
			path, root := s.Watch, s.Watch
			if tc.target != nil {
				path, root = tc.target(t, s)
			}
			for _, act := range tc.before {
				act(t, root)
			}
			exclude := "'*.probe'"
			for _, pattern := range tc.exclude {
				exclude += ", '" + pattern + "'"
			}
			watch := ""
			if tc.watch != "" {
				watch = "\n        " + tc.watch
			}
			s.Start(fmt.Sprintf(`watchTargets:
  - path: '%s'
    lifeCycle:
      onChange:
        - %s
    option:
      watch:
        exclude: [%s]%s
      hookBatch:
        waitMillisecond: 0
        deliver: each
`, path, s.Hook("--line=CHANGE:{{.Event}},{{.IsDir}},{{.Path}}"), exclude, watch))

			changes := &changeSteps{t: t, s: s, root: root, dirSentinels: tc.dirSentinels}
			for _, step := range tc.steps {
				for _, act := range step.act {
					act(t, root)
				}
				got := changes.next(step.name, step.want, append([]string{""}, step.dirs...)...)
				if !reflect.DeepEqual(got, step.want) {
					t.Errorf("%s: hook received %q, want %q", step.name, got, step.want)
				}
			}
			s.Stop()
		})
	}
	t.Run("path is a folder link", func(t *testing.T) {
		s := harness.NewScenario(t)
		path, root := linkedTarget(t, s)
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
`, path, s.Hook("--line=CHANGE:{{.Path}},{{.AbsPath}},{{.Name}},{{.ExtName}},{{.ParentPath}},{{.ParentAbsPath}},{{.Event}},{{.IsDir}}")))
		target := filepath.Join(root, "a.txt")
		var want []string
		for _, step := range []struct {
			event string
			act   fsAction
		}{{"C", touch("a.txt")}, {"U", update("a.txt")}, {"D", remove("a.txt")}} {
			step.act(t, root)
			line := "CHANGE:" + changeValues(target, step.event)
			s.AwaitLine("resolved "+step.event+" variables", 10*time.Second, line)
			want = append(want, line)
		}
		s.Stop()
		if got := s.Lines(); !reflect.DeepEqual(got, want) {
			t.Fatalf("resolved event variables: got %q, want %q", got, want)
		}
	})
}

// dotNamedTarget은 이름이 .으로 시작하는 폴더를 감시 대상 path로 만든다. Start가 s.Watch에 준비 확인용 파일을
// 쓰므로 s.Watch도 그 폴더로 바꾼다.
func dotNamedTarget(t *testing.T, s *harness.Scenario) (string, string) {
	s.Watch = filepath.Join(s.Work, ".watch")
	mkdirAll(t, s.Watch)
	return s.Watch, s.Watch
}

// linkedTarget은 s.Watch를 가리키는 링크와 링크를 모두 푼 감시 폴더 경로를 돌려준다.
func linkedTarget(t *testing.T, s *harness.Scenario) (string, string) {
	link := filepath.Join(s.Work, "link")
	if err := os.Symlink(s.Watch, link); err != nil {
		t.Skipf("symlink not available: %v", err)
	}
	resolved, err := filepath.EvalSymlinks(s.Watch)
	if err != nil {
		t.Fatal(err)
	}
	return link, resolved
}
