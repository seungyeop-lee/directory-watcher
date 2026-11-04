package helper

import (
	"context"
	"io"
	"log/slog"

	"github.com/moby/term"
)

const LogLevelStringDefaultValue = "ERROR"

// logOut은 os.Stdout이 아니라 term.StdStreams의 stdout이다. ANSI 색 코드를 직접 해석하지 못하는 Windows 콘솔에서는 이 writer가 색 코드를 처리한다.
var logOut io.Writer

func init() {
	_, logOut, _ = term.StdStreams()
}

var GlobalLogger Logger

type LogLevelString string

func (l LogLevelString) GetLogLevel() LogLevel {
	switch l {
	case "ERROR":
		return ERROR
	case "INFO":
		return INFO
	case "DEBUG":
		return DEBUG
	default:
		return ERROR
	}
}

type LogLevel uint

const (
	ERROR LogLevel = iota
	INFO
	DEBUG
)

func (l LogLevel) slogLevel() slog.Level {
	switch l {
	case DEBUG:
		return slog.LevelDebug
	case INFO:
		return slog.LevelInfo
	default:
		return slog.LevelError
	}
}

type Logger interface {
	Debug(string)
	Info(string)
	Error(string)
}

type slogLogger struct {
	logger *slog.Logger
}

func NewBasicLogger(logLevel LogLevel) Logger {
	return newLogger(logLevel, logOut)
}

func newLogger(logLevel LogLevel, out io.Writer) Logger {
	return slogLogger{logger: slog.New(newPrefixHandler(logLevel.slogLevel(), out))}
}

func (l slogLogger) Debug(message string) {
	l.logger.Debug(message)
}

func (l slogLogger) Info(message string) {
	l.logger.Info(message)
}

func (l slogLogger) Error(message string) {
	if message == "" {
		return
	}
	l.logger.Error(message)
}

// prefixHandler는 레코드의 메시지 한 줄을 레벨별 접두어와 색을 붙여 Write 한 번으로 쓴다.
// 시간과 attribute는 그리지 않는다. Logger 인터페이스가 메시지 문자열만 받으므로 attribute가 들어올 경로가 없다.
type prefixHandler struct {
	minLevel slog.Level
	errorOut *Formatter
	infoOut  *Formatter
	debugOut *Formatter
}

var _ slog.Handler = prefixHandler{}

func newPrefixHandler(minLevel slog.Level, out io.Writer) prefixHandler {
	return prefixHandler{
		minLevel: minLevel,
		errorOut: NewFormatter("[ERROR]", ErrorLoggerColorFunc, out),
		infoOut:  NewFormatter("[INFO]", InfoLoggerColorFunc, out),
		debugOut: NewFormatter("[DEBUG]", DebugLoggerColorFunc, out),
	}
}

func (h prefixHandler) Enabled(_ context.Context, level slog.Level) bool {
	return level >= h.minLevel
}

func (h prefixHandler) Handle(_ context.Context, r slog.Record) error {
	_, err := h.formatterFor(r.Level).Write([]byte(r.Message + "\n"))
	return err
}

func (h prefixHandler) formatterFor(level slog.Level) *Formatter {
	switch {
	case level >= slog.LevelError:
		return h.errorOut
	case level >= slog.LevelInfo:
		return h.infoOut
	default:
		return h.debugOut
	}
}

func (h prefixHandler) WithAttrs([]slog.Attr) slog.Handler {
	return h
}

func (h prefixHandler) WithGroup(string) slog.Handler {
	return h
}
