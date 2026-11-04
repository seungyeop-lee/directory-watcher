package cmd_test

import (
	"strings"
	"testing"

	"github.com/fsnotify/fsnotify"

	"github.com/seungyeop-lee/directory-watcher/v3/internal/app/domain"
	command "github.com/seungyeop-lee/directory-watcher/v3/internal/app/domain/cmd"
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

// 설정을 읽을 때 CheckTemplates는 템플릿 변수의 이름만 검사하므로, {{.Path.Missing}}처럼 문자열 값의 필드를
// 읽는 명령은 검사를 통과하고 실행할 때 템플릿을 채우지 못한다.
func TestCmdsFailsWhenCommandCannotBeBuilt(t *testing.T) {
	unbuildable := command.SingleCmd{Shell: command.DefaultShell(), Cmd: "echo {{.Path.Missing}}"}
	if err := unbuildable.CheckTemplates(); err != nil {
		t.Fatalf("command is rejected before watching starts: %v", err)
	}
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
