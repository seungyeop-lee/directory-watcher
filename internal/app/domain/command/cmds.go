package command

import (
	"context"

	"github.com/seungyeop-lee/directory-watcher/v3/internal/app/domain"
)

type Cmds []ExecCmdBuilder

var _ ExecCmdBuilder = (*Cmds)(nil)

func (c Cmds) Build(ctx context.Context, runDir domain.Path, event *domain.Event) ([]Command, error) {
	return buildEach(ctx, runDir, event, c)
}

// buildEach는 builders를 차례로 빌드한 명령을 이어 붙인다. 빈 명령은 건너뛰고, 다른 오류가 나면 명령을 하나도 돌려주지 않는다.
func buildEach[B ExecCmdBuilder](ctx context.Context, runDir domain.Path, event *domain.Event, builders []B) ([]Command, error) {
	var result []Command
	for _, builder := range builders {
		cmds, err := filterError(builder.Build(ctx, runDir, event))
		if err != nil {
			return nil, err
		}
		result = append(result, cmds...)
	}
	return result, nil
}
