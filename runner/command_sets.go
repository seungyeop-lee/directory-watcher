package runner

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

type Cmd string

func (c Cmd) String() string {
	return string(c)
}

func (c Cmd) Run(runDir Path) error {
	if c == "" {
		return errors.New("cmd is empty")
	}

	args := strings.Split(c.String(), " ")
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

type Path string

func (p Path) String() string {
	return string(p)
}

func (p Path) Equal(input Path) bool {
	pAbs, _ := filepath.Abs(string(p))
	inputAbs, _ := filepath.Abs(string(input))
	return pAbs == inputAbs
}

type Paths []Path

func (p Paths) Equal(input Path) bool {
	for _, path := range p {
		if path.Equal(input) {
			return true
		}
	}
	return false
}

type CommandSets struct {
	InitCmd   Cmd          `yaml:"initCmd"`
	EndCmd    Cmd          `yaml:"endCmd"`
	BeforeCmd Cmd          `yaml:"beforeCmd"`
	AfterCmd  Cmd          `yaml:"afterCmd"`
	Sets      []CommandSet `yaml:"sets"`
}

type CommandSet struct {
	InitCmd         Cmd   `yaml:"initCmd"`
	EndCmd          Cmd   `yaml:"endCmd"`
	GlobalBeforeCmd Cmd   `yaml:"beforeCmd"`
	GlobalAfterCmd  Cmd   `yaml:"afterCmd"`
	Cmd             Cmd   `yaml:"cmd"`
	Path            Path  `yaml:"path"`
	ExcludeDir      Paths `yaml:"excludeDir"`
}
