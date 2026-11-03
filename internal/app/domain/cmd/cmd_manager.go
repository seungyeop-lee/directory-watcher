package cmd

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/moby/term"

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

	out := hookOutput(ctx.Context())
	for _, cmd := range cmds {
		if err := runPrintingOutput(ctx, cmd, out); err != nil {
			return fmt.Errorf("command %s: %w", cmd, err)
		}
	}

	return nil
}

func hookOutput(ctx context.Context) io.Writer {
	if out, ok := ctx.Value(helper.FormatterKey).(io.Writer); ok {
		return out
	}
	_, out, _ := term.StdStreams()
	return out
}

// runPrintingOutput는 명령을 실행하고, 명령이 낸 출력을 out에 모두 쓴 뒤에 돌아온다.
// 명령이 취소되어도 파이프가 닫힐 때까지 출력을 계속 읽는다. 명령이 남긴 프로세스가
// 파이프를 계속 열어 두면, 명령이 끝난 뒤 cmd.WaitDelay만큼만 기다리고 파이프를 닫는다.
func runPrintingOutput(ctx *helper.WatcherContext, cmd *exec.Cmd, out io.Writer) error {
	r, w, err := os.Pipe()
	if err != nil {
		return err
	}
	// 파이프 파일을 직접 넘겨야 os/exec가 출력을 대신 읽지 않는다. os/exec가 읽으면 Wait가
	// 남은 자식 프로세스가 파이프를 닫을 때까지 최대 WaitDelay만큼 기다리므로, RunCommand가
	// Wait 뒤에 하는 프로세스 트리 정리가 그만큼 늦어진다.
	cmd.Stdout, cmd.Stderr = w, w
	printed := make(chan struct{})
	go func() {
		defer close(printed)
		printLines(r, out)
	}()

	err = ctx.RunCommand(cmd)
	_ = w.Close()
	select {
	case <-printed:
	case <-time.After(cmd.WaitDelay):
	}
	_ = r.Close()
	<-printed
	return err
}

func printLines(r io.Reader, out io.Writer) {
	reader := bufio.NewReader(r)
	for {
		line, err := reader.ReadString('\n')
		if line != "" {
			_, _ = fmt.Fprint(out, strings.TrimSuffix(strings.TrimSuffix(line, "\n"), "\r")+"\n")
		}
		if err != nil {
			return
		}
	}
}
