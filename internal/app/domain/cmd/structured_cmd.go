package cmd

import (
	"github.com/seungyeop-lee/directory-watcher/v2/internal/app/domain"
)

type StructuredCmd struct {
	Cmd domain.Cmd
	Dir domain.Path
}

var _ domain.Cmd = (*StructuredCmd)(nil)

func (s StructuredCmd) Run(_ domain.Path, event *domain.Event) error {
	return s.Cmd.Run(s.Dir, event)
}
