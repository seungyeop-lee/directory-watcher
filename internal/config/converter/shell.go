package converter

import (
	"github.com/seungyeop-lee/directory-watcher/v3/internal/app/domain/cmd"
	"github.com/seungyeop-lee/directory-watcher/v3/internal/config"
)

type ShellConverter struct {
	shell config.Shell
}

func NewShellConverter(shell config.Shell) *ShellConverter {
	return &ShellConverter{shell: shell}
}

func (c ShellConverter) Convert() (cmd.Shell, error) {
	if c.shell == "" {
		return cmd.DefaultShell(), nil
	}
	return cmd.LookupShell(string(c.shell))
}
