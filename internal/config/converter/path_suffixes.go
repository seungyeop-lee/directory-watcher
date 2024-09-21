package converter

import (
	"github.com/seungyeop-lee/directory-watcher/v2/internal/app/domain"
	"github.com/seungyeop-lee/directory-watcher/v2/internal/config"
)

type PathSuffixesConverter struct {
	pathSuffixes config.PathSuffixes
}

func NewPathSuffixesConverter(pathSuffixes config.PathSuffixes) *PathSuffixesConverter {
	return &PathSuffixesConverter{pathSuffixes: pathSuffixes}
}

func (c PathSuffixesConverter) Convert() domain.PathSuffixes {
	result := domain.PathSuffixes{}
	for _, e := range c.pathSuffixes {
		result = append(result, domain.PathSuffix(e))
	}
	return result
}
