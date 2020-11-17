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
	args := strings.Split(c.String(), " ")
	cmd := exec.Command(args[0], args[1:]...)

	cmd.Dir = runDir.String()

	cmd.Stdout = os.Stdout

	log.Println("start command")
	if err := cmd.Start(); err != nil {
		log.Printf("can't start command: %s", err)
		return
	}
	log.Println("wait command")
	err := cmd.Wait()
	log.Println("finish command")

	if err != nil {
		log.Println("command fails to run or doesn't complete successfully")
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

type CommandSet struct {
	InitCmd    Cmd
	Cmd        Cmd
	Path       Path
	ExcludeDir Paths
}

type CommandSets []CommandSet
