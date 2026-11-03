package domain

import (
	"log"
	"os"
	"path/filepath"
	"strings"
)

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

	return p.Contains(input)
}

// Contains는 input이 p와 같거나 p 아래에 있는지를 디스크를 보지 않고 경로만으로 판단한다.
// 상대 경로와 빈 경로는 작업 디렉터리 기준으로 푼다.
func (p Path) Contains(input Path) bool {
	_, ok := p.relOf(input)
	return ok
}

// Rebase는 from과 같거나 from 아래에 있는 경로를 to 아래의 같은 위치로 바꾸고, 그 밖의 경로는 그대로 돌려준다.
func (p Path) Rebase(from, to Path) Path {
	rel, ok := from.relOf(p)
	if !ok {
		return p
	}
	return Path(filepath.Join(string(to), rel))
}

// relOf는 input이 p와 같거나 p 아래에 있으면 p 기준 상대 경로를 돌려준다.
func (p Path) relOf(input Path) (string, bool) {
	pAbs, _ := filepath.Abs(string(p))
	inputAbs, _ := filepath.Abs(string(input))

	rel, err := filepath.Rel(pAbs, inputAbs)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", false
	}
	return rel, true
}

func (p Path) IsDir() bool {
	stat, err := os.Stat(string(p))
	if err != nil {
		return false
	}
	return stat.IsDir()
}

func (p Path) Abs() string {
	abs, err := filepath.Abs(string(p))
	if err != nil {
		log.Fatal(err)
	}
	return abs
}

func (p Path) FileName() string {
	return filepath.Base(string(p))
}

func (p Path) IsDefaultExcludeFile() bool {
	name := p.FileName()
	return strings.HasPrefix(name, ".") || strings.HasPrefix(name, "_")
}

func (p Path) ExtName() string {
	return filepath.Ext(string(p))
}

func (p Path) DirPath() string {
	return filepath.Dir(string(p))
}

func (p Path) DirAbsPath() string {
	return filepath.Dir(p.Abs())
}

func (p Path) Alias() string {
	return filepath.Base(p.String())
}
