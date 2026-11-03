// Package guestrun은 Windows 게스트 안에서 하는 E2E 실행 한 번을 정의한다. Mac
// 실행기와 게스트 프로그램이 실행 디렉터리에서 주고받는 파일, 실행기가 결과를
// 판정하는 방법, 두 기기 사이에서 디렉터리를 옮기는 tar 형식을 정한다.
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

// 게스트 실행 디렉터리 안에 두는 파일이다. 디렉터리 이름은 실행 ID다.
const (
	RequestFile     = "request.json"
	SourceFile      = "source.tar"
	StartedFile     = "started.txt"
	ResultFile      = "result.json"
	StdoutFile      = "stdout.log"
	StderrFile      = "stderr.log"
	DiagnosticsFile = "diagnostics.tar"
)

// Request는 게스트 프로그램에 무엇을 실행할지 알려 준다. Args와 Env는 go 명령에
// 넘기는 값이며, 게스트 프로그램은 이번 실행 전용 임시 디렉터리와 진단 디렉터리를
// Env에 더한다.
type Request struct {
	Go           string
	Args         []string
	Env          []string
	Deadline     time.Duration
	SourceSHA256 string
}

// Result는 go가 끝나거나 멈춘 뒤 진단 자료를 아카이브로 묶고 나서, 게스트 프로그램이
// 한 번 쓴다.
type Result struct {
	RunID string
	// ExitCode는 go가 시작하지 못했거나 deadline에 멈췄을 때 -1이다.
	ExitCode int
	TimedOut bool
	// Error는 테스트 실패가 아닌 게스트 쪽 실패를 알린다. 소스 체크섬이
	// 맞지 않거나 실행 디렉터리가 고정 디스크 밖에 있는 경우가 여기에 해당한다.
	Error string
	// Leftovers는 실행 디렉터리에서 시작되어 go가 끝난 뒤에도 살아 있던 프로세스를
	// 나열한다. 게스트 프로그램은 이 프로세스들을 종료한다.
	Leftovers         []string
	Guest             Guest
	Started, Finished time.Time
}

// Guest는 테스트를 실행한 환경을 기록한다.
type Guest struct {
	Windows   string
	GoVersion string
	GoEnv     string
	RunDir    string
	DriveType string
}

// Check는 결과 파일을 해석해, 그 파일이 runID의 결과이고 실행이 통과했을 때만 nil을
// 돌려주고 그 밖에는 오류를 돌려준다. 빈 파일은 게스트가 아직 끝나지 않았다는 뜻이다.
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

// ArchiveDir는 dir 아래의 일반 파일을 dir 기준 상대 경로로 담아 tar 스트림으로
// 쓴다.
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

// Extract는 tar 스트림에 담긴 일반 파일을 dir 아래에 쓴다. 다른 종류의 항목과 dir
// 밖으로 나가는 경로는 거부한다.
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
