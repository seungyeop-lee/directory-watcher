//go:build !windows

package cmd

// Windows가 아닌 OS의 콘솔에는 code page가 없어서 바꿀 것이 없다.
func useUTF8ConsoleOutput() (restore func()) { return func() {} }
