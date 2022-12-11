package domain

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/seungyeop-lee/directory-watcher/internal/helper"
)

type Cmd interface {
	Run(runDir Path) error
}

type Cmds []Cmd

func (c Cmds) Run(runDir Path) error {
	for _, cmd := range c {
		if err := filterError(cmd.Run(runDir)); err != nil {
			return err
		}
	}
	return nil
}

type EmptyCmd struct{}

var _ Cmd = EmptyCmd{}

func (e EmptyCmd) Run(_ Path) error {
	return nil
}

type SingleCmd string

var _ Cmd = SingleCmd("")

func (c SingleCmd) Run(runDir Path) error {
	if c == "" {
		return helper.NewEmptyCmdError()
	}

	args := strings.Split(string(c), " ")
	cmd := exec.Command(args[0], args[1:]...)

	cmd.Dir = runDir.String()

	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("can't start command: %s", err)
	}
	err := cmd.Wait()

	if err != nil {
		return fmt.Errorf("command fails to run or doesn't complete successfully: %v", err)
	}

	return nil
}

type MultiLineCmd struct {
	Cmds []SingleCmd
}

var _ Cmd = MultiLineCmd{}

func (m MultiLineCmd) Run(runDir Path) error {
	for _, cmd := range m.Cmds {
		if err := filterError(cmd.Run(runDir)); err != nil {
			return err
		}
	}
	return nil
}

type StructuredCmd struct {
	Cmd Cmd
	Dir Path
}

var _ Cmd = StructuredCmd{}

func (s StructuredCmd) Run(_ Path) error {
	return s.Cmd.Run(s.Dir)
}

func filterError(err error) error {
	if err == nil {
		return nil
	}

	switch err.(type) {
	case helper.EmptyCmdError:
		return nil
	default:
		return err
	}
}
