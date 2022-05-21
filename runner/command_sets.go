package runner

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
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

func (p Path) IsSubFolder(input Path) bool {
	pAbs, _ := filepath.Abs(string(p))
	inputAbs, _ := filepath.Abs(string(input))

	for i := 0; i < len(pAbs); i++ {
		// 비교대상의 절대 경로가 기준의 절대 경로보다 짦을 경우 서브폴더가 아니다.
		if len(inputAbs) < i+1 {
			return false
		}

		if pAbs[i] != inputAbs[i] {
			return false
		}
	}
	return true
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

func (p Paths) IsSubFolder(input Path) bool {
	for _, path := range p {
		if path.IsSubFolder(input) {
			return true
		}
	}
	return false
}

type Millisecond uint

func (m Millisecond) Duration() time.Duration {
	return time.Duration(m) * time.Millisecond
}

type CommandSets struct {
	InitCmd   Cmd          `yaml:"initCmd"`
	EndCmd    Cmd          `yaml:"endCmd"`
	BeforeCmd Cmd          `yaml:"beforeCmd"`
	AfterCmd  Cmd          `yaml:"afterCmd"`
	Sets      []CommandSet `yaml:"sets"`
}

type CommandSet struct {
	InitCmd         Cmd         `yaml:"initCmd"`
	EndCmd          Cmd         `yaml:"endCmd"`
	GlobalBeforeCmd Cmd         `yaml:"beforeCmd"`
	GlobalAfterCmd  Cmd         `yaml:"afterCmd"`
	Cmd             Cmd         `yaml:"cmd"`
	Path            Path        `yaml:"path"`
	ExcludeDir      Paths       `yaml:"excludeDir"`
	WaitMillisecond Millisecond `yaml:"waitMillisecond"`
}
