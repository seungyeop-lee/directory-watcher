//go:build windows

package cmd

import "golang.org/x/sys/windows"

const utf8CodePage = 65001

// useUTF8ConsoleOutput은 watcher가 붙은 콘솔의 출력 code page를 UTF-8로 바꾸고, 원래 값으로 되돌리는 함수를 돌려준다.
// hook 셸(cmd, powershell)은 시작할 때 읽은 콘솔 출력 code page로 출력을 인코딩하는데, watcher는 hook 출력 바이트를
// UTF-8로 보고 내보내므로 두 인코딩을 맞춘다. 입력 code page는 바꾸지 않는다. 콘솔이 없으면 아무것도 하지 않는다.
// 콘솔은 그 콘솔에 붙은 모든 프로세스가 함께 쓰므로, watcher가 실행되는 동안에는 사용자의 셸도 UTF-8 출력 code page를 쓴다.
func useUTF8ConsoleOutput() (restore func()) {
	original, err := windows.GetConsoleOutputCP()
	if err != nil {
		return func() {}
	}
	if err := windows.SetConsoleOutputCP(utf8CodePage); err != nil {
		return func() {}
	}
	return func() { _ = windows.SetConsoleOutputCP(original) }
}
