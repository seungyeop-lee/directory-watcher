package command

import (
	"fmt"
	"slices"
	"strconv"
	"strings"
	"text/template"
	"text/template/parse"

	"github.com/seungyeop-lee/directory-watcher/v3/internal/app/domain"
)

// eventVariables는 명령 문자열의 템플릿 변수와, 그 값을 명령에 넘기는 환경 변수다.
var eventVariables = []struct {
	template, env string
	value         func(e *domain.Event) string
}{
	{"Path", "DW_PATH", func(e *domain.Event) string { return e.Path.String() }},
	{"AbsPath", "DW_ABS_PATH", func(e *domain.Event) string { return e.Path.Abs() }},
	{"Name", "DW_NAME", func(e *domain.Event) string { return e.Path.Name() }},
	{"ExtName", "DW_EXT_NAME", func(e *domain.Event) string {
		// 폴더 이름의 점 뒤 글자는 확장자가 아니다.
		if e.IsDir {
			return ""
		}
		return e.Path.ExtName()
	}},
	{"ParentPath", "DW_PARENT_PATH", func(e *domain.Event) string { return e.Path.ParentPath() }},
	{"ParentAbsPath", "DW_PARENT_ABS_PATH", func(e *domain.Event) string { return e.Path.ParentAbsPath() }},
	{"Event", "DW_EVENT", func(e *domain.Event) string { return e.Operation.String() }},
	{"IsDir", "DW_IS_DIR", func(e *domain.Event) string { return strconv.FormatBool(e.IsDir) }},
}

// EventVariableNames는 hook 명령에서 쓸 수 있는 템플릿 변수의 이름과, 같은 값을 담는 환경 변수의 이름을 같은 순서로 돌려준다.
func EventVariableNames() (templates, envs []string) {
	for _, v := range eventVariables {
		templates = append(templates, v.template)
		envs = append(envs, v.env)
	}
	return templates, envs
}

func (c SingleCmd) parseTemplate() (*template.Template, error) {
	t, err := template.New("parser").Parse(c.Cmd)
	if err != nil {
		return nil, fmt.Errorf("can't parse command: %s", err)
	}
	return t, nil
}

// CheckTemplates는 이벤트를 받는 hook의 명령을 감시를 시작하기 전에 검사한다. 명령에는 {{.Path}}나 {{$.Path}}처럼
// eventVariables의 변수를 그대로 넣는 자리만 쓸 수 있고, 다른 형태를 쓰거나 모르는 변수를 쓰면 오류를 낸다.
//   - Build가 셸에 넘기는 명령은 변수 자리를 셸 참조로 채운 템플릿 결과다. 그래서 {{if eq .Event "C"}}나
//     {{len .Path}} 같은 조건과 함수는 실제 값이 아니라 "$DW_PATH" 같은 참조 문자열을 보고 평가된다.
//   - Go 템플릿은 값 map에 없는 변수를 만나도 오류 없이 <no value>를 넣으므로, 변수 이름은 따로 검사한다.
func (c SingleCmd) CheckTemplates() error {
	t, err := c.parseTemplate()
	if err != nil {
		return err
	}
	for _, defined := range t.Templates() {
		if defined.Name() != t.Name() {
			return unsupportedActionError(fmt.Sprintf("{{define %q}}", defined.Name()))
		}
	}
	names, _ := EventVariableNames()
	// if, range, with는 만나는 즉시 거부하므로, 그 안의 node는 따로 보지 않는다.
	for _, node := range t.Root.Nodes {
		switch n := node.(type) {
		case *parse.TextNode:
		case *parse.ActionNode:
			name, ok := substitutedVariable(n.Pipe)
			if !ok {
				return unsupportedActionError(n.String())
			}
			if !slices.Contains(names, name) {
				return fmt.Errorf("unknown template variable %q; available: %s", name, strings.Join(names, ", "))
			}
		default:
			return unsupportedActionError(node.String())
		}
	}
	return nil
}

// substitutedVariable은 pipe가 {{.Path}}나 {{$.Path}}처럼 변수 하나를 그대로 읽으면 그 변수 이름을 돌려준다.
func substitutedVariable(pipe *parse.PipeNode) (name string, ok bool) {
	if len(pipe.Decl) > 0 || len(pipe.Cmds) != 1 || len(pipe.Cmds[0].Args) != 1 {
		return "", false
	}
	switch arg := pipe.Cmds[0].Args[0].(type) {
	case *parse.FieldNode:
		if len(arg.Ident) == 1 {
			return arg.Ident[0], true
		}
	case *parse.VariableNode:
		if len(arg.Ident) == 2 && arg.Ident[0] == "$" {
			return arg.Ident[1], true
		}
	}
	return "", false
}

func unsupportedActionError(action string) error {
	return fmt.Errorf("unsupported template action %s; only template variables such as {{.Path}} are supported", action)
}

func execute(t *template.Template, data map[string]string) (string, error) {
	b := strings.Builder{}
	if err := t.Execute(&b, data); err != nil {
		return "", fmt.Errorf("can't execute command: %s", err)
	}
	return b.String(), nil
}
