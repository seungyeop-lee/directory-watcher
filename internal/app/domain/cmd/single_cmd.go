package cmd

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"text/template"

	"github.com/moby/term"

	"github.com/seungyeop-lee/directory-watcher/v2/internal/app/domain"
	"github.com/seungyeop-lee/directory-watcher/v2/internal/helper"
)

type SingleCmd string

var _ ExecCmdBuilder = (*SingleCmd)(nil)

func (c SingleCmd) Build(ctx context.Context, runDir domain.Path, event *domain.Event) ([]*exec.Cmd, error) {
	if c == "" {
		return nil, helper.NewEmptyCmdError()
	}

	argsStr, err := c.buildCmdStringWithEventInfo(event)
	if err != nil {
		return nil, err
	}

	args := strings.Split(argsStr, " ")
	cmd := exec.CommandContext(ctx, args[0], args[1:]...)

	cmd.Dir = runDir.String()

	in, out := targetInOut(ctx)

	cmd.Stdin = in

	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	go printLog(ctx, stdoutPipe, out)

	stderrPipe, err := cmd.StderrPipe()
	if err != nil {
		return nil, err
	}
	go printLog(ctx, stderrPipe, out)

	helper.SetupForOs(cmd)

	return []*exec.Cmd{cmd}, nil
}

func (c SingleCmd) buildCmdStringWithEventInfo(event *domain.Event) (string, error) {
	if event == nil {
		return string(c), nil
	}

	t, err := template.New("parser").Parse(string(c))
	if err != nil {
		return "", fmt.Errorf("can't parse command: %s", err)
	}

	b := strings.Builder{}
	err = t.Execute(&b, map[string]string{
		"Path":       event.Path.String(),
		"AbsPath":    event.Path.Abs(),
		"FileName":   event.Path.FileName(),
		"ExtName":    event.Path.ExtName(),
		"DirPath":    event.Path.DirPath(),
		"DirAbsPath": event.Path.DirAbsPath(),
		"Event":      event.Operation.String(),
	})
	if err != nil {
		return "", fmt.Errorf("can't execute command: %s", err)
	}

	return b.String(), err
}

func targetInOut(ctx context.Context) (io.ReadCloser, io.Writer) {
	termIn, termOut, _ := term.StdStreams()
	out, ok := ctx.Value(helper.FormatterKey).(io.Writer)
	if !ok {
		out = termOut
	}
	return termIn, out
}

func printLog(ctx context.Context, rc io.ReadCloser, out io.Writer) {
	scanner := bufio.NewScanner(rc)
	for {
		select {
		case <-ctx.Done():
			return
		default:
			if !scanner.Scan() {
				return
			}
			line := scanner.Text() + "\n"
			_, _ = fmt.Fprint(out, line)
		}
	}
}
