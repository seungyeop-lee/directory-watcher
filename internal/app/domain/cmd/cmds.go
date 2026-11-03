package cmd

import (
	"context"

	"github.com/seungyeop-lee/directory-watcher/v3/internal/app/domain"
	"github.com/seungyeop-lee/directory-watcher/v3/internal/helper"
)

type Cmds []ExecCmdBuilder

var _ ExecCmdBuilder = (*Cmds)(nil)

func (c Cmds) Build(ctx context.Context, runDir domain.Path, event *domain.Event) ([]Command, error) {
	var result []Command
	for _, cmd := range c {
		if cmds, err := helper.FilterError(cmd.Build(ctx, runDir, event)); err != nil {
			return nil, err
		} else {
			result = append(result, cmds...)
		}
	}
	return result, nil
}
