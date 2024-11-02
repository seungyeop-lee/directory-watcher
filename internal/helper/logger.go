package helper

import (
	"log"
)

const LogLevelStringDefaultValue = "ERROR"

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
		log.Println(message)
	}
}

func (l basicLogger) Info(message string) {
	if l.logLevel == DEBUG || l.logLevel == INFO {
		log.Println(message)
	}
}

func (l basicLogger) Error(message string) {
	if message == "" {
		return
	}

	if l.logLevel == DEBUG || l.logLevel == INFO || l.logLevel == ERROR {
		log.Println(message)
	}
}
