package cmd

import (
	"context"
	"os/exec"

	"github.com/seungyeop-lee/directory-watcher/v2/internal/app/domain"
	"github.com/seungyeop-lee/directory-watcher/v2/internal/helper"
)

type ExecCmdBuilder interface {
	Build(ctx context.Context, runDir domain.Path, event *domain.Event) ([]*exec.Cmd, error)
}

type Manager struct {
	builder ExecCmdBuilder
}

func NewManager(builder ExecCmdBuilder) *Manager {
	return &Manager{
		builder: builder,
	}
}

var _ domain.Cmd = (*Manager)(nil)

func (c *Manager) Run(ctx *helper.WatcherContext, runDir domain.Path, event *domain.Event) error {
	cmds, err := c.builder.Build(ctx.Context(), runDir, event)
	if err != nil {
		return err
	}
	ctx.ApplyGracefulShutdown(cmds)

	for _, cmd := range cmds {
		if err := cmd.Run(); err != nil {
			return err
		}
	}

	return nil
}
