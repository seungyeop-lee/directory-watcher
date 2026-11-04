package command_test

import (
	"strings"
	"testing"

	"github.com/fsnotify/fsnotify"

	"github.com/seungyeop-lee/directory-watcher/v3/internal/app/domain"
	"github.com/seungyeop-lee/directory-watcher/v3/internal/app/domain/command"
)

// 이 명령들은 변수 이름은 모두 eventVariables에 있지만, 어떤 이벤트로 실행해도 템플릿을 채우지 못한다.
func TestCheckTemplatesRejectsCommandThatAlwaysFailsToRun(t *testing.T) {
	commands := map[string]string{
		"field of a value":            `echo {{.Path.Missing}}`,
		"field of a $ value":          `echo {{$.Path.Missing}}`,
		"field of a parenthesized":    `echo {{(.Path).Missing}}`,
		"field of a declared":         `echo {{with $x := .Path}}{{$x.Missing}}{{end}}`,
		"missing template":            `echo {{template "x"}}`,
		"argument to a value":         `echo {{.Path 1}}`,
		"call of a value":             `echo {{call .Path}}`,
		"comparison with a bool":      `echo {{if eq .IsDir true}}dir{{end}}`,
		"index past every reference":  `echo {{index .Path 99}}`,
		"slice past every reference":  `echo {{slice .Path 0 99}}`,
		"field after a valid command": `echo {{.Path}} {{.Name.Missing}}`,
	}
	for _, shell := range testedShells(t) {
		for name, cmd := range commands {
			t.Run(shell.String()+"/"+name, func(t *testing.T) {
				err := command.SingleCmd{Shell: shell, Cmd: cmd}.CheckTemplates()
				if err == nil || !strings.HasPrefix(err.Error(), "can't execute command: ") {
					t.Fatalf("CheckTemplates(%q) = %v, want a can't execute command error", cmd, err)
				}
			})
		}
	}
}

func TestCheckTemplatesAcceptsCommandThatRuns(t *testing.T) {
	commands := map[string]string{
		"variable":            `echo {{.Path}}`,
		"$ variable":          `echo {{$.Path}}`,
		"variable in define":  `{{define "x"}}echo {{.Path}}{{end}}{{template "x" .}}`,
		"variables in branch": `echo {{if .Event}}{{.Name}}{{else}}{{.ExtName}}{{end}}`,
		"printf of variables": `echo {{printf "%s/%s" .ParentPath .Name}}`,
	}
	event := &domain.Event{Path: "dir/b.txt", Operation: domain.Operation(fsnotify.Create)}
	for _, shell := range testedShells(t) {
		for name, cmd := range commands {
			t.Run(shell.String()+"/"+name, func(t *testing.T) {
				c := command.SingleCmd{Shell: shell, Cmd: cmd}
				if err := c.CheckTemplates(); err != nil {
					t.Fatalf("CheckTemplates(%q) = %v", cmd, err)
				}
				if _, err := c.Build(t.Context(), domain.Path(t.TempDir()), event); err != nil {
					t.Fatalf("Build(%q) = %v", cmd, err)
				}
			})
		}
	}
}

// 검사는 실제 이벤트 값을 모르므로, 값에 따라 실패하는 명령은 검사를 통과하고 실행할 때 실패한다.
// 셸 참조는 모두 9글자보다 길고 이벤트 경로 b.txt는 그보다 짧다.
func TestCheckTemplatesAcceptsCommandThatFailsOnlyForSomeValues(t *testing.T) {
	event := &domain.Event{Path: "b.txt", Operation: domain.Operation(fsnotify.Create)}
	for _, shell := range testedShells(t) {
		t.Run(shell.String(), func(t *testing.T) {
			c := command.SingleCmd{Shell: shell, Cmd: `echo {{index .Path 9}}`}
			if err := c.CheckTemplates(); err != nil {
				t.Fatalf("CheckTemplates = %v", err)
			}
			_, err := c.Build(t.Context(), domain.Path(t.TempDir()), event)
			if err == nil || !strings.HasPrefix(err.Error(), "can't execute command: ") {
				t.Fatalf("Build = %v, want a can't execute command error", err)
			}
		})
	}
}

func TestCheckTemplatesKeepsNameAndParseErrors(t *testing.T) {
	tests := map[string]string{
		`echo {{.Path.Missing}} {{.FilePath}}`: `unknown template variable "FilePath"; available: Path, AbsPath, Name, ExtName, ParentPath, ParentAbsPath, Event, IsDir`,
		`echo {{foo .Path}}`:                   `can't parse command: template: parser:1: function "foo" not defined`,
	}
	for cmd, want := range tests {
		err := command.SingleCmd{Shell: command.DefaultShell(), Cmd: cmd}.CheckTemplates()
		if err == nil || err.Error() != want {
			t.Errorf("CheckTemplates(%q) = %v, want %q", cmd, err, want)
		}
	}
}
