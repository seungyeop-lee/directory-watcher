package config

import "github.com/seungyeop-lee/directory-watcher/v2/internal/app/domain"

type Config interface {
	BuildCommandSet() domain.CommandSet
}
