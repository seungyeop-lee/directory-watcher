//go:build windows

package cmd

import (
	"context"
	"os/exec"
	"syscall"
)

const defaultShellName = "cmd"

// cmdExeCommand는 script를 cmd.exe로 실행한다. cmd.exe는 Go가 인자를 감쌀 때 쓰는 \" 이스케이프를
// 해석하지 않으므로 명령줄을 직접 만든다. /s는 /c 뒤의 바깥 따옴표 한 쌍만 걷어 내 script를 그대로
// 실행하게 하고, /v:on은 템플릿 참조가 쓰는 지연 확장 !VAR!을 켠다. /d는 레지스트리의 AutoRun 명령을 건너뛴다.
func cmdExeCommand(ctx context.Context, script string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, "cmd.exe", "/d", "/v:on", "/s", "/c", script)
	cmd.SysProcAttr = &syscall.SysProcAttr{CmdLine: `cmd.exe /d /v:on /s /c "` + script + `"`}
	return cmd
}

// powershellCommand는 script를 Windows PowerShell로 실행한다. 사용자 프로필을 읽지 않고, 입력을 기다리는
// 확인 질문은 오류로 끝낸다.
func powershellCommand(ctx context.Context, script string) *exec.Cmd {
	return exec.CommandContext(ctx, "powershell.exe", "-NoProfile", "-NonInteractive", "-Command", script)
}
