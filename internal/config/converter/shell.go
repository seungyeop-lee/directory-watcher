package converter

import (
	"github.com/seungyeop-lee/directory-watcher/v3/internal/app/domain/command"
	"github.com/seungyeop-lee/directory-watcher/v3/internal/config"
)

type ShellConverter struct {
	shell config.Shell
}

func NewShellConverter(shell config.Shell) *ShellConverter {
	return &ShellConverter{shell: shell}
}

func (c ShellConverter) Convert() (command.Shell, error) {
	if c.shell == "" {
		return command.DefaultShell(), nil
	}
	return command.LookupShell(string(c.shell))
}
