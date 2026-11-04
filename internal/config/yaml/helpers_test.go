package yaml_test

import (
	"testing"

	"github.com/seungyeop-lee/directory-watcher/v3/internal/app/domain"
	configyaml "github.com/seungyeop-lee/directory-watcher/v3/internal/config/yaml"
)

func buildCommandSet(t *testing.T, data string) domain.CommandSet {
	t.Helper()
	commands, err := configyaml.NewConfig([]byte(data)).BuildCommandSet()
	if err != nil {
		t.Fatal(err)
	}
	return commands
}
