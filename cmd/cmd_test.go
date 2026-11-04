package cmd

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func executeRoot(t *testing.T, args ...string) (stdout, stderr string, err error) {
	t.Helper()
	var out, errOut bytes.Buffer
	rootCmd.SetOut(&out)
	rootCmd.SetErr(&errOut)
	rootCmd.SetArgs(args)
	t.Cleanup(func() {
		rootCmd.SetOut(nil)
		rootCmd.SetErr(nil)
		rootCmd.SetArgs(nil)
		// RunE가 켠 SilenceUsage가 다음 테스트의 rootCmd에 남지 않게 되돌린다.
		rootCmd.SilenceUsage = false
	})
	err = rootCmd.Execute()
	return out.String(), errOut.String(), err
}

func TestRunErrorPrintsErrorWithoutUsage(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "missing.yml")

	stdout, stderr, err := executeRoot(t, "-c", missing)

	if err == nil {
		t.Fatal("Execute returned nil for a missing config file")
	}
	if !strings.Contains(stderr, "Error: "+err.Error()) {
		t.Fatalf("stderr lacks the error line:\n%s", stderr)
	}
	if strings.Contains(stdout+stderr, "Usage:") {
		t.Fatalf("usage printed for a run error:\nstdout:\n%s\nstderr:\n%s", stdout, stderr)
	}
}

func TestUnknownFlagPrintsUsage(t *testing.T) {
	stdout, stderr, err := executeRoot(t, "--no-such-flag")

	if err == nil {
		t.Fatal("Execute returned nil for an unknown flag")
	}
	if !strings.Contains(stdout+stderr, "Usage:") {
		t.Fatalf("usage missing for an unknown flag:\nstdout:\n%s\nstderr:\n%s", stdout, stderr)
	}
}

func TestRemovedExcludeKeyStopsBeforeWatching(t *testing.T) {
	dir := t.TempDir()
	config := filepath.Join(dir, "config.yml")
	data := fmt.Sprintf(`global:
  lifeCycle:
    onStartWatch:
      - dir: %q
        cmd: echo started> started.txt
watchTargets:
  - path: %q
    option:
      excludeDir: [dist]
`, dir, dir)
	if err := os.WriteFile(config, []byte(data), 0600); err != nil {
		t.Fatal(err)
	}

	_, stderr, err := executeRoot(t, "-c", config)

	want := `Error: watchTargets[0].option.excludeDir is no longer supported; use option.exclude (see USAGE "Moving from excludeDir and excludeSuffix")`
	if err == nil || !strings.Contains(stderr, want) {
		t.Fatalf("Execute error = %v, stderr:\n%s\nwant %q", err, stderr, want)
	}
	if _, err := os.Stat(filepath.Join(dir, "started.txt")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("onStartWatch ran before the config error: %v", err)
	}
}

func TestUnknownTemplateVariableStopsBeforeWatching(t *testing.T) {
	dir := t.TempDir()
	config := filepath.Join(dir, "config.yml")
	data := fmt.Sprintf(`global:
  lifeCycle:
    onStartWatch:
      - dir: %q
        cmd: echo started> started.txt
watchTargets:
  - path: %q
    lifeCycle:
      onStartWatch:
        - dir: %q
          cmd: echo target started> target.txt
      onChange:
        - echo {{.Path}}
        - echo {{.DirPath}}
`, dir, dir, dir)
	if err := os.WriteFile(config, []byte(data), 0600); err != nil {
		t.Fatal(err)
	}

	_, stderr, err := executeRoot(t, "-c", config)

	want := `Error: watchTargets[0].lifeCycle.onChange[1]: unknown template variable "DirPath"; available: Path, AbsPath, Name, ExtName, ParentPath, ParentAbsPath, Event`
	if err == nil || !strings.Contains(stderr, want) {
		t.Fatalf("Execute error = %v, stderr:\n%s\nwant %q", err, stderr, want)
	}
	for _, name := range []string{"started.txt", "target.txt"} {
		if _, err := os.Stat(filepath.Join(dir, name)); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("a hook made %s before the config error: %v", name, err)
		}
	}
}
