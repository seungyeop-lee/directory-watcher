package helper

import (
	"log"
)

const LogLevelStringDefaultValue = "ERROR"

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

type basicLogger struct {
	logLevel LogLevel
}

func NewBasicLogger(logLevel LogLevel) basicLogger {
	return basicLogger{
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
