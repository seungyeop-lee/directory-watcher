package cmd

import (
	"github.com/seungyeop-lee/directory-watcher/v2/internal/app/domain"
)

type EmptyCmd struct{}

var _ domain.Cmd = (*EmptyCmd)(nil)

func (e EmptyCmd) Run(_ domain.Path, _ *domain.Event) error {
	return nil
}
