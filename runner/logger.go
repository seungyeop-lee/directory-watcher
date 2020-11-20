package runner

type logger interface {
	Info(string)
	Error(string)
}
