package yaml

import (
	"github.com/seungyeop-lee/directory-watcher/internal/app/domain"
)

type PathSuffixesConverter struct {
	pathSuffixes PathSuffixes
}

func NewPathSuffixesConverter(pathSuffixes PathSuffixes) *PathSuffixesConverter {
	return &PathSuffixesConverter{pathSuffixes: pathSuffixes}
}

func (c PathSuffixesConverter) Convert() domain.PathSuffixes {
	result := domain.PathSuffixes{}
	for _, e := range c.pathSuffixes {
		result = append(result, domain.PathSuffix(e))
	}
	return result
}
