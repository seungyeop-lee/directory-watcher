package helper

import (
	"fmt"
	"io"

	"github.com/moby/term"
)

const LogLevelStringDefaultValue = "ERROR"

var ErrorLoggerOut io.Writer
var InfoLoggerOut io.Writer
var DebugLoggerOut io.Writer

func init() {
	_, out, _ := term.StdStreams()
	ErrorLoggerOut = NewFormatter("[ERROR]", ErrorLoggerColorFunc, out)
	InfoLoggerOut = NewFormatter("[INFO]", InfoLoggerColorFunc, out)
	DebugLoggerOut = NewFormatter("[DEBUG]", DebugLoggerColorFunc, out)
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

type Logger interface {
	Debug(string)
	Info(string)
	Error(string)
}

type basicLogger struct {
	logLevel LogLevel
}

func NewBasicLogger(logLevel LogLevel) Logger {
	return &basicLogger{
		logLevel: logLevel,
	}
}

func (l basicLogger) Debug(message string) {
	if l.logLevel == DEBUG {
		_, _ = fmt.Fprintln(DebugLoggerOut, message)
	}
}

func (l basicLogger) Info(message string) {
	if l.logLevel == DEBUG || l.logLevel == INFO {
		_, _ = fmt.Fprintln(InfoLoggerOut, message)
	}
}

func (l basicLogger) Error(message string) {
	if message == "" {
		return
	}

	if l.logLevel == DEBUG || l.logLevel == INFO || l.logLevel == ERROR {
		_, _ = fmt.Fprintln(ErrorLoggerOut, message)
	}
}
