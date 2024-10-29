//go:build windows

package cmd

import (
	"os"
	"os/exec"
	"syscall"
)

func terminateProcess(pid int) error {
	process, err := os.FindProcess(pid)
	if err != nil {
		return err
	}
	return process.Signal(syscall.SIGINT)
}

func setupForOs(cmd *exec.Cmd) {
}
