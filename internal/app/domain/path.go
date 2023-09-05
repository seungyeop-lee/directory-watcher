package domain

import (
	"log"
	"os"
	"path/filepath"
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

	return p.isSubDirLogical(input)
}

func (p Path) isSubDirLogical(input Path) bool {
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

func (p Path) Abs() string {
	abs, err := filepath.Abs(string(p))
	if err != nil {
		log.Fatal(err)
	}
	return abs
}
