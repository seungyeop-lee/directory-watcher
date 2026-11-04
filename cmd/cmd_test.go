package cmd

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/pflag"
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
		// RunE가 켠 SilenceUsage와 이번에 준 플래그 값이 다음 테스트의 rootCmd에 남지 않게 되돌린다.
		rootCmd.SilenceUsage = false
		rootCmd.Flags().VisitAll(func(f *pflag.Flag) {
			_ = f.Value.Set(f.DefValue)
			f.Changed = false
		})
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

	want := `Error: watchTargets[0].option.excludeDir is no longer supported; use option.watch.exclude`
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

// hookCommandConfig는 실행 위치 dir에 started.txt를 만드는 전역 onStartWatch와, 감시를 시작하지 못해 watcher를 바로
// 끝내는 없는 감시 대상을 적은 설정 파일을 만들고 그 경로를 돌려준다.
func hookCommandConfig(t *testing.T, dir string) string {
	t.Helper()
	config := filepath.Join(dir, "config.yml")
	data := fmt.Sprintf(`global:
  lifeCycle:
    onStartWatch:
      - dir: %q
        cmd: echo started> started.txt
watchTargets:
  - path: %q
`, dir, filepath.Join(dir, "missing-target"))
	if err := os.WriteFile(config, []byte(data), 0600); err != nil {
		t.Fatal(err)
	}
	return config
}

// executeRootCapturingHooks는 executeRoot처럼 rootCmd를 실행하고, hook 출력이 가는 표준 출력을 모아 돌려준다.
func executeRootCapturingHooks(t *testing.T, args ...string) (hookOutput string, err error) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "stdout")
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	original := os.Stdout
	os.Stdout = file
	_, _, err = executeRoot(t, args...)
	os.Stdout = original
	data, readErr := os.ReadFile(path)
	if readErr != nil {
		t.Fatal(readErr)
	}
	return string(data), err
}

func TestHookCommandPrintingOptions(t *testing.T) {
	cases := map[string]struct {
		args        []string
		wantPrinted bool
		wantRun     bool
	}{
		"default":   {nil, true, true},
		"--silent":  {[]string{"--silent"}, false, true},
		"-s":        {[]string{"-s"}, false, true},
		"--dry-run": {[]string{"--dry-run"}, true, false},
		"-n":        {[]string{"-n"}, true, false},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			output, err := executeRootCapturingHooks(t, append([]string{"-c", hookCommandConfig(t, dir)}, tc.args...)...)
			if err == nil || !strings.Contains(err.Error(), "missing-target") {
				t.Fatalf("Execute error = %v, want the missing target error", err)
			}
			line := "onStartWatch: echo started> started.txt (dir: " + dir + ")"
			if printed := strings.Contains(output, line); printed != tc.wantPrinted {
				t.Errorf("printed %q = %t, want %t; output:\n%s", line, printed, tc.wantPrinted, output)
			}
			_, statErr := os.Stat(filepath.Join(dir, "started.txt"))
			if ran := statErr == nil; ran != tc.wantRun {
				t.Errorf("onStartWatch ran = %t, want %t", ran, tc.wantRun)
			}
		})
	}
}

func TestSilentWithDryRunStopsBeforeWatching(t *testing.T) {
	dir := t.TempDir()
	output, err := executeRootCapturingHooks(t, "-c", hookCommandConfig(t, dir), "-s", "-n")
	if err == nil || !strings.Contains(err.Error(), "silent") || !strings.Contains(err.Error(), "dry-run") {
		t.Fatalf("Execute error = %v, want an error naming silent and dry-run", err)
	}
	if output != "" {
		t.Fatalf("hooks printed before the flag error:\n%s", output)
	}
	if _, err := os.Stat(filepath.Join(dir, "started.txt")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("onStartWatch ran before the flag error: %v", err)
	}
	// hook 실행 문서 두 판의 오류 예시 줄은 이 오류 메시지를 옮겨 적은 것이다.
	for _, file := range []string{"running.md", "running.ko.md"} {
		data, readErr := os.ReadFile(filepath.Join("..", "docs", "guide", file))
		if readErr != nil {
			t.Fatal(readErr)
		}
		if !strings.Contains(string(data), "Error: "+err.Error()) {
			t.Errorf("%s has no error example %q", file, "Error: "+err.Error())
		}
	}
}

// 설치 문서 두 판의 명령줄 옵션 블록은 directory-watcher --help의 출력을 옮겨 적은 것이다.
func TestInstallationGuideShowsHelpOutput(t *testing.T) {
	use, version := rootCmd.Use, rootCmd.Version
	rootCmd.Use, rootCmd.Version = "directory-watcher", "v0.0.0"
	t.Cleanup(func() { rootCmd.Use, rootCmd.Version = use, version })
	// cobra는 처음 실행할 때의 Use로 -h 설명을 만든다. 앞선 테스트가 Use 없이 실행했으면 "help for this command"가
	// 되므로, Execute가 Use를 정한 뒤 처음 실행하는 실제 실행과 같게 맞춘다.
	if help := rootCmd.Flags().Lookup("help"); help != nil {
		help.Usage = "help for directory-watcher"
	}
	help, _, err := executeRoot(t, "--help")
	if err != nil {
		t.Fatal(err)
	}
	for _, file := range []string{"installation.md", "installation.ko.md"} {
		data, err := os.ReadFile(filepath.Join("..", "docs", "guide", file))
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(data), "```shell\n"+help+"```\n") {
			t.Errorf("%s has no shell block equal to the --help output:\n%s", file, help)
		}
	}
}
