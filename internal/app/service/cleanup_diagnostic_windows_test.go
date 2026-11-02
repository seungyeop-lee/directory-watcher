package service

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
	"unsafe"

	"github.com/fsnotify/fsnotify"
	"github.com/seungyeop-lee/directory-watcher/v2/internal/app/domain"
	"golang.org/x/sys/windows"
)

type diagnosticHandle struct {
	Object uintptr
	PID uintptr
	Handle uintptr
	Access uint32
	Trace uint16
	Type uint16
	Attributes uint32
	Reserved uint32
}

func diagnosticOwners(t *testing.T, path string, all bool) {
	t.Helper()
	query := windows.NewLazySystemDLL("ntdll.dll").NewProc("NtQuerySystemInformation")
	buffer := make([]byte, 1<<20)
	for {
		var needed uint32
		status, _, _ := query.Call(64, uintptr(unsafe.Pointer(&buffer[0])), uintptr(len(buffer)), uintptr(unsafe.Pointer(&needed)))
		if status == 0 { break }
		if uint32(status) != 0xc0000004 { t.Logf("handle snapshot status=%x", status); return }
		buffer = make([]byte, int(needed)+1<<20)
	}
	count := *(*uintptr)(unsafe.Pointer(&buffer[0]))
	entries := unsafe.Slice((*diagnosticHandle)(unsafe.Pointer(&buffer[2*unsafe.Sizeof(uintptr(0))])), count)
	prefix := strings.ToLower(filepath.Clean(path))
	matches, denied := 0, 0
	for _, entry := range entries {
		if !all && entry.PID != uintptr(os.Getpid()) { continue }
		process, err := windows.OpenProcess(windows.PROCESS_DUP_HANDLE, false, uint32(entry.PID))
		if err != nil { denied++; continue }
		var duplicate windows.Handle
		err = windows.DuplicateHandle(process, windows.Handle(entry.Handle), windows.CurrentProcess(), &duplicate, 0, false, windows.DUPLICATE_SAME_ACCESS)
		windows.CloseHandle(process)
		if err != nil { continue }
		kind, err := windows.GetFileType(duplicate)
		if err == nil && kind == windows.FILE_TYPE_DISK {
			name := make([]uint16, 32768)
			n, err := windows.GetFinalPathNameByHandle(duplicate, &name[0], uint32(len(name)), 0)
			if err == nil && n < uint32(len(name)) {
				resolved := strings.TrimPrefix(windows.UTF16ToString(name[:n]), `\\?\`)
				if strings.HasPrefix(strings.ToLower(resolved), prefix) {
					matches++
					t.Logf("OWNER pid=%d self=%t handle=%x object=%x access=%x path=%q", entry.PID, entry.PID == uintptr(os.Getpid()), entry.Handle, entry.Object, entry.Access, resolved)
				}
			}
		}
		windows.CloseHandle(duplicate)
	}
	t.Logf("owner snapshot path=%q matches=%d inaccessible-handles=%d total=%d", path, matches, denied, count)
}

func diagnosticRemove(t *testing.T, path string, started time.Time) {
	t.Helper()
	before := time.Now()
	err := os.RemoveAll(path)
	t.Logf("REMOVE path=%q since-start=%s duration=%s err=%v", path, time.Since(started), time.Since(before), err)
	if err != nil {
		diagnosticOwners(t, path, true)
		t.Errorf("initial resource deletion failed: %v", err)
	}
}

func TestWindowsCleanupDiagnostic(t *testing.T) {
	for _, adds := range []int{1, 2} {
		for _, consume := range []bool{false, true} {
			t.Run(fmt.Sprintf("raw/adds=%d/consume=%t", adds, consume), func(t *testing.T) {
				path := t.TempDir()
				started := time.Now()
				w, err := fsnotify.NewWatcher()
				if err != nil { t.Fatal(err) }
				if consume {
					go func() { for range w.Events {} }()
					go func() { for range w.Errors {} }()
				}
				for range adds { if err := w.Add(path); err != nil { t.Fatal(err) } }
				diagnosticOwners(t, path, false)
				before := time.Now()
				err = w.Close()
				t.Logf("CLOSE raw since-start=%s duration=%s err=%v", time.Since(started), time.Since(before), err)
				diagnosticRemove(t, path, started)
			})
		}
	}
	for _, recursive := range []bool{false, true} {
		for _, source := range []string{"hybrid", "hybrid-drained", "errors-only", "native", "actual-input"} {
			t.Run(fmt.Sprintf("service/recursive=%t/source=%s", recursive, source), func(t *testing.T) {
				path := t.TempDir()
				started := time.Now()
				r := NewWatchTargetRunner(domain.GlobalCommandSet{}, domain.WatchTargetsCommandSet{Path: domain.Path(path), Option: domain.WatchTargetsOption{NoWait:true, WatchSubDir:recursive, WatchEvent:domain.WatchEvent{Create:true}}}, newMockLogger())
				ready, changed := make(chan struct{}), make(chan struct{}, 8)
				r.runStartHook = func() { close(ready) }
				r.runFinishHook = func() {}
				r.runChangeHook = func(domain.Event) { changed <- struct{}{} }
				if source == "hybrid" || source == "hybrid-drained" {
					controlledSource(r)
					if source == "hybrid-drained" {
						go func() { for range r.watcher.Events {} }()
						go func() { for range r.watcher.Errors {} }()
					}
				}
				if source == "errors-only" { r.watchErrors = make(chan error) }
				result := make(chan error, 1)
				go func() { result <- r.Run() }()
				select { case <-ready: case err := <-result: t.Fatalf("run before ready: %v", err); case <-time.After(10*time.Second): t.Fatal("readiness timeout") }
				if source == "actual-input" {
					if err := os.WriteFile(filepath.Join(path,"actual.txt"), []byte("event"), 0600); err != nil { t.Fatal(err) }
					select { case <-changed: case <-time.After(10*time.Second): t.Fatal("actual event timeout") }
				}
				diagnosticOwners(t, path, false)
				before := time.Now()
				var wg sync.WaitGroup
				wg.Add(1)
				r.Stop(&wg)
				t.Logf("STOP source=%s since-start=%s duration=%s run=%v", source, time.Since(started), time.Since(before), <-result)
				diagnosticRemove(t, path, started)
			})
		}
	}
}
