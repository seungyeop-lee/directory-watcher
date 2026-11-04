package command_test

import (
	"strings"
	"testing"

	"github.com/fsnotify/fsnotify"

	"github.com/seungyeop-lee/directory-watcher/v3/internal/app/domain"
	"github.com/seungyeop-lee/directory-watcher/v3/internal/app/domain/command"
)

func TestCmdsSkipsEmptyCommand(t *testing.T) {
	event := &domain.Event{Path: "b.txt", Operation: domain.Operation(fsnotify.Create)}
	ctx, output := capturedContext(t)
	builder := command.Cmds{
		command.SingleCmd{Shell: command.DefaultShell(), Cmd: ""},
		command.SingleCmd{Shell: command.DefaultShell(), Cmd: "echo after"},
	}
	if err := command.NewManager(builder).Run(ctx, domain.Path(t.TempDir()), event); err != nil {
		t.Fatal(err)
	}
	if got, want := hookLines(output()), []string{"after"}; !equalLines(got, want) {
		t.Fatalf("hook printed %q, want %q", got, want)
	}
}

// 설정을 읽을 때 CheckTemplates는 {{.Path.Missing}}을 거부하므로, 이 테스트는 검사를 거치지 않은 명령으로 Build가 실패하는 경우를 만든다.
func TestCmdsFailsWhenCommandCannotBeBuilt(t *testing.T) {
	unbuildable := command.SingleCmd{Shell: command.DefaultShell(), Cmd: "echo {{.Path.Missing}}"}
	event := &domain.Event{Path: "b.txt", Operation: domain.Operation(fsnotify.Create)}
	ctx, output := capturedContext(t)
	builder := command.Cmds{unbuildable, command.SingleCmd{Shell: command.DefaultShell(), Cmd: "echo after"}}
	err := command.NewManager(builder).Run(ctx, domain.Path(t.TempDir()), event)
	if err == nil || !strings.Contains(err.Error(), "can't execute command") {
		t.Fatalf("hook with a command that cannot be built returned %v", err)
	}
	if got := hookLines(output()); len(got) != 0 {
		t.Fatalf("commands ran although one could not be built: %q", got)
	}
}
