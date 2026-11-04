package command

import (
	"context"
	"os"

	"github.com/moby/term"

	"github.com/seungyeop-lee/directory-watcher/v3/internal/app/domain"
	"github.com/seungyeop-lee/directory-watcher/v3/internal/process"
)

// SingleCmd는 설정 파일에 적힌 hook 명령 문자열 하나를 Shell로 실행한다.
type SingleCmd struct {
	Shell Shell
	Cmd   string
}

var _ ExecCmdBuilder = (*SingleCmd)(nil)

func (c SingleCmd) Build(ctx context.Context, runDir domain.Path, event *domain.Event) ([]Command, error) {
	if c.Cmd == "" {
		return nil, newEmptyCmdError()
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

	process.SetupForOs(cmd)

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

	t, err := c.parseTemplate()
	if err != nil {
		return "", "", err
	}

	values := map[string]string{}
	for _, v := range eventVariables {
		values[v.template] = v.value(event)
	}
	if script, err = execute(t, c.templateRefs()); err != nil {
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

// templateRefs는 템플릿 변수마다 그 값을 담은 환경 변수를 읽는 c.Shell의 참조다.
func (c SingleCmd) templateRefs() map[string]string {
	refs := map[string]string{}
	for _, v := range eventVariables {
		refs[v.template] = c.Shell.templateRef(c.refEnv(v.env))
	}
	return refs
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
