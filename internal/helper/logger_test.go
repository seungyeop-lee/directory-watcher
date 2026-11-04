package helper

import (
	"bytes"
	"fmt"
	"testing"
)

// logLine은 slog로 바꾸기 전 Formatter가 쓰던 로그 한 줄을 그대로 만든다.
// 접두어는 ANSI 색 코드까지 포함한 길이로 20칸을 채운다.
func logLine(colorCode, prefix, message string) string {
	colored := func(s string) string { return "\033[" + colorCode + "m" + s + "\033[0m" }
	return fmt.Sprintf("%-20s%s%s\n", colored(prefix), colored(" | "), message)
}

func errorLine(message string) string { return logLine("31", "[ERROR]", message) }
func infoLine(message string) string  { return logLine("37", "[INFO]", message) }
func debugLine(message string) string { return logLine("37;1", "[DEBUG]", message) }

func TestLoggerWritesOnlyMessagesAtOrAboveLogLevel(t *testing.T) {
	for _, tc := range []struct {
		logLevel LogLevelString
		want     string
	}{
		{"ERROR", errorLine("error message")},
		{"INFO", infoLine("info message") + errorLine("error message")},
		{"DEBUG", debugLine("debug message") + infoLine("info message") + errorLine("error message")},
		{"WARN", errorLine("error message")},
		{"", errorLine("error message")},
	} {
		t.Run(string(tc.logLevel), func(t *testing.T) {
			var out bytes.Buffer
			logger := newLogger(tc.logLevel.GetLogLevel(), &out)

			logger.Debug("debug message")
			logger.Info("info message")
			logger.Error("error message")

			if got := out.String(); got != tc.want {
				t.Fatalf("output mismatch\n got: %q\nwant: %q", got, tc.want)
			}
		})
	}
}

func TestLoggerPrefixesOnlyFirstLineOfMultilineMessage(t *testing.T) {
	var out bytes.Buffer
	logger := newLogger(DEBUG, &out)
	message := "{\n  \"key\": \"value\"\n}"

	logger.Debug(message)

	if got, want := out.String(), debugLine(message); got != want {
		t.Fatalf("output mismatch\n got: %q\nwant: %q", got, want)
	}
}

func TestLoggerWritesEachMessageInOneWrite(t *testing.T) {
	out := &writeRecorder{}
	logger := newLogger(DEBUG, out)

	logger.Info("first\nsecond")

	if want := []string{infoLine("first\nsecond")}; fmt.Sprint(out.writes) != fmt.Sprint(want) {
		t.Fatalf("writes mismatch\n got: %q\nwant: %q", out.writes, want)
	}
}

func TestLoggerSkipsEmptyErrorMessage(t *testing.T) {
	var out bytes.Buffer
	logger := newLogger(DEBUG, &out)

	logger.Error("")

	if got := out.String(); got != "" {
		t.Fatalf("output = %q, want empty", got)
	}
}

// writeRecorder는 Write가 불린 횟수와 각 호출의 바이트를 기록한다.
type writeRecorder struct {
	writes []string
}

func (w *writeRecorder) Write(p []byte) (int, error) {
	w.writes = append(w.writes, string(p))
	return len(p), nil
}
