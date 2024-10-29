package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"text/template"

	"github.com/seungyeop-lee/directory-watcher/v2/internal/app/domain"
	"github.com/seungyeop-lee/directory-watcher/v2/internal/helper"
)

type SingleCmd string

var _ ExecCmdBuilder = (*SingleCmd)(nil)

func (c SingleCmd) Build(runDir domain.Path, event *domain.Event) ([]*exec.Cmd, error) {
	if c == "" {
		return nil, helper.NewEmptyCmdError()
	}

	argsStr, err := c.buildCmdStringWithEventInfo(event)
	if err != nil {
		return nil, err
	}

	args := strings.Split(argsStr, " ")
	cmd := exec.Command(args[0], args[1:]...)

	cmd.Dir = runDir.String()

	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	//Setpgid: true로 설정하면 프로세스 그룹이 만들어지고, 자식 프로세스들도 이 그룹에 포함 됨
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Setpgid: true,
	}

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
