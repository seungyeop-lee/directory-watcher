package cmd

import (
	"context"

	"github.com/seungyeop-lee/directory-watcher/v2/internal/app/domain"
)

type EmptyCmd struct{}

var _ ExecCmdBuilder = (*EmptyCmd)(nil)

func (e EmptyCmd) Build(_ context.Context, _ domain.Path, _ *domain.Event) ([]Command, error) {
	return []Command{}, nil
}
