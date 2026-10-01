//go:build unix

package main

import (
	"os/exec"
	"syscall"
)

// killGroupOnCancel runs cmd in a process group of its own and makes
// cancelling it kill the whole group, so that nothing cmd started outlives
// it. Without this only cmd itself is killed: for "sh -c CMD" that is the
// shell, and the real command carries on as an orphan.
func killGroupOnCancel(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error { return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL) }
}
