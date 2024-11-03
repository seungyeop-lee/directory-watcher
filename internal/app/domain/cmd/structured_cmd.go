package cmd

import (
	"context"
	"os/exec"

	"github.com/seungyeop-lee/directory-watcher/v2/internal/app/domain"
)

type StructuredCmd struct {
	Cmd ExecCmdBuilder
	Dir domain.Path
}

var _ ExecCmdBuilder = (*StructuredCmd)(nil)

func (s StructuredCmd) Build(ctx context.Context, runDir domain.Path, event *domain.Event) ([]*exec.Cmd, error) {
	return s.Cmd.Build(ctx, s.Dir, event)
}
