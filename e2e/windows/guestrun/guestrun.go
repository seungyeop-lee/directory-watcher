// Package guestrun은 Windows 게스트 안에서 하는 E2E 실행 한 번을 정의한다. Mac
// 실행기와 게스트 프로그램이 실행 디렉터리에서 주고받는 파일, 실행기가 결과를
// 판정하는 방법, 두 기기 사이에서 디렉터리를 옮기는 tar 형식을 정한다.
package guestrun

import (
	"archive/tar"
	"crypto/sha256"
	"encoding/hex"
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
	ProgramFile     = "guest.exe"
	ResultFile      = "result.json"
	StdoutFile      = "stdout.log"
	StderrFile      = "stderr.log"
	DiagnosticsFile = "diagnostics.tar"
)

// StartedSuffix를 실행 디렉터리 경로 뒤에 붙인 경로가 시작 표시 파일이다. 게스트
// 프로그램은 시작하자마자 이 파일에 실행 ID를 쓴다. 시작 표시 파일은 실행 디렉터리
// 밖에 둔다. 2026-11-05에 실행기 세 개가 같은 VM을 함께 쓸 때, Guest Agent가 실행
// 디렉터리 안에 있던 시작 표시 파일을 연 채로 남겨서 그 실행 디렉터리를 지우지
// 못했다(e2e/windows의 pullLimit 참고).
const StartedSuffix = ".started"

// CollectedFiles는 실행기가 결과를 받은 뒤 Mac으로 가져가는 파일이다.
var CollectedFiles = []string{StdoutFile, StderrFile, DiagnosticsFile}

// Request는 게스트 프로그램에 무엇을 실행할지 알려 준다. Args와 Env는 go 명령에
// 넘기는 값이며, 게스트 프로그램은 이번 실행 전용 임시 디렉터리와 진단 디렉터리를
// Env에 더한다.
type Request struct {
	// RunID가 있어서, 같은 소스와 인자로 동시에 도는 두 실행의 요청도 내용이 다르다.
	RunID    string
	Go       string
	Args     []string
	Env      []string
	Deadline time.Duration
	// SHA256은 실행기가 요청과 함께 보낸 파일의 SHA-256을 파일 이름별로 적는다. 게스트
	// 프로그램은 테스트를 시작하기 전에 Verify로 이 값과 파일을 대조한다.
	SHA256 map[string]string
}

// Verify는 요청이 runID의 것이고, 요청에 적힌 파일이 모두 dir에 같은 내용으로 있을 때만
// nil을 돌려준다. utmctl file push는 Guest Agent가 다른 요청에 준 file handle에 쓸 수
// 있어서, 보낸 파일이 비거나 다른 실행의 내용을 담을 수 있다.
func (req Request) Verify(dir, runID string) error {
	if req.RunID != runID {
		return fmt.Errorf("request belongs to run %q, not %q", req.RunID, runID)
	}
	for name, want := range req.SHA256 {
		data, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			return err
		}
		if got := Digest(data); got != want {
			return fmt.Errorf("%s has SHA-256 %s, not %s from the request (%d bytes received; "+
				"utmctl file push may have written it to another guest file)", name, got, want, len(data))
		}
	}
	return nil
}

// Digest는 Request와 Result에 적는 SHA-256(16진수)을 구한다.
func Digest(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// Result는 go가 끝나거나 멈춘 뒤 진단 자료를 아카이브로 묶고 나서, 게스트 프로그램이
// 한 번 쓴다.
type Result struct {
	RunID string
	// ExitCode는 go가 시작하지 못했거나 deadline에 멈췄을 때 -1이다.
	ExitCode int
	TimedOut bool
	// Error는 테스트 실패가 아닌 게스트 쪽 실패를 알린다. 보낸 파일이 요청과
	// 다르거나(Request.Verify) 실행 디렉터리가 고정 디스크 밖에 있는 경우가 여기에 해당한다.
	Error string
	// Leftovers는 실행 디렉터리에서 시작되어 go가 끝난 뒤에도 살아 있던 프로세스를
	// 나열한다. 게스트 프로그램은 이 프로세스들을 종료한다.
	Leftovers []string
	// SHA256은 CollectedFiles 가운데 게스트에 있는 파일의 SHA-256을 파일 이름별로 적는다.
	// 실행기는 가져온 파일의 내용을 이 값과 대조하고, 이 값이 없는 파일은 게스트에 없던
	// 파일로 보아 가져오지 않는다.
	SHA256            map[string]string
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

// Decode는 결과 파일을 해석해, 그 파일이 runID의 결과일 때만 돌려주고 그 밖에는 오류를
// 돌려준다. 빈 파일은 게스트가 아직 끝나지 않았다는 뜻이다.
func Decode(data []byte, runID string) (Result, error) {
	var result Result
	if len(data) == 0 {
		return result, fmt.Errorf("guest result %s is missing", ResultFile)
	}
	if err := json.Unmarshal(data, &result); err != nil {
		return result, fmt.Errorf("guest result is unreadable: %w", err)
	}
	if result.RunID != runID {
		return result, fmt.Errorf("guest result belongs to run %q, not %q", result.RunID, runID)
	}
	return result, nil
}

// Check는 실행이 통과했을 때만 nil을 돌려주고 그 밖에는 오류를 돌려준다.
func (result Result) Check() error {
	switch {
	case result.Error != "":
		return fmt.Errorf("guest run failed: %s", result.Error)
	case result.TimedOut:
		return fmt.Errorf("go test did not finish within the deadline and was stopped")
	case result.ExitCode != 0:
		return fmt.Errorf("go test exited with %d", result.ExitCode)
	case len(result.Leftovers) > 0:
		return fmt.Errorf("processes outlived go test: %s", strings.Join(result.Leftovers, "; "))
	}
	return nil
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
