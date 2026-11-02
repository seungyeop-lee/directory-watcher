package yaml_test

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/seungyeop-lee/directory-watcher/v2/internal/app/domain"
	configyaml "github.com/seungyeop-lee/directory-watcher/v2/internal/config/yaml"
	"github.com/seungyeop-lee/directory-watcher/v2/internal/helper"
)

func TestDecodedOnErrorRunsStructuredCommand(t *testing.T) {
	workdir := t.TempDir()
	t.Setenv("DW_ERROR_CAUSE", "overflow 'quoted' {{.Path}} $HOME")
	t.Setenv("DW_CONFIG_NOTIFICATION_PROCESS", "1")
	data := fmt.Sprintf(`global:
  lifeCycle:
    onError:
      - dir: %q
        cmd: %q
watchTargets: []
`, workdir, os.Args[0]+" -test.run=^TestConfigNotificationProcess$")
	commands := configyaml.NewConfig([]byte(data)).BuildCommandSet()
	hook := commands.Global.LifeCycle.OnError
	if hook == nil {
		t.Fatal("decoded onError did not build a command")
	}
	ctx := helper.NewWatcherContext("config-test")
	defer ctx.Cancel()
	if err := hook.Run(ctx, domain.Path(t.TempDir()), nil); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(filepath.Join(workdir, "notification.txt"))
	if err != nil {
		t.Fatal(err)
	}
	want := workdir + "\n" + os.Getenv("DW_ERROR_CAUSE")
	if string(got) != want {
		t.Fatalf("notification output = %q, want %q", got, want)
	}
}

func TestDecodedLegacyConfigBuildsWithoutOnError(t *testing.T) {
	commands := configyaml.NewConfig([]byte("global:\n  lifeCycle: {}\nwatchTargets: []\n")).BuildCommandSet()
	ctx := helper.NewWatcherContext("config-test")
	defer ctx.Cancel()
	if err := commands.Global.LifeCycle.OnStartWatch.Run(ctx, domain.Path(t.TempDir()), nil); err != nil {
		t.Fatal(err)
	}
}

func TestDecodedAbsentOnErrorRunsWithoutNotification(t *testing.T) {
	commands := configyaml.NewConfig([]byte("watchTargets: []\n")).BuildCommandSet()
	ctx := helper.NewWatcherContext("config-test")
	defer ctx.Cancel()
	if err := commands.Global.LifeCycle.OnError.Run(ctx, domain.Path(t.TempDir()), nil); err != nil {
		t.Fatal(err)
	}
}

func TestConfigNotificationProcess(t *testing.T) {
	if os.Getenv("DW_CONFIG_NOTIFICATION_PROCESS") != "1" {
		return
	}
	workdir, err := os.Getwd()
	if err != nil {
		os.Exit(2)
	}
	if err := os.WriteFile("notification.txt", []byte(workdir+"\n"+os.Getenv("DW_ERROR_CAUSE")), 0600); err != nil {
		os.Exit(3)
	}
	os.Exit(0)
}
