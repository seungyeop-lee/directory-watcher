package cmd

import (
	"github.com/seungyeop-lee/directory-watcher/v2/internal/app/domain"
	"github.com/seungyeop-lee/directory-watcher/v2/internal/helper"
)

type MultiLineCmd struct {
	Cmds []SingleCmd
}

var _ domain.Cmd = (*MultiLineCmd)(nil)

func (m MultiLineCmd) Run(runDir domain.Path, event *domain.Event) error {
	for _, cmd := range m.Cmds {
		if err := helper.FilterError(cmd.Run(runDir, event)); err != nil {
			return err
		}
	}
	return nil
}
