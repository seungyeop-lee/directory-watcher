package guestrun

import (
	"archive/tar"
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func encode(t *testing.T, r Result) []byte {
	t.Helper()
	data, err := json.Marshal(r)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func TestDecodeAcceptsOnlyAResultOfTheRun(t *testing.T) {
	if result, err := Decode(encode(t, Result{RunID: "run-a", ExitCode: 1}), "run-a"); err != nil || result.ExitCode != 1 {
		t.Fatalf("Decode = %+v, %v; want the result of run-a", result, err)
	}
	for name, tc := range map[string]struct {
		data []byte
		want string
	}{
		"missing":   {nil, "missing"},
		"truncated": {[]byte(`{"RunID":"run-a"`), "unreadable"},
		"other run": {encode(t, Result{RunID: "run-b"}), `belongs to run "run-b"`},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := Decode(tc.data, "run-a")
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("Decode error = %v, want it to mention %q", err, tc.want)
			}
		})
	}
}

func TestCheckAcceptsOnlyAPassedResult(t *testing.T) {
	if err := (Result{RunID: "run-a"}).Check(); err != nil {
		t.Fatalf("passed result rejected: %v", err)
	}
	for name, tc := range map[string]struct {
		result Result
		want   string
	}{
		"guest failure": {Result{ExitCode: -1, Error: "source checksum mismatch"}, "source checksum mismatch"},
		"timeout":       {Result{ExitCode: -1, TimedOut: true}, "deadline"},
		"test failure":  {Result{ExitCode: 1}, "exited with 1"},
		"leftovers":     {Result{Leftovers: []string{"pid=7 watcher.exe"}}, "pid=7 watcher.exe"},
	} {
		t.Run(name, func(t *testing.T) {
			err := tc.result.Check()
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("Check error = %v, want it to mention %q", err, tc.want)
			}
		})
	}
}

// TestRequestVerifyAcceptsOnlyFilesSentForTheRun은 utmctl file push가 다른 요청이 연
// 파일에 쓴 경우를 흉내 낸다. 그러면 이번 실행의 파일은 비거나 다른 실행이 보낸 내용을 담는다.
func TestRequestVerifyAcceptsOnlyFilesSentForTheRun(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "run-a")
	if err := os.Mkdir(dir, 0755); err != nil {
		t.Fatal(err)
	}
	write := func(name, content string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
	}
	write(SourceFile, "own source")
	write(ProgramFile, "")
	sent := map[string]string{SourceFile: Digest([]byte("own source"))}
	if err := (Request{RunID: "run-a", SHA256: sent}).Verify(dir, "run-a"); err != nil {
		t.Fatalf("Verify = %v; want the files sent for run-a accepted", err)
	}
	for name, tc := range map[string]struct {
		request Request
		want    string
	}{
		"other run":    {Request{RunID: "run-b", SHA256: sent}, `belongs to run "run-b"`},
		"empty file":   {Request{RunID: "run-a", SHA256: map[string]string{ProgramFile: Digest([]byte("own program"))}}, ProgramFile},
		"missing file": {Request{RunID: "run-a", SHA256: map[string]string{StdoutFile: Digest(nil)}}, StdoutFile},
	} {
		t.Run(name, func(t *testing.T) {
			err := tc.request.Verify(dir, "run-a")
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("Verify error = %v, want it to mention %q", err, tc.want)
			}
		})
	}
}

func TestArchiveDirRoundTripsThroughExtract(t *testing.T) {
	src := t.TempDir()
	if err := os.MkdirAll(filepath.Join(src, "run", "nested"), 0755); err != nil {
		t.Fatal(err)
	}
	files := map[string]string{"top.log": "한글 로그", filepath.Join("run", "nested", "records.log"): "start|1"}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(src, name), []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
	}
	var data bytes.Buffer
	if err := ArchiveDir(&data, src); err != nil {
		t.Fatal(err)
	}
	dst := t.TempDir()
	if err := Extract(&data, dst); err != nil {
		t.Fatal(err)
	}
	for name, content := range files {
		got, err := os.ReadFile(filepath.Join(dst, name))
		if err != nil || string(got) != content {
			t.Fatalf("%s = %q, %v; want %q", name, got, err, content)
		}
	}
}

func TestExtractRejectsPathsLeavingTheTarget(t *testing.T) {
	var data bytes.Buffer
	archive := tar.NewWriter(&data)
	if err := archive.WriteHeader(&tar.Header{Name: "../escape", Mode: 0644, Typeflag: tar.TypeReg}); err != nil {
		t.Fatal(err)
	}
	if err := archive.Close(); err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	if err := Extract(&data, filepath.Join(dir, "target")); err == nil {
		t.Fatal("Extract accepted ../escape")
	}
	if _, err := os.Stat(filepath.Join(dir, "escape")); !os.IsNotExist(err) {
		t.Fatalf("escape file exists: %v", err)
	}
}

func TestExtractRejectsSymlinks(t *testing.T) {
	var data bytes.Buffer
	archive := tar.NewWriter(&data)
	if err := archive.WriteHeader(&tar.Header{Name: "src/.agents/skills", Linkname: "../.claude/skills", Mode: 0777, Typeflag: tar.TypeSymlink}); err != nil {
		t.Fatal(err)
	}
	if err := archive.Close(); err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	if err := Extract(&data, dir); err == nil || !strings.Contains(err.Error(), "is not a regular file") {
		t.Fatalf("Extract = %v; want the symlink entry rejected", err)
	}
	if _, err := os.Lstat(filepath.Join(dir, "src", ".agents", "skills")); !os.IsNotExist(err) {
		t.Fatalf("symlink entry was extracted: %v", err)
	}
}
