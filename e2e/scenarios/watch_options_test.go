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
}

// TestWatchOptions는 option.watch의 exclude와 fileEvent none, 폴더 링크인 path가 사용 안내대로 hook을 실행하는지
// 확인한다. hook을 실행하지 않아야 하는 동작은, 뒤이어 만든 표지가 hook에 닿을 때까지 그 동작의 기록이 없는지로
// 판정한다.
func TestWatchOptions(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name    string
		exclude []string
		// watch는 option.watch 아래에 exclude와 함께 둘 YAML 줄이다.
		watch        string
		before       []fsAction
		dirSentinels bool
		steps        []watchOptionStep
	}{
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
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			s := harness.NewScenario(t)
			for _, act := range tc.before {
				act(t, s.Watch)
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
`, s.Watch, s.Hook("--line=CHANGE:{{.Event}},{{.IsDir}},{{.Path}}"), exclude, watch))

			changes := &changeSteps{t: t, s: s, root: s.Watch, dirSentinels: tc.dirSentinels}
			for _, step := range tc.steps {
				for _, act := range step.act {
					act(t, s.Watch)
				}
				got := changes.next(step.name, step.want, "")
				if !reflect.DeepEqual(got, step.want) {
					t.Errorf("%s: hook received %q, want %q", step.name, got, step.want)
				}
			}
			s.Stop()
		})
	}
	t.Run("path is a folder link", func(t *testing.T) {
		t.Parallel()
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
