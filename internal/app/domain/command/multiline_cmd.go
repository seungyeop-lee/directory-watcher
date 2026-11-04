package command

import (
	"context"

	"github.com/seungyeop-lee/directory-watcher/v3/internal/app/domain"
)

type MultiLineCmd struct {
	Cmds []SingleCmd
}

var _ ExecCmdBuilder = (*MultiLineCmd)(nil)

func (m MultiLineCmd) Build(ctx context.Context, runDir domain.Path, event *domain.Event) ([]Command, error) {
	return buildEach(ctx, runDir, event, m.Cmds)
}
