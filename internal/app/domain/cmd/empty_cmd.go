package cmd

import (
	"context"
	"os/exec"

	"github.com/seungyeop-lee/directory-watcher/v2/internal/app/domain"
)

type EmptyCmd struct{}

var _ ExecCmdBuilder = (*EmptyCmd)(nil)

func (e EmptyCmd) Build(_ context.Context, _ domain.Path, _ *domain.Event) ([]*exec.Cmd, error) {
	return []*exec.Cmd{}, nil
}
