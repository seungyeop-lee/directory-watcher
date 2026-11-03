package e2e

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/sys/windows"
)

func TestCleanupReportsLockedResources(t *testing.T) {
	if mode := os.Getenv("DW_E2E_CLEANUP_LOCK"); mode != "" {
		path := filepath.Dir(watcherBin)
		lockPath := watcherBin
		if mode == "scenario" {
			s := newScenario(t)
			path, lockPath = s.work, s.work
			handle := lockCleanupPath(t, lockPath)
			defer windows.CloseHandle(handle)
			if err := os.WriteFile(os.Getenv("DW_E2E_CLEANUP_LOCK_RECORD"), []byte(path), 0600); err != nil {
				t.Fatal(err)
			}
			s.cleanup()
			return
		}
		// The subprocess owns this handle until os.Exit, after TestMain cleanup.
		lockCleanupPath(t, lockPath)
		if err := os.WriteFile(os.Getenv("DW_E2E_CLEANUP_LOCK_RECORD"), []byte(path), 0600); err != nil {
			t.Fatal(err)
		}
		return
	}
	for _, tc := range []struct{ mode, message string }{
		{"scenario", "remove run directory:"},
		{"binaries", "remove test binaries:"},
	} {
		t.Run(tc.mode, func(t *testing.T) {
			record := filepath.Join(t.TempDir(), "directory")
			cmd := exec.Command(os.Args[0], "-test.run=^TestCleanupReportsLockedResources$", "-test.v")
			cmd.Env = append(os.Environ(), "DW_E2E_CLEANUP_LOCK="+tc.mode, "DW_E2E_CLEANUP_LOCK_RECORD="+record)
			out, err := cmd.CombinedOutput()
			var exit *exec.ExitError
			if !errors.As(err, &exit) || !strings.Contains(string(out), tc.message) {
				t.Errorf("cleanup failure was not consumer-visible: err=%v\n%s", err, out)
			}
			path, readErr := os.ReadFile(record)
			if readErr != nil {
				t.Fatal(readErr)
			}
			if err := os.RemoveAll(string(path)); err != nil {
				t.Fatalf("release locked fixture: %v", err)
			}
		})
	}
}

func lockCleanupPath(t *testing.T, path string) windows.Handle {
	t.Helper()
	handle, err := windows.CreateFile(windows.StringToUTF16Ptr(path), windows.FILE_READ_ATTRIBUTES, windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE, nil, windows.OPEN_EXISTING, windows.FILE_FLAG_BACKUP_SEMANTICS, 0)
	if err != nil {
		t.Fatal(err)
	}
	return handle
}
