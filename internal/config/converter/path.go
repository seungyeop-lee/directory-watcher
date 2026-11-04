package converter

import (
	"github.com/seungyeop-lee/directory-watcher/v3/internal/app/domain"
	"github.com/seungyeop-lee/directory-watcher/v3/internal/config"
)

type PathConverter struct {
	path config.Path
}

func NewPathConverter(path config.Path) *PathConverter {
	return &PathConverter{path: path}
}

func (c PathConverter) Convert() (domain.Path, error) {
	return domain.Path(c.path), nil
}
