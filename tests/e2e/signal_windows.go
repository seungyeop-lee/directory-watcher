//go:build windows

package e2e

import (
	"os/exec"
)

func setupProcGroup(cmd *exec.Cmd) {}

// gracefulStop on Windows force-kills the process. Graceful shutdown via
// CTRL_BREAK_EVENT is not supported because the production code listens only
// for SIGINT/SIGTERM (syscall.SIGBREAK is not in the notify list), and
// CTRL_C_EVENT cannot be delivered to a process group other than the caller's.
// Consequently the onFinishWatch hooks are not verified on Windows.
func gracefulStop(cmd *exec.Cmd) error {
	return cmd.Process.Kill()
}

const canVerifyFinishHooks = false
