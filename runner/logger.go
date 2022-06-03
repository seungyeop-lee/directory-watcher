package runner

type logger interface {
	Debug(string)
	Info(string)
	Error(string)
}
