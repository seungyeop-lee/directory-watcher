package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"text/template"

	"github.com/seungyeop-lee/directory-watcher/v2/internal/app/domain"
	"github.com/seungyeop-lee/directory-watcher/v2/internal/helper"
)

type SingleCmd string

var _ domain.Cmd = (*SingleCmd)(nil)

func (c SingleCmd) Run(runDir domain.Path, event *domain.Event) error {
	if c == "" {
		return helper.NewEmptyCmdError()
	}

	argsStr, err := c.buildCmdStringWithEventInfo(event)
	if err != nil {
		return err
	}

	args := strings.Split(argsStr, " ")
	cmd := exec.Command(args[0], args[1:]...)

	cmd.Dir = runDir.String()

	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("can't start command: %s", err)
	}
	err = cmd.Wait()

	if err != nil {
		return fmt.Errorf("command fails to run or doesn't complete successfully: %v", err)
	}

	return nil
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
