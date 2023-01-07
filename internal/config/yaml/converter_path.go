package yaml

import (
	"github.com/seungyeop-lee/directory-watcher/v2/internal/app/domain"
)

type PathConverter struct {
	path Path
}

func NewPathConverter(path Path) *PathConverter {
	return &PathConverter{path: path}
}

func (c PathConverter) Convert() domain.Path {
	return domain.Path(c.path)
}

type PathsConverter struct {
	paths Paths
}

func NewPathsConverter(paths Paths) *PathsConverter {
	return &PathsConverter{paths: paths}
}

func (c PathsConverter) Convert() domain.Paths {
	result := domain.Paths{}
	for _, p := range c.paths {
		result = append(result, domain.Path(p))
	}
	return result
}
