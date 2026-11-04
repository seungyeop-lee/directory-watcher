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

func TestHookWorkingDirectory(t *testing.T) {
	for _, explicitDir := range []bool{false, true} {
		t.Run(fmt.Sprintf("dir=%t", explicitDir), func(t *testing.T) {
			s := harness.NewScenario(t)
			work, err := filepath.EvalSymlinks(s.Work)
			if err != nil {
				t.Fatal(err)
			}
			s.Work, s.Watch, s.Records = work, filepath.Join(work, "watch"), filepath.Join(work, "records.log")
			globalDir, targetDir := s.Work, s.Watch
			dir := ""
			if explicitDir {
				globalDir = filepath.Join(s.Work, "hook cwd")
				targetDir = globalDir
				if err := os.Mkdir(globalDir, 0755); err != nil {
					t.Fatal(err)
				}
				dir = fmt.Sprintf("\n          dir: '%s'", globalDir)
			}
			hook := func(label string) string {
				return "cmd: " + s.Hook("--workdir", "--line="+label) + dir
			}
			s.Start(fmt.Sprintf(`global:
  lifeCycle:
    onStartWatch:
        - %s
    onFinishWatch:
        - %s
watchTargets:
  - path: '%s'
    lifeCycle:
      onStartWatch:
        - %s
      onChange:
        - %s
      onFinishWatch:
        - %s
    option:
      watch:
        exclude: ['*.probe']
        fileEvent: C
`, hook("GLOBAL:start"), hook("GLOBAL:finish"), s.Watch, hook("TARGET:start"), hook("TARGET:change"), hook("TARGET:finish")))
			s.AwaitLine("target start hook", 10*time.Second, "TARGET:start")
			s.Write(filepath.Join(s.Watch, "created.txt"), nil)
			s.AwaitLine("target change hook", 10*time.Second, "TARGET:change")
			s.Stop()
			want := []string{
				"WORKDIR|" + globalDir, "GLOBAL:start",
				"WORKDIR|" + targetDir, "TARGET:start",
				"WORKDIR|" + targetDir, "TARGET:change",
				"WORKDIR|" + targetDir, "TARGET:finish",
				"WORKDIR|" + globalDir, "GLOBAL:finish",
			}
			if got := s.Lines(); !reflect.DeepEqual(got, want) {
				t.Fatalf("hook directories or sequence mismatch\nwant: %q\ngot: %q", want, got)
			}
		})
	}
}
