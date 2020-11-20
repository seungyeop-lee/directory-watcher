package helper

import "log"

type basicLogger struct {
	isVerbose bool
}

func NewBasicLogger(isVerbose bool) basicLogger {
	return basicLogger{isVerbose: isVerbose}
}

func (l basicLogger) Info(message string) {
	if l.isVerbose {
		log.Println(message)
	}
}

func (l basicLogger) Error(message string) {
	log.Println(message)
}
