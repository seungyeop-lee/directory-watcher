//go:build !windows

package command

import (
	"context"
	"os/exec"
)

const defaultShellName = "sh"

// cmd와 powershell은 Windows에만 있는 셸이라 실행 방식이 없다. LookupShell이 이 셸을 고른 설정을 시작할 때 거부한다.
var cmdExeCommand, powershellCommand func(ctx context.Context, script string) *exec.Cmd
