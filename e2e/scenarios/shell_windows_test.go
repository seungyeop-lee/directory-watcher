//go:build windows

package scenarios

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/seungyeop-lee/directory-watcher/v3/e2e/harness"
)

func TestPowershellEventVariables(t *testing.T) {
	s := harness.NewScenario(t)
	config := "shell: powershell\n" + strings.ReplaceAll(goldenConfig(s), "- '\"", "- '& \"")
	s.Start(config)
	s.AwaitLine("start hooks", 10*time.Second, "TARGET:onStartWatch")
	target := filepath.Join(s.Watch, goldenFileName)
	f, err := os.OpenFile(target, os.O_CREATE|os.O_RDONLY|os.O_EXCL, 0644)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	s.AwaitLine("create hooks", 10*time.Second, "GLOBAL:onAfterChange:"+changeValues(target, "C"))
	s.Write(target, []byte("hello"))
	s.AwaitLine("update hooks", 10*time.Second, "GLOBAL:onAfterChange:"+changeValues(target, "U"))
	if err := os.Remove(target); err != nil {
		t.Fatal(err)
	}
	s.AwaitLine("delete hooks", 10*time.Second, "GLOBAL:onAfterChange:"+changeValues(target, "D"))
	s.Stop()
	want := append(expectedSequence(target), "TARGET:onFinishWatch", "GLOBAL:onFinishWatch")
	if got := s.Lines(); !reflect.DeepEqual(got, want) {
		t.Fatalf("PowerShell hook sequence or eight event variables mismatch\nwant: %q\ngot: %q", want, got)
	}
	for _, dir := range []string{s.Work, s.Watch} {
		made, err := filepath.Glob(filepath.Join(dir, "pwned*"))
		if err != nil {
			t.Fatal(err)
		}
		if len(made) != 0 {
			t.Fatalf("file name ran as a PowerShell command and made %q", made)
		}
	}
}

func TestWindowsShellEmptyArgument(t *testing.T) {
	for _, shell := range []string{"cmd", "powershell"} {
		t.Run(shell, func(t *testing.T) {
			s := harness.NewScenario(t)
			empty := s.Hook("--line", "{{.ExtName}}", "AFTER")
			end := s.Hook("--line=END")
			if shell == "powershell" {
				empty = "'& " + strings.Trim(empty, "'") + "'"
				end = "'& " + strings.Trim(end, "'") + "'"
			}
			s.Start(fmt.Sprintf(`shell: %s
watchTargets:
  - path: '%s'
    lifeCycle:
      onChange:
        - %s
        - %s
    option:
      watch:
        exclude: ['*.probe']
        fileEvent: C
`, shell, s.Watch, empty, end))
			s.Write(filepath.Join(s.Watch, "no-extension"), nil)
			s.AwaitLine("command after empty argument", 10*time.Second, "END")
			s.Stop()
			want := []string{"", "END"}
			if shell == "powershell" {
				want[0] = "AFTER"
			}
			if got := s.Lines(); !reflect.DeepEqual(got, want) {
				t.Fatalf("%s empty argument mismatch\nwant: %q\ngot: %q", shell, want, got)
			}
		})
	}
}
