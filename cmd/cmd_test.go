package cmd

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/spf13/pflag"
)

func executeRoot(t *testing.T, args ...string) (stdout, stderr string, err error) {
	t.Helper()
	var out, errOut bytes.Buffer
	err = executeRootWriting(t, &out, &errOut, args...)
	return out.String(), errOut.String(), err
}

// executeRootWriting은 rootCmd의 표준 출력을 out에, 표준 오류를 errOut에 쓰게 하고 rootCmd를 실행한다.
func executeRootWriting(t *testing.T, out, errOut io.Writer, args ...string) error {
	t.Helper()
	rootCmd.SetOut(out)
	rootCmd.SetErr(errOut)
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
	return rootCmd.Execute()
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

// flagGroups는 usage 출력을 "Flags:"처럼 콜론으로 끝나는 머리줄로 나누어, 머리줄마다 그 아래에 나온 flag의 긴 이름을 돌려준다.
func flagGroups(usage string) map[string][]string {
	groups := map[string][]string{}
	header := ""
	for _, line := range strings.Split(usage, "\n") {
		if strings.HasSuffix(line, ":") && !strings.HasPrefix(line, " ") {
			header = line
			continue
		}
		if _, after, found := strings.Cut(line, "--"); found && strings.HasPrefix(strings.TrimSpace(line), "-") {
			name, _, _ := strings.Cut(after, " ")
			groups[header] = append(groups[header], name)
		}
	}
	return groups
}

// setInstalledCommand는 rootCmd의 Use와 Version을 Execute가 정하는 값처럼 채워, -h와 -v가 실제 실행처럼 나오게 한다.
func setInstalledCommand(t *testing.T) {
	t.Helper()
	use, version := rootCmd.Use, rootCmd.Version
	rootCmd.Use, rootCmd.Version = "directory-watcher", "v0.0.0"
	t.Cleanup(func() { rootCmd.Use, rootCmd.Version = use, version })
	// cobra는 처음 실행할 때의 Use로 -h와 -v의 설명을 만든다. 앞선 테스트가 Use 없이 실행했으면 "for this command"로
	// 끝나므로, Execute가 Use를 정한 뒤 처음 실행하는 실제 실행과 같게 맞춘다.
	for _, name := range []string{"help", "version"} {
		if flag := rootCmd.Flags().Lookup(name); flag != nil {
			flag.Usage = name + " for directory-watcher"
		}
	}
}

func TestUsageShowsLogLevelInMaintenanceFlags(t *testing.T) {
	setInstalledCommand(t)
	want := map[string][]string{
		"Flags:":             {"config-path", "dry-run", "help", "silent", "version"},
		"Maintenance Flags:": {"log-level"},
	}
	for name, args := range map[string][]string{
		"--help":       {"--help"},
		"unknown flag": {"--no-such-flag"},
	} {
		t.Run(name, func(t *testing.T) {
			stdout, stderr, _ := executeRoot(t, args...)
			if got := flagGroups(stdout + stderr); !reflect.DeepEqual(got, want) {
				t.Fatalf("flag groups = %v, want %v; output:\n%s%s", got, want, stdout, stderr)
			}
		})
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

// writeMissingTargetConfig는 감시를 시작하지 못해 watcher를 바로 끝내는 감시 대상 하나를 적은 설정 파일을 path에 만든다.
// 감시 대상 경로에 path를 넣어서, 끝날 때의 오류로 어느 설정 파일을 읽었는지 알 수 있다.
func writeMissingTargetConfig(t *testing.T, path string) {
	t.Helper()
	data := fmt.Sprintf("watchTargets:\n  - path: %q\n", "missing-from-"+filepath.Base(path))
	if err := os.WriteFile(path, []byte(data), 0600); err != nil {
		t.Fatal(err)
	}
}

func TestConfigLookupWithoutConfigPath(t *testing.T) {
	cases := map[string]struct {
		files []string
		want  string
	}{
		"only config.directory-watcher.yml": {[]string{"config.directory-watcher.yml"}, "config.directory-watcher.yml"},
		"only config.yml":                   {[]string{"config.yml"}, "config.yml"},
		"both":                              {[]string{"config.directory-watcher.yml", "config.yml"}, "config.directory-watcher.yml"},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			t.Chdir(t.TempDir())
			for _, file := range tc.files {
				writeMissingTargetConfig(t, file)
			}

			stdout, _, err := executeRoot(t)

			if err == nil || !strings.Contains(err.Error(), "missing-from-"+tc.want) {
				t.Fatalf("Execute error = %v, want the missing target of %s", err, tc.want)
			}
			if line := "config: " + tc.want + "\n"; !strings.Contains(stdout, line) {
				t.Errorf("stdout lacks %q:\n%s", line, stdout)
			}
		})
	}
}

func TestNoConfigFileFound(t *testing.T) {
	guide, err := filepath.Abs(filepath.Join("..", "docs", "guide"))
	if err != nil {
		t.Fatal(err)
	}
	t.Chdir(t.TempDir())

	stdout, stderr, err := executeRoot(t)

	want := "Error: no config file found: tried config.directory-watcher.yml, config.yml (use -c to set a path)"
	if err == nil || !strings.Contains(stderr, want) {
		t.Fatalf("Execute error = %v, stderr:\n%s\nwant %q", err, stderr, want)
	}
	if strings.Contains(stdout, "config:") {
		t.Errorf("config line printed without a config file:\n%s", stdout)
	}
	// 설정 파일 문서 두 판은 이 오류 메시지를 옮겨 적은 것이다.
	for _, file := range []string{"configuration.md", "configuration.ko.md"} {
		data, readErr := os.ReadFile(filepath.Join(guide, file))
		if readErr != nil {
			t.Fatal(readErr)
		}
		if !strings.Contains(string(data), want) {
			t.Errorf("%s has no error example %q", file, want)
		}
	}
}

func TestConfigPathReadsOnlyThatPath(t *testing.T) {
	t.Chdir(t.TempDir())
	writeMissingTargetConfig(t, "config.directory-watcher.yml")
	writeMissingTargetConfig(t, "config.yml")

	stdout, _, err := executeRoot(t, "-c", "missing.yml")

	if !errors.Is(err, fs.ErrNotExist) || !strings.Contains(err.Error(), "missing.yml") {
		t.Fatalf("Execute error = %v, want missing.yml not to exist", err)
	}
	if strings.Contains(stdout, "config:") {
		t.Errorf("config line printed for a missing -c path:\n%s", stdout)
	}
}

func TestConfigLineComesBeforeConfigError(t *testing.T) {
	t.Chdir(t.TempDir())
	if err := os.WriteFile("config.yml", []byte("watchTargets:\n  - path: .\n    option:\n      excludeDir: [dist]\n"), 0600); err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer

	err := executeRootWriting(t, &output, &output)

	if err == nil {
		t.Fatal("Execute returned nil for a config error")
	}
	line, errLine := strings.Index(output.String(), "config: config.yml\n"), strings.Index(output.String(), "Error: ")
	if line < 0 || errLine < 0 || line > errLine {
		t.Fatalf("want the config line before the error line:\n%s", output.String())
	}
}

func TestConfigLinePrintingOptions(t *testing.T) {
	custom := filepath.Join("sub", "custom.yml")
	cases := map[string]struct {
		args []string
		want string
	}{
		"found":            {nil, "config: config.yml\n"},
		"-c":               {[]string{"-c", custom}, "config: " + custom + "\n"},
		"--config-path":    {[]string{"--config-path", custom}, "config: " + custom + "\n"},
		"-n":               {[]string{"-n"}, "config: config.yml\n"},
		"-s":               {[]string{"-s"}, ""},
		"-c with --silent": {[]string{"-c", custom, "--silent"}, ""},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			t.Chdir(t.TempDir())
			if err := os.Mkdir("sub", 0700); err != nil {
				t.Fatal(err)
			}
			writeMissingTargetConfig(t, "config.yml")
			writeMissingTargetConfig(t, custom)

			stdout, _, err := executeRoot(t, tc.args...)

			if err == nil || !strings.Contains(err.Error(), "missing-from-") {
				t.Fatalf("Execute error = %v, want the missing target error", err)
			}
			if tc.want == "" {
				if strings.Contains(stdout, "config:") {
					t.Errorf("config line printed:\n%s", stdout)
				}
			} else if !strings.Contains(stdout, tc.want) {
				t.Errorf("stdout lacks %q:\n%s", tc.want, stdout)
			}
		})
	}
}

// 설치 문서 두 판의 명령줄 옵션 블록은 directory-watcher --help의 출력을 옮겨 적은 것이다.
func TestInstallationGuideShowsHelpOutput(t *testing.T) {
	setInstalledCommand(t)
	help, _, err := executeRoot(t, "--help")
	if err != nil {
		t.Fatal(err)
	}
	for _, file := range []string{"installation.md", "installation.ko.md"} {
		data, err := os.ReadFile(filepath.Join("..", "docs", "guide", file))
		if err != nil {
			t.Fatal(err)
		}
		// Windows의 git은 문서의 줄 끝을 CRLF로 바꿔 받을 수 있으므로, LF로 바꾼 뒤 여러 줄을 비교한다.
		doc := strings.ReplaceAll(string(data), "\r\n", "\n")
		if !strings.Contains(doc, "```shell\n"+help+"```\n") {
			t.Errorf("%s has no shell block equal to the --help output:\n%s", file, help)
		}
	}
}
