//go:build darwin

package main

import (
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"
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

// shortRetryInterval은 테스트 동안 재시도 간격과 결과를 다시 확인하는 간격을 줄인다.
func shortRetryInterval(t *testing.T) {
	t.Helper()
	retry, result := retryInterval, resultInterval
	t.Cleanup(func() { retryInterval, resultInterval = retry, result })
	retryInterval, resultInterval = 10*time.Millisecond, 10*time.Millisecond
}

// shortWaits는 테스트 동안 재시도 간격과 제한 시간을 줄인다. 실패를 보고하는 테스트는
// 제한 시간이 끝날 때까지 재시도하므로 이 제한 시간만큼 걸린다. 테스트가 처음 실행하는
// 가짜 utmctl은 macOS에서 시작하는 데 200ms 넘게 걸리기도 해서 첫 시도 중에 제한 시간이
// 끝날 수 있으므로, 재시도한 끝에 성공해야 하는 테스트는 shortRetryInterval만 쓴다.
func shortWaits(t *testing.T) {
	t.Helper()
	shortRetryInterval(t)
	remove, dir, start, started, grace := removeLimit, guestDirLimit, programStartLimit, startedLimit, resultGrace
	t.Cleanup(func() {
		removeLimit, guestDirLimit, programStartLimit, startedLimit, resultGrace = remove, dir, start, started, grace
	})
	const limit = 200 * time.Millisecond
	removeLimit, guestDirLimit, programStartLimit, startedLimit, resultGrace = limit, limit, limit, limit, limit
}
