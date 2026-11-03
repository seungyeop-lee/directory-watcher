package cmd

import (
	"context"

	"github.com/seungyeop-lee/directory-watcher/v2/internal/app/domain"
	"github.com/seungyeop-lee/directory-watcher/v2/internal/helper"
)

type MultiLineCmd struct {
	Cmds []SingleCmd
}

var _ ExecCmdBuilder = (*MultiLineCmd)(nil)

func (m MultiLineCmd) Build(ctx context.Context, runDir domain.Path, event *domain.Event) ([]Command, error) {
	var result []Command
	for _, cmd := range m.Cmds {
		if cmds, err := helper.FilterError(cmd.Build(ctx, runDir, event)); err != nil {
			return nil, err
		} else {
			result = append(result, cmds...)
		}
	}
	return result, nil
}
