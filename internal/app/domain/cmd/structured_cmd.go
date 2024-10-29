package cmd

import (
	"os/exec"

	"github.com/seungyeop-lee/directory-watcher/v2/internal/app/domain"
)

type StructuredCmd struct {
	Cmd ExecCmdBuilder
	Dir domain.Path
}

var _ ExecCmdBuilder = (*StructuredCmd)(nil)

func (s StructuredCmd) Build(runDir domain.Path, event *domain.Event) ([]*exec.Cmd, error) {
	return s.Cmd.Build(s.Dir, event)
}
