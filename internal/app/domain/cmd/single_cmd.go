package cmd

import (
	"context"
	"fmt"
	"os"
	"slices"
	"strings"
	"text/template"
	"text/template/parse"

	"github.com/moby/term"

	"github.com/seungyeop-lee/directory-watcher/v3/internal/app/domain"
	"github.com/seungyeop-lee/directory-watcher/v3/internal/helper"
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
	{"Name", "DW_NAME", func(e *domain.Event) string { return e.Path.Name() }},
	{"ExtName", "DW_EXT_NAME", func(e *domain.Event) string { return e.Path.ExtName() }},
	{"ParentPath", "DW_PARENT_PATH", func(e *domain.Event) string { return e.Path.ParentPath() }},
	{"ParentAbsPath", "DW_PARENT_ABS_PATH", func(e *domain.Event) string { return e.Path.ParentAbsPath() }},
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

	t, err := c.parseTemplate()
	if err != nil {
		return "", "", err
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

func (c SingleCmd) parseTemplate() (*template.Template, error) {
	t, err := template.New("parser").Parse(c.Cmd)
	if err != nil {
		return nil, fmt.Errorf("can't parse command: %s", err)
	}
	return t, nil
}

// CheckTemplates는 명령이 eventVariables에 없는 템플릿 변수를 쓰면 오류를 낸다. Go 템플릿은 값 map에 없는
// 변수를 만나도 오류 없이 <no value>를 넣으므로, 이벤트를 받는 hook의 명령은 감시를 시작하기 전에 이 검사를 거친다.
func (c SingleCmd) CheckTemplates() error {
	t, err := c.parseTemplate()
	if err != nil {
		return err
	}
	var names []string
	for _, v := range eventVariables {
		names = append(names, v.template)
	}
	for _, defined := range t.Templates() {
		for _, name := range referencedVariables(defined.Root) {
			if !slices.Contains(names, name) {
				return fmt.Errorf("unknown template variable %q; available: %s", name, strings.Join(names, ", "))
			}
		}
	}
	return nil
}

// referencedVariables는 템플릿 node 아래에서 {{.Path}}나 {{$.Path}}처럼 템플릿 값에서 읽는 변수 이름을 모은다.
func referencedVariables(node parse.Node) []string {
	switch n := node.(type) {
	case *parse.FieldNode:
		return n.Ident[:1]
	case *parse.VariableNode:
		if n.Ident[0] == "$" && len(n.Ident) > 1 {
			return n.Ident[1:2]
		}
	case *parse.ChainNode:
		return referencedVariables(n.Node)
	case *parse.ListNode:
		return referencedVariablesIn(n.Nodes)
	case *parse.ActionNode:
		return referencedVariables(n.Pipe)
	case *parse.PipeNode:
		// {{template "name"}}처럼 값을 넘기지 않는 TemplateNode는 Pipe가 nil이다.
		if n == nil {
			return nil
		}
		var names []string
		for _, cmd := range n.Cmds {
			names = append(names, referencedVariables(cmd)...)
		}
		return names
	case *parse.CommandNode:
		return referencedVariablesIn(n.Args)
	case *parse.TemplateNode:
		return referencedVariables(n.Pipe)
	case *parse.IfNode:
		return referencedVariablesInBranch(n.BranchNode)
	case *parse.RangeNode:
		return referencedVariablesInBranch(n.BranchNode)
	case *parse.WithNode:
		return referencedVariablesInBranch(n.BranchNode)
	}
	return nil
}

func referencedVariablesIn(nodes []parse.Node) []string {
	var names []string
	for _, node := range nodes {
		names = append(names, referencedVariables(node)...)
	}
	return names
}

func referencedVariablesInBranch(b parse.BranchNode) []string {
	names := referencedVariables(b.Pipe)
	names = append(names, referencedVariables(b.List)...)
	if b.ElseList != nil {
		names = append(names, referencedVariables(b.ElseList)...)
	}
	return names
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
