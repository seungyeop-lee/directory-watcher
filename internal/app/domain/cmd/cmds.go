package cmd

import (
	"github.com/seungyeop-lee/directory-watcher/v2/internal/app/domain"
	"github.com/seungyeop-lee/directory-watcher/v2/internal/helper"
)

type Cmds []domain.Cmd

var _ domain.Cmd = (*Cmds)(nil)

func (c Cmds) Run(runDir domain.Path, event *domain.Event) error {
	for _, cmd := range c {
		if err := helper.FilterError(cmd.Run(runDir, event)); err != nil {
			return err
		}
	}
	return nil
}
