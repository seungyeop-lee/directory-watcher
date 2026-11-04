package command

import (
	"context"
	"fmt"

	"github.com/seungyeop-lee/directory-watcher/v3/internal/app/domain"
)

type StructuredCmd struct {
	Cmd ExecCmdBuilder
	Dir domain.Path
}

var _ ExecCmdBuilder = (*StructuredCmd)(nil)

func (s StructuredCmd) Build(ctx context.Context, runDir domain.Path, event *domain.Event) ([]Command, error) {
	return s.Cmd.Build(ctx, s.Dir, event)
}

// String은 명령을 괄호로 묶어 dir이 어느 명령까지 적용되는지 보여 준다.
func (s StructuredCmd) String() string {
	cmd := s.Cmd.String()
	if cmd == "" || s.Dir == "" {
		return cmd
	}
	return fmt.Sprintf("(in %s: %s)", s.Dir, cmd)
}
