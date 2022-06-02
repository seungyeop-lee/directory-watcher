package runner

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/seungyeop-lee/directory-watcher/helper"
)

type Cmd interface {
	Run(runDir Path) error
}

type Cmds []Cmd

func (c Cmds) Run(runDir Path) error {
	for _, cmd := range c {
		err := cmd.Run(runDir)
		if err != nil {
			switch err {
			case helper.EmptyCmdError:
				break
			default:
				return err
			}
		}
	}
	return nil
}

type EmptyCmd struct{}

func (e EmptyCmd) Run(_ Path) error {
	return nil
}

type SingleCmd string

func (c SingleCmd) Run(runDir Path) error {
	if c == "" {
		return helper.EmptyCmdError
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

func (m MultiLineCmd) Run(runDir Path) error {
	for _, cmd := range m.Cmds {
		err := cmd.Run(runDir)
		if err != nil {
			switch err {
			case helper.EmptyCmdError:
				break
			default:
				return err
			}
		}
	}
	return nil
}

type StructuredCmd struct {
	Cmd Cmd
	Dir Path
}

func (s StructuredCmd) Run(runDir Path) error {
	if s.Dir == "" {
		s.Dir = runDir
	}

	return s.Cmd.Run(s.Dir)
}
