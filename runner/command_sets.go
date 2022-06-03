package runner

import (
	"os"
	"path/filepath"
	"time"
)

type Path string

func (p Path) String() string {
	return string(p)
}

func (p Path) Equal(input Path) bool {
	pAbs, _ := filepath.Abs(string(p))
	inputAbs, _ := filepath.Abs(string(input))

	return pAbs == inputAbs
}

func (p Path) IsSubDir(input Path) bool {
	if !p.IsDir() || !input.IsDir() {
		return false
	}

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

func (p Path) IsDir() bool {
	stat, err := os.Stat(string(p))
	if err != nil {
		return false
	}
	return stat.IsDir()
}

func (p Path) Ext() string {
	return filepath.Ext(string(p))
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
		if path.IsSubDir(input) {
			return true
		}
	}
	return false
}

type Ext string

type Exts []Ext

func (e Exts) Equal(input Ext) bool {
	for _, ext := range e {
		if ext == input {
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
	ExcludeExt      Exts        `yaml:"excludeExt"`
	WaitMillisecond Millisecond `yaml:"waitMillisecond"`
}
