package config

import "github.com/seungyeop-lee/directory-watcher/v3/internal/app/domain"

type Config interface {
	BuildCommandSet() (domain.CommandSet, error)
}
