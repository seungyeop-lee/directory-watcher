//go:build darwin

package main

import (
	"io"
	"os"
	"path/filepath"
	"testing"
)

// fakeUTMCtl은 $@로 받은 인자로 script를 실행하는 utmctl 대체 파일을 쓴다.
func fakeUTMCtl(t *testing.T, script string) utm {
	t.Helper()
	return utm{ctl: fakeCommand(t, "utmctl", script), log: io.Discard}
}

func fakeCommand(t *testing.T, name, script string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"+script), 0755); err != nil {
		t.Fatal(err)
	}
	return path
}
