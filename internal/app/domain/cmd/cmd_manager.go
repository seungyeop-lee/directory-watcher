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
		runErr := make(chan error, 1)
		go func(c *exec.Cmd) {
			runErr <- c.Run()
		}(cmd)

		select {
		case e := <-runErr:
			helper.GlobalLogger.Debug("Run command: " + cmd.String())
			if e != nil {
				return e
			}
		case <-ctx.Context().Done():
			helper.GlobalLogger.Debug("Cancel command: " + cmd.String())
			return ctx.Context().Err()
		}
	}

	return nil
}
