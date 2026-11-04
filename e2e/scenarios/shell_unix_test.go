//go:build !windows

package scenarios

import (
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/seungyeop-lee/directory-watcher/v3/e2e/harness"
)

func TestWindowsShellRejectedOnUnix(t *testing.T) {
	s := harness.NewScenario(t)
	s.Launch(harness.WatcherBin(), fmt.Sprintf(`shell: cmd
global:
  lifeCycle:
    onStartWatch:
      - %s
    onFinishWatch:
      - %s
    onError:
      - %s
    onQueueWarning:
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
`, s.Hook("--line=GLOBAL:start"), s.Hook("--line=GLOBAL:finish"), s.Hook("--line=GLOBAL:error"), s.Hook("--line=GLOBAL:queue"), s.Watch, s.Hook("--line=TARGET:start"), s.Hook("--line=TARGET:change"), s.Hook("--line=TARGET:finish")))
	s.Await("invalid shell exit", 10*time.Second, s.Exited)
	var exit *exec.ExitError
	if !errors.As(s.ExitErr(), &exit) || exit.ExitCode() != 1 {
		t.Fatalf("exit = %v, want exit code 1\n%s", s.ExitErr(), s.Log())
	}
	if got, want := strings.TrimSpace(s.Log()), `Error: shell "cmd" runs only on Windows`; got != want {
		t.Fatalf("output = %q, want %q", got, want)
	}
	if got := s.Lines(); len(got) != 0 {
		t.Fatalf("unsupported shell executed hooks: %q", got)
	}
}
