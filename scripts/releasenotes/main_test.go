package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const changelog = `# Changelog

## [Unreleased]

### Changed

- Unreleased change.

## [3.0.0] - 2026-11-04

### Changed

- Hook commands run in a shell.

### Fixed

- Stopping a hook stops every process.

## [2.0.0] - 2026-01-01

- Older change.
`

func runOn(t *testing.T, content string, args ...string) (code int, stdout, stderr string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "CHANGELOG.md")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	var out, errOut bytes.Buffer
	code = run(path, args, &out, &errOut)
	return code, out.String(), errOut.String()
}

func TestPrintsSectionBodyUpToNextSection(t *testing.T) {
	code, stdout, stderr := runOn(t, changelog, "3.0.0")
	want := "### Changed\n\n- Hook commands run in a shell.\n\n### Fixed\n\n- Stopping a hook stops every process.\n"
	if code != 0 || stdout != want {
		t.Fatalf("code=%d stdout=%q stderr=%q, want code 0 and stdout %q", code, stdout, stderr, want)
	}
}

func TestPrintsLastSectionAndAcceptsCRLF(t *testing.T) {
	code, stdout, stderr := runOn(t, strings.ReplaceAll(changelog, "\n", "\r\n"), "2.0.0")
	if want := "- Older change.\n"; code != 0 || stdout != want {
		t.Fatalf("code=%d stdout=%q stderr=%q, want code 0 and stdout %q", code, stdout, stderr, want)
	}
}

func TestFailsWithoutPublishableSection(t *testing.T) {
	tests := []struct {
		name, content, version, message string
	}{
		{"missing section", changelog, "3.0.1", "has no ## [3.0.1] section"},
		{"empty body", "## [3.0.0] - 2026-11-04\n\n\n## [2.0.0] - 2026-01-01\n\n- Older change.\n", "3.0.0", "section is empty"},
		{"no date", "## [3.0.0]\n\n- Change.\n", "3.0.0", `but it ends with ""`},
		{"unreleased instead of date", "## [3.0.0] - Unreleased\n\n- Change.\n", "3.0.0", `but it ends with "Unreleased"`},
		{"invalid date", "## [3.0.0] - 2026-13-01\n\n- Change.\n", "3.0.0", `but it ends with "2026-13-01"`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			code, stdout, stderr := runOn(t, tt.content, tt.version)
			if code == 0 || stdout != "" || !strings.Contains(stderr, tt.message) {
				t.Fatalf("code=%d stdout=%q stderr=%q, want non-zero code, no stdout, and stderr containing %q", code, stdout, stderr, tt.message)
			}
		})
	}
}

func TestFailsWithoutVersionArgument(t *testing.T) {
	code, stdout, _ := runOn(t, changelog)
	if code == 0 || stdout != "" {
		t.Fatalf("code=%d stdout=%q, want non-zero code and no stdout", code, stdout)
	}
}
