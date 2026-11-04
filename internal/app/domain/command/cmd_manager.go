package command

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/moby/term"

	"github.com/seungyeop-lee/directory-watcher/v3/internal/app/domain"
	"github.com/seungyeop-lee/directory-watcher/v3/internal/helper"
	"github.com/seungyeop-lee/directory-watcher/v3/internal/process"
)

type ExecCmdBuilder interface {
	Build(ctx context.Context, runDir domain.Path, event *domain.Event) ([]Command, error)
	// String은 설정 파일에 적힌 명령을 로그에 보여 줄 문자열로 돌려준다. 빌드할 명령이 없으면 빈 문자열이다.
	fmt.Stringer
}

// Command는 실행할 프로세스와, 실패 로그와 실행 전 출력에 보여 줄 명령 문자열이다.
type Command struct {
	*exec.Cmd
	// Filled는 템플릿 변수 자리에 값을 그대로 넣은 명령이다. 실행하지 않고 실패 로그와 실행 전 출력에만 쓴다.
	// 템플릿 변수가 없는 명령이면 비어 있다.
	Filled string
	// written은 설정에 적힌 명령이다.
	written string
	// dir은 설정의 dir로 지정한 작업 폴더다. dir을 적지 않은 명령이면 비어 있다.
	dir string
}

func (c Command) String() string {
	if c.Filled == "" {
		return c.Cmd.String()
	}
	return fmt.Sprintf("%s (with values: %s)", c.Cmd, c.Filled)
}

// announcement는 명령을 실행하기 직전에 hook 이름과 함께 출력하는 줄이다. 셸에 넘기는 명령은 템플릿 변수 자리가
// 환경 변수 참조라서 읽기 어려우므로, 값을 채운 명령을 보여 준다.
func (c Command) announcement(hook string) string {
	line := c.Filled
	if line == "" {
		line = c.written
	}
	if c.dir != "" {
		line += " (dir: " + c.dir + ")"
	}
	return hook + ": " + line + "\n"
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

// String은 hook이 실행할 명령을 돌려준다. 명령이 없는 hook이면 (none)이다.
func (c *Manager) String() string {
	if s := c.builder.String(); s != "" {
		return s
	}
	return "(none)"
}

func (c *Manager) Run(ctx *process.WatcherContext, runDir domain.Path, event *domain.Event) error {
	cmds, err := c.builder.Build(ctx.Context(), runDir, event)
	if err != nil {
		return err
	}
	out := hookOutput(ctx.Context())
	mode := ctx.Mode()
	for _, cmd := range cmds {
		ctx.ApplyGracefulShutdown(cmd.Cmd)
		if !mode.Silent {
			_, _ = fmt.Fprint(out, cmd.announcement(ctx.Hook()))
		}
		err := checkWorkingDir(cmd.Dir)
		if err == nil && !mode.DryRun {
			err = runPrintingOutput(ctx, cmd.Cmd, out)
		}
		if err != nil {
			return fmt.Errorf("command %s: %w", cmd, err)
		}
	}

	return nil
}

// checkWorkingDir는 명령을 실행하기 전에 작업 폴더가 있는지 확인한다. hook 명령은 process.SetupForOs가
// SysProcAttr를 설정하므로 os.StartProcess가 작업 폴더를 미리 확인하지 않고, 폴더가 없으면
// "fork/exec <실행 파일>: no such file or directory"처럼 실행 파일이 없다고 읽히는 오류를 낸다.
// 앞 명령이 만든 폴더에서 다음 명령을 실행할 수 있도록, 명령마다 실행 직전에 확인한다.
func checkWorkingDir(dir string) error {
	if dir == "" {
		return nil
	}
	info, err := os.Stat(dir)
	if err != nil {
		return fmt.Errorf("working directory %s: %w", dir, err.(*fs.PathError).Err)
	}
	if !info.IsDir() {
		return fmt.Errorf("working directory %s: not a directory", dir)
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

// leftoverOutputWait는 서버를 백그라운드로 띄우는 hook이 변경마다 다음 hook을 오래 늦추지 않게 짧게 둔다.
const leftoverOutputWait = time.Second

// runPrintingOutput는 명령을 실행하고, 명령이 낸 출력을 out에 모두 쓴 뒤에 돌아온다.
// 명령이 취소되어도 파이프가 닫힐 때까지 출력을 계속 읽는다. 명령이 남긴 프로세스가
// 파이프를 계속 열어 두면, 명령이 끝난 뒤 leftoverOutputWait만큼만 기다리고 파이프를 닫는다.
// 그 뒤에 남은 프로세스가 쓰는 출력은 버린다.
func runPrintingOutput(ctx *process.WatcherContext, cmd *exec.Cmd, out io.Writer) error {
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
	case <-time.After(leftoverOutputWait):
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
