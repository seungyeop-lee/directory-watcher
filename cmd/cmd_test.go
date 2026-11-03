package cmd

import (
	"bytes"
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
