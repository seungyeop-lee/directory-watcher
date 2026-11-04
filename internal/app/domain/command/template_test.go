package command_test

import (
	"strings"
	"testing"

	"github.com/fsnotify/fsnotify"

	"github.com/seungyeop-lee/directory-watcher/v3/internal/app/domain"
	"github.com/seungyeop-lee/directory-watcher/v3/internal/app/domain/command"
)

// 셸에 넘기는 명령은 변수 자리에 셸 참조를 채운 템플릿 결과이므로, 조건과 함수는 실제 값이 아니라 참조
// 문자열을 보고 평가된다. 그래서 변수 치환이 아닌 형태는 모두 설정을 읽을 때 거부한다.
func TestCheckTemplatesRejectsAnythingButVariables(t *testing.T) {
	commands := map[string]string{
		"if comparison":            `echo {{if eq .Event "C"}}created{{else}}other{{end}}`,
		"if on a variable":         `echo {{if .IsDir}}dir{{end}}`,
		"with":                     `echo {{with $x := .Path}}{{$x}}{{end}}`,
		"range":                    `echo {{range .Path}}x{{end}}`,
		"function":                 `echo {{len .Path}}`,
		"printf":                   `echo {{printf "%s/%s" .ParentPath .Name}}`,
		"pipeline":                 `echo {{.Path | printf "%q"}}`,
		"call":                     `echo {{call .Path}}`,
		"index":                    `echo {{index .Path 9}}`,
		"slice":                    `echo {{slice .Path 0 3}}`,
		"field of a value":         `echo {{.Path.Missing}}`,
		"field of a $ value":       `echo {{$.Path.Missing}}`,
		"parenthesized":            `echo {{(.Path)}}`,
		"argument to a value":      `echo {{.Path 1}}`,
		"declaration":              `echo {{$x := .Path}}`,
		"dot":                      `echo {{.}}`,
		"$":                        `echo {{$}}`,
		"literal":                  `echo {{"text"}}`,
		"template":                 `echo {{template "x"}}`,
		"define":                   `{{define "x"}}echo {{.Path}}{{end}}echo`,
		"after a valid variable":   `echo {{.Path}} {{len .Name}}`,
		"variable inside a branch": `echo {{if .Event}}{{.Name}}{{end}}`,
	}
	for name, cmd := range commands {
		t.Run(name, func(t *testing.T) {
			err := command.SingleCmd{Shell: command.DefaultShell(), Cmd: cmd}.CheckTemplates()
			if err == nil || !strings.HasPrefix(err.Error(), "unsupported template action ") {
				t.Fatalf("CheckTemplates(%q) = %v, want an unsupported template action error", cmd, err)
			}
		})
	}
}

func TestCheckTemplatesAcceptsVariables(t *testing.T) {
	commands := map[string]string{
		"variable":               `echo {{.Path}}`,
		"$ variable":             `echo {{$.Path}}`,
		"variables next to text": `echo {{.ParentPath}}/{{.Name}} {{.Event}}`,
		"trim markers":           `echo {{- .Path -}}`,
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

func TestCheckTemplatesErrorMessages(t *testing.T) {
	tests := map[string]string{
		`echo {{.Path}} {{.FilePath}}`:                 `unknown template variable "FilePath"; available: Path, AbsPath, Name, ExtName, ParentPath, ParentAbsPath, Event, IsDir`,
		`echo {{if eq .Event "C"}}created{{end}}`:      `unsupported template action {{if eq .Event "C"}}created{{end}}; only template variables such as {{.Path}} are supported`,
		`{{define "x"}}echo {{.Path}}{{end}}echo done`: `unsupported template action {{define "x"}}; only template variables such as {{.Path}} are supported`,
		`echo {{foo .Path}}`:                           `can't parse command: template: parser:1: function "foo" not defined`,
	}
	for cmd, want := range tests {
		err := command.SingleCmd{Shell: command.DefaultShell(), Cmd: cmd}.CheckTemplates()
		if err == nil || err.Error() != want {
			t.Errorf("CheckTemplates(%q) = %v, want %q", cmd, err, want)
		}
	}
}
