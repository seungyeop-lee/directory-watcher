package domain

type Cmd interface {
	Run(runDir Path) error
}
