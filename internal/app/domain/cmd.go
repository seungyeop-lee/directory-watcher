package domain

type Cmd interface {
	Run(runDir Path, event *Event) error
}
