package harness

import (
	"strings"
	"testing"
	"time"
)

func TestDLLInitFailureReportListsSignalsAroundFailure(t *testing.T) {
	log := strings.Join([]string{
		"[DEBUG]             | sent CTRL+BREAK to process group 300 at 2026-11-05T03:00:00.9Z",
		"[DEBUG]             | process 300 exited with code 0x1 (started 2026-11-05T03:00:00.5Z, exited 2026-11-05T03:00:01Z)",
		"[DEBUG]             | process 400 exited with code 0x0 (started 2026-11-05T03:00:01.1Z, exited 2026-11-05T03:00:01.2Z)",
		"[DEBUG]             | sent CTRL+BREAK to process group 200 at 2026-11-05T03:00:02.005Z (error: The parameter is incorrect.)",
		"[DEBUG]             | process 200 exited with code 0xc0000142 (started 2026-11-05T03:00:02Z, exited 2026-11-05T03:00:02.01Z)",
		"[ERROR]             | onChange hook command failed, skipping remaining commands: exit status 3221225794",
	}, "\n")
	harness := []consoleEvent{{at: time.Date(2026, 11, 5, 3, 0, 3, 0, time.UTC), signal: true, pid: 100, from: "harness"}}

	got := formatDLLInitFailureReport(log, "watcher:100", harness)

	want := strings.Join([]string{
		"STATUS_DLL_INIT_FAILED (0xc0000142): CTRL+BREAK sent in this console since the scenario started, and exits of the failed processes and of signal targets, in time order:",
		"2026-11-05T03:00:00.9Z CTRL+BREAK pid=300 from=watcher:100",
		"2026-11-05T03:00:01Z exit pid=300 code=0x1 started=2026-11-05T03:00:00.5Z from=watcher:100",
		"2026-11-05T03:00:02.005Z CTRL+BREAK pid=200 from=watcher:100 error=The parameter is incorrect. target=failed-process",
		"2026-11-05T03:00:02.01Z exit pid=200 code=0xc0000142 started=2026-11-05T03:00:02Z from=watcher:100",
		"2026-11-05T03:00:03Z CTRL+BREAK pid=100 from=harness",
		"",
	}, "\n")
	if got != want {
		t.Fatalf("report =\n%s\nwant\n%s", got, want)
	}
}

func TestDLLInitFailureReportIsEmptyWithoutFailure(t *testing.T) {
	log := "[DEBUG]             | sent CTRL+BREAK to process group 300 at 2026-11-05T03:00:00.9Z\n" +
		"[DEBUG]             | process 300 exited with code 0x1 (started 2026-11-05T03:00:00.5Z, exited 2026-11-05T03:00:01Z)\n"
	harness := []consoleEvent{{at: time.Date(2026, 11, 5, 3, 0, 3, 0, time.UTC), signal: true, pid: 100, from: "harness"}}

	if got := formatDLLInitFailureReport(log, "watcher:100", harness); got != "" {
		t.Fatalf("report = %q, want none", got)
	}
}

// DEBUG보다 낮은 수준으로 실행한 watcher는 종료 기록 없이 hook 실패 오류만 남긴다.
func TestDLLInitFailureReportNotesFailureWithoutExitRecord(t *testing.T) {
	log := "[ERROR]             | onChange hook command failed, skipping remaining commands: exit status 3221225794\n"
	harness := []consoleEvent{{at: time.Date(2026, 11, 5, 3, 0, 3, 0, time.UTC), signal: true, pid: 100, from: "harness"}}

	got := formatDLLInitFailureReport(log, "watcher:100", harness)

	want := strings.Join([]string{
		"STATUS_DLL_INIT_FAILED (0xc0000142): CTRL+BREAK sent in this console since the scenario started, and exits of the failed processes and of signal targets, in time order:",
		"watcher:100 logged exit status 3221225794 without exit records; run it with --log-level=DEBUG to record exit times and the CTRL+BREAK it sends",
		"2026-11-05T03:00:03Z CTRL+BREAK pid=100 from=harness",
		"",
	}, "\n")
	if got != want {
		t.Fatalf("report =\n%s\nwant\n%s", got, want)
	}
}
