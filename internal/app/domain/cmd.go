package domain

import (
	"github.com/seungyeop-lee/directory-watcher/v2/internal/helper"
)

type Cmd interface {
	Run(runDir Path) error
}

type Cmds []Cmd

func (c Cmds) Run(runDir Path) error {
	for _, cmd := range c {
		if err := helper.FilterError(cmd.Run(runDir)); err != nil {
			return err
		}
	}
	return nil
}
