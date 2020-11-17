package runner

import "path/filepath"

type Cmd string

func (c Cmd) String() string {
	return string(c)
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
