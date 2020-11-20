package runner

import (
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

type Cmd string

func (c Cmd) String() string {
	return string(c)
}

func (c Cmd) Run(runDir Path) {
	if c == "" {
		log.Println("cmd is empty")
		return
	}

	args := strings.Split(c.String(), " ")
	cmd := exec.Command(args[0], args[1:]...)

	cmd.Dir = runDir.String()

	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	log.Println("start:", cmd)
	if err := cmd.Start(); err != nil {
		log.Printf("can't start command: %s", err)
		return
	}
	err := cmd.Wait()
	log.Println("finish:", cmd)

	if err != nil {
		log.Println("command fails to run or doesn't complete successfully:", err)
	}

	return
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
	InitCmd Cmd          `yaml:"initCmd"`
	EndCmd  Cmd          `yaml:"endCmd"`
	Sets    []CommandSet `yaml:"sets"`
}

type CommandSet struct {
	InitCmd    Cmd   `yaml:"initCmd"`
	EndCmd     Cmd   `yaml:"endCmd"`
	Cmd        Cmd   `yaml:"cmd"`
	Path       Path  `yaml:"path"`
	ExcludeDir Paths `yaml:"excludeDir"`
}
