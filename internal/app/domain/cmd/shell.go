package cmd

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"unicode"
)

// Shell은 hook 명령 문자열을 실행하는 셸이다. 셸을 더 지원하려면 shells에 항목을 추가한다.
type Shell struct {
	name string
	// command는 script를 이 셸로 실행하는 프로세스를 만든다. 이 OS에 없는 셸이면 nil이다.
	command func(ctx context.Context, script string) *exec.Cmd
	// templateRef는 템플릿 자리에 넣을 환경 변수 참조를 만든다. 템플릿을 따옴표 밖에 쓰면 참조가
	// 값을 인자 하나로 펼치고, 값 안의 셸 문법은 어느 자리에서도 다시 해석되지 않아야 한다.
	templateRef func(env string) string
	// argValue는 셸이 펼친 참조가 프로그램에 원래 값 그대로 닿도록 바꾼 값을 만든다. nil이 아니면
	// templateRef는 원래 환경 변수 대신 이 값을 담은 <환경 변수>_ARG를 참조한다.
	argValue func(value string) string
}

// argEnvSuffix는 argValue가 만든 값을 담는 환경 변수 이름의 접미사다.
const argEnvSuffix = "_ARG"

var shells = []Shell{
	{
		name: "sh",
		command: func(ctx context.Context, script string) *exec.Cmd {
			return exec.CommandContext(ctx, "sh", "-c", script)
		},
		templateRef: func(env string) string { return `"$` + env + `"` },
	},
	{
		name:    "cmd",
		command: cmdExeCommand,
		// %VAR%는 cmd.exe가 & 같은 특수 문자를 해석하기 전에 펼치므로, 템플릿을 큰따옴표 안에 쓰면
		// 값의 &가 명령을 나눈다. 지연 확장 !VAR!은 해석이 끝난 뒤 펼치므로 값이 명령이 되지 않는다.
		templateRef: func(env string) string { return "!" + env + "!" },
		// 따옴표까지 값에 넣는다. 값이 비어 있어도 변수가 정의된 채 남고(cmd.exe는 빈 변수의 !VAR!을 글자
		// 그대로 둔다), 끝의 \가 닫는 따옴표를 글자로 만들지 않는다.
		argValue: func(value string) string { return `"` + doubleTrailingBackslashes(value) + `"` },
	},
	{
		name:        "powershell",
		command:     powershellCommand,
		templateRef: func(env string) string { return "${env:" + env + "}" },
		// Windows PowerShell 5.1은 공백이 든 인자를 큰따옴표로 감쌀 때 끝의 \를 두 번 쓰지 않는다.
		argValue: func(value string) string {
			if strings.IndexFunc(value, unicode.IsSpace) < 0 {
				return value
			}
			return doubleTrailingBackslashes(value)
		},
	},
}

// doubleTrailingBackslashes는 값 끝의 \를 두 번씩 쓴다. 프로그램은 명령줄을 인자로 나눌 때 닫는 따옴표 앞의
// \를 2개마다 1개로 읽고, 홀수 개면 따옴표를 글자로 읽는다(CommandLineToArgvW 규칙).
func doubleTrailingBackslashes(value string) string {
	trimmed := strings.TrimRight(value, `\`)
	return value + value[len(trimmed):]
}

func (s Shell) String() string { return s.name }

// ShellNames는 설정 파일의 shell에 쓸 수 있는 이름을 모두 돌려준다. 이 OS에서 쓸 수 없는 셸의 이름도 들어 있다.
func ShellNames() []string {
	names := make([]string, len(shells))
	for i, shell := range shells {
		names[i] = shell.name
	}
	return names
}

// LookupShell은 설정 파일의 shell 값에 해당하는 셸을 돌려준다. 목록에 없거나 이 OS에서 쓸 수 없는 셸이면 오류를 낸다.
func LookupShell(name string) (Shell, error) {
	for _, shell := range shells {
		if shell.name == name {
			if shell.command == nil {
				return Shell{}, fmt.Errorf("shell %q runs only on Windows", name)
			}
			return shell, nil
		}
	}
	return Shell{}, fmt.Errorf("unknown shell %q: must be one of %s", name, strings.Join(ShellNames(), ", "))
}

// DefaultShell은 설정 파일에 shell이 없을 때 쓰는 셸이다. Windows에서는 cmd, 그 밖의 OS에서는 sh다.
func DefaultShell() Shell {
	shell, err := LookupShell(defaultShellName)
	if err != nil {
		panic(err)
	}
	return shell
}
