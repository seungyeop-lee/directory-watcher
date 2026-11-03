package scenarios

import (
	"os"
	"testing"

	"github.com/seungyeop-lee/directory-watcher/v2/e2e/harness"
)

func TestMain(m *testing.M) { os.Exit(harness.Run(m)) }
