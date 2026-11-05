package harness

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/sys/windows"
)

// TestMain은 e2e/scenarios가 쓰는 것과 같은 Run을 거치므로, 이 테스트의 하위
// 프로세스는 실제 바이너리 빌드와 삭제를 거친다. 바이너리가 필요한 것은 이 Windows
// 테스트뿐이며, 다른 플랫폼은 빌드를 건너뛴다.
func TestMain(m *testing.M) { os.Exit(Run(m)) }

func TestCleanupReportsLockedResources(t *testing.T) {
	skipShort(t)
	if mode := os.Getenv("DW_E2E_CLEANUP_LOCK"); mode != "" {
		path := filepath.Dir(watcherBin)
		lockPath := watcherBin
		if mode == "scenario" {
			s := NewScenario(t)
			path, lockPath = s.Work, s.Work
			for _, entry := range []string{s.Watch, s.Records} {
				if err := os.Remove(entry); err != nil {
					t.Fatal(err)
				}
			}
			handle := lockCleanupPath(t, lockPath)
			defer windows.CloseHandle(handle)
			if err := os.WriteFile(os.Getenv("DW_E2E_CLEANUP_LOCK_RECORD"), []byte(path), 0600); err != nil {
				t.Fatal(err)
			}
			s.cleanup()
			return
		}
		// 하위 프로세스는 TestMain 정리가 끝난 뒤의 os.Exit까지 이 handle을 소유한다.
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
			cmd.Env = append(os.Environ(), "DW_E2E_KEEP=0", "DW_E2E_CLEANUP_LOCK="+tc.mode, "DW_E2E_CLEANUP_LOCK_RECORD="+record)
			out, err := cmd.CombinedOutput()
			output := string(out)
			locked := strings.Contains(output, "deny-delete positive control passed:")
			if !locked {
				t.Errorf("locking precondition was not verified: err=%v\n%s", err, output)
			}
			var exit *exec.ExitError
			visible := errors.As(err, &exit) && strings.Contains(output, tc.message)
			if !visible {
				t.Errorf("cleanup failure was not consumer-visible: err=%v\n%s", err, output)
			}
			if locked && visible {
				t.Logf("deny-delete positive control passed: %s; cleanup failure verified", tc.mode)
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
	handle, err := windows.CreateFile(windows.StringToUTF16Ptr(path), windows.GENERIC_READ, windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE, nil, windows.OPEN_EXISTING, windows.FILE_FLAG_BACKUP_SEMANTICS, 0)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(path); !errors.Is(err, windows.ERROR_SHARING_VIOLATION) {
		windows.CloseHandle(handle)
		t.Fatalf("deny-delete fixture did not block removal with sharing violation: %v", err)
	}
	t.Logf("deny-delete positive control passed: %s path=%q access=GENERIC_READ share=READ|WRITE", os.Getenv("DW_E2E_CLEANUP_LOCK"), path)
	return handle
}
