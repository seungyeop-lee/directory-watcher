package converter

import (
	"github.com/seungyeop-lee/directory-watcher/v2/internal/app/domain"
	"github.com/seungyeop-lee/directory-watcher/v2/internal/config"
)

type PathConverter struct {
	path config.Path
}

func NewPathConverter(path config.Path) *PathConverter {
	return &PathConverter{path: path}
}

func (c PathConverter) Convert() domain.Path {
	return domain.Path(c.path)
}

type PathsConverter struct {
	paths config.Paths
}

func NewPathsConverter(paths config.Paths) *PathsConverter {
	return &PathsConverter{paths: paths}
}

func (c PathsConverter) Convert() domain.Paths {
	result := domain.Paths{}
	for _, p := range c.paths {
		result = append(result, domain.Path(p))
	}
	return result
}
