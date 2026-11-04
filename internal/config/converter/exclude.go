package converter

import (
	"github.com/seungyeop-lee/directory-watcher/v3/internal/app/domain"
	"github.com/seungyeop-lee/directory-watcher/v3/internal/config"
)

type ExcludeConverter struct {
	exclude config.Exclude
}

func NewExcludeConverter(exclude config.Exclude) *ExcludeConverter {
	return &ExcludeConverter{exclude: exclude}
}

func (c ExcludeConverter) Convert() (domain.ExcludePatterns, error) {
	return domain.ParseExcludePatterns(c.exclude)
}
