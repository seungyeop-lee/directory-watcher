package cmd

import (
	"context"
	"fmt"
	"os"
	"strings"
	"text/template"

	"github.com/moby/term"

	"github.com/seungyeop-lee/directory-watcher/v2/internal/app/domain"
	"github.com/seungyeop-lee/directory-watcher/v2/internal/helper"
)

// SingleCmd는 설정 파일에 적힌 hook 명령 문자열 하나를 Shell로 실행한다.
type SingleCmd struct {
	Shell Shell
	Cmd   string
}

var _ ExecCmdBuilder = (*SingleCmd)(nil)

// eventVariables는 명령 문자열의 템플릿 변수와, 그 값을 명령에 넘기는 환경 변수다.
var eventVariables = []struct {
	template, env string
	value         func(e *domain.Event) string
}{
	{"Path", "DW_PATH", func(e *domain.Event) string { return e.Path.String() }},
	{"AbsPath", "DW_ABS_PATH", func(e *domain.Event) string { return e.Path.Abs() }},
	{"FileName", "DW_FILE_NAME", func(e *domain.Event) string { return e.Path.FileName() }},
	{"ExtName", "DW_EXT_NAME", func(e *domain.Event) string { return e.Path.ExtName() }},
	{"DirPath", "DW_DIR_PATH", func(e *domain.Event) string { return e.Path.DirPath() }},
	{"DirAbsPath", "DW_DIR_ABS_PATH", func(e *domain.Event) string { return e.Path.DirAbsPath() }},
	{"Event", "DW_EVENT", func(e *domain.Event) string { return e.Operation.String() }},
}

func (c SingleCmd) Build(ctx context.Context, runDir domain.Path, event *domain.Event) ([]Command, error) {
	if c.Cmd == "" {
		return nil, helper.NewEmptyCmdError()
	}

	script, filled, err := c.replaceTemplates(event)
	if err != nil {
		return nil, err
	}

	cmd := c.Shell.command(ctx, script)
	if event != nil {
		cmd.Env = append(os.Environ(), c.eventEnvironment(event)...)
	}
	cmd.Dir = runDir.String()
	cmd.Stdin, _, _ = term.StdStreams()

	helper.SetupForOs(cmd)

	return []Command{{Cmd: cmd, Filled: filled}}, nil
}

// replaceTemplates는 실행할 명령과, 실패 로그에 보여 줄 명령을 만든다. 실행할 명령은 템플릿 변수 자리를
// 값을 담은 환경 변수의 참조로 바꾼다. 값을 명령 문자열에 넣지 않으므로 파일 이름이 셸 명령으로 해석되지
// 않는다. 실패 로그에 보여 줄 명령은 그 자리에 값을 그대로 넣고, 템플릿 변수가 없으면 비워 둔다.
// 이벤트가 없는 hook의 명령은 그대로 둔다.
func (c SingleCmd) replaceTemplates(event *domain.Event) (script, filled string, err error) {
	if event == nil {
		return c.Cmd, "", nil
	}

	t, err := template.New("parser").Parse(c.Cmd)
	if err != nil {
		return "", "", fmt.Errorf("can't parse command: %s", err)
	}

	refs, values := map[string]string{}, map[string]string{}
	for _, v := range eventVariables {
		refs[v.template] = c.Shell.templateRef(c.refEnv(v.env))
		values[v.template] = v.value(event)
	}
	if script, err = execute(t, refs); err != nil {
		return "", "", err
	}
	if filled, err = execute(t, values); err != nil {
		return "", "", err
	}
	if script == c.Cmd {
		filled = ""
	}
	return script, filled, nil
}

func execute(t *template.Template, data map[string]string) (string, error) {
	b := strings.Builder{}
	if err := t.Execute(&b, data); err != nil {
		return "", fmt.Errorf("can't execute command: %s", err)
	}
	return b.String(), nil
}

// refEnv는 템플릿 참조가 읽는 환경 변수 이름이다.
func (c SingleCmd) refEnv(env string) string {
	if c.Shell.argValue == nil {
		return env
	}
	return env + argEnvSuffix
}

func (c SingleCmd) eventEnvironment(event *domain.Event) []string {
	var env []string
	for _, v := range eventVariables {
		value := v.value(event)
		env = append(env, v.env+"="+value)
		if c.Shell.argValue != nil {
			env = append(env, c.refEnv(v.env)+"="+c.Shell.argValue(value))
		}
	}
	return env
}
