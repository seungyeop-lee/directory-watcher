// Package guestrun defines one E2E run inside the Windows guest: the files
// that the Mac runner and the guest program exchange in the run directory,
// how the runner judges the result, and the tar format that carries
// directories between the two machines.
package guestrun

import (
	"archive/tar"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Files in the guest run directory. The directory name is the run ID.
const (
	RequestFile     = "request.json"
	SourceFile      = "source.tar"
	StartedFile     = "started.txt"
	ResultFile      = "result.json"
	StdoutFile      = "stdout.log"
	StderrFile      = "stderr.log"
	DiagnosticsFile = "diagnostics.tar"
)

// Request tells the guest program what to run. Args and Env are those of the
// go command; the guest program adds the run-specific temporary and
// diagnostics directories to Env.
type Request struct {
	Go           string
	Args         []string
	Env          []string
	Deadline     time.Duration
	SourceSHA256 string
}

// Result is written by the guest program once, after go has exited or been
// stopped and the diagnostics are archived.
type Result struct {
	RunID string
	// ExitCode is -1 when go did not start or was stopped at the deadline.
	ExitCode int
	TimedOut bool
	// Error reports a guest-side failure that is not a test failure, such as
	// a source checksum mismatch or a run directory outside a fixed disk.
	Error string
	// Leftovers lists processes started from the run directory that were
	// still alive after go exited. The guest program terminates them.
	Leftovers         []string
	Guest             Guest
	Started, Finished time.Time
}

// Guest records the environment the tests ran in.
type Guest struct {
	Windows   string
	GoVersion string
	GoEnv     string
	RunDir    string
	DriveType string
}

// Check parses a result file and returns an error unless it is the result of
// runID and the run passed. An empty file means the guest has not finished.
func Check(data []byte, runID string) (Result, error) {
	var result Result
	if len(data) == 0 {
		return result, fmt.Errorf("guest result %s is missing", ResultFile)
	}
	if err := json.Unmarshal(data, &result); err != nil {
		return result, fmt.Errorf("guest result is unreadable: %w", err)
	}
	switch {
	case result.RunID != runID:
		return result, fmt.Errorf("guest result belongs to run %q, not %q", result.RunID, runID)
	case result.Error != "":
		return result, fmt.Errorf("guest run failed: %s", result.Error)
	case result.TimedOut:
		return result, fmt.Errorf("go test did not finish within the deadline and was stopped")
	case result.ExitCode != 0:
		return result, fmt.Errorf("go test exited with %d", result.ExitCode)
	case len(result.Leftovers) > 0:
		return result, fmt.Errorf("processes outlived go test: %s", strings.Join(result.Leftovers, "; "))
	}
	return result, nil
}

// ArchiveDir writes the regular files under dir as a tar stream with paths
// relative to dir.
func ArchiveDir(out io.Writer, dir string) error {
	archive := tar.NewWriter(out)
	err := filepath.WalkDir(dir, func(path string, entry fs.DirEntry, err error) error {
		if err != nil || !entry.Type().IsRegular() {
			return err
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		header, err := tar.FileInfoHeader(info, "")
		if err != nil {
			return err
		}
		name, err := filepath.Rel(dir, path)
		if err != nil {
			return err
		}
		header.Name = filepath.ToSlash(name)
		if err := archive.WriteHeader(header); err != nil {
			return err
		}
		file, err := os.Open(path)
		if err != nil {
			return err
		}
		defer file.Close()
		_, err = io.Copy(archive, file)
		return err
	})
	if err != nil {
		return err
	}
	return archive.Close()
}

// Extract writes the regular files of a tar stream under dir. Other entry
// types and paths that leave dir are rejected.
func Extract(in io.Reader, dir string) error {
	archive := tar.NewReader(in)
	for {
		header, err := archive.Next()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		if !filepath.IsLocal(header.Name) {
			return fmt.Errorf("archive path leaves the target directory: %q", header.Name)
		}
		if header.Typeflag != tar.TypeReg {
			return fmt.Errorf("archive entry %q is not a regular file", header.Name)
		}
		path := filepath.Join(dir, filepath.FromSlash(header.Name))
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			return err
		}
		file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, header.FileInfo().Mode().Perm())
		if err != nil {
			return err
		}
		_, copyErr := io.Copy(file, archive)
		closeErr := file.Close()
		if copyErr != nil {
			return copyErr
		}
		if closeErr != nil {
			return closeErr
		}
	}
}
