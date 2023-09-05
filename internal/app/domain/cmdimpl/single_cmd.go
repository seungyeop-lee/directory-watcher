package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/seungyeop-lee/directory-watcher/v2/internal/app/domain"
	"github.com/seungyeop-lee/directory-watcher/v2/internal/helper"
)

type SingleCmd string

var _ domain.Cmd = (*SingleCmd)(nil)

func (c SingleCmd) Run(runDir domain.Path) error {
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
