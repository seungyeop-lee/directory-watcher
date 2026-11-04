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

// CheckTemplates는 이벤트를 받는 hook의 명령을 감시를 시작하기 전에 검사한다. 명령이 eventVariables에 없는
// 템플릿 변수를 쓰거나, 템플릿을 실행할 때 반드시 실패하면 오류를 낸다.
//   - Go 템플릿은 값 map에 없는 변수를 만나도 오류 없이 <no value>를 넣으므로, 변수 이름은 따로 검사한다.
//   - 실행 검사는 Build의 첫 실행과 같은 c.Shell의 참조 값으로 템플릿을 실행한다. {{.Path.Missing}}처럼
//     어떤 이벤트에서도 실패하는 명령은 여기서 걸리고, {{index .Path 9}}처럼 실제 값에 따라 실패하는 명령은
//     실행할 때 실패한다.
func (c SingleCmd) CheckTemplates() error {
	t, err := c.parseTemplate()
	if err != nil {
		return err
	}
	names, _ := EventVariableNames()
	for _, defined := range t.Templates() {
		for _, name := range referencedVariables(defined.Root) {
			if !slices.Contains(names, name) {
				return fmt.Errorf("unknown template variable %q; available: %s", name, strings.Join(names, ", "))
			}
		}
	}
	_, err = execute(t, c.templateRefs())
	return err
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
