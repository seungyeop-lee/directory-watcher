package service

type Logger interface {
	Debug(string)
	Info(string)
	Error(string)
}
