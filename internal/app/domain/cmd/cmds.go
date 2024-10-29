package cmd

import (
	"os/exec"

	"github.com/seungyeop-lee/directory-watcher/v2/internal/app/domain"
	"github.com/seungyeop-lee/directory-watcher/v2/internal/helper"
)

type Cmds []ExecCmdBuilder

var _ ExecCmdBuilder = (*Cmds)(nil)

func (c Cmds) Build(runDir domain.Path, event *domain.Event) ([]*exec.Cmd, error) {
	var result []*exec.Cmd
	for _, cmd := range c {
		if cmds, err := helper.FilterError(cmd.Build(runDir, event)); err != nil {
			return nil, err
		} else {
			result = append(result, cmds...)
		}
	}
	return result, nil
}
