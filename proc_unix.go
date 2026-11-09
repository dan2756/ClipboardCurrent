//go:build unix

package main

import (
	"os"
	"os/exec"
	"syscall"
	"time"
)

// killGroupOnCancel runs cmd in a process group of its own and makes
// cancelling it kill the whole group, so that nothing cmd started outlives
// it. Without this only cmd itself is killed: for "sh -c CMD" that is the
// shell, and the real command carries on as an orphan.
func killGroupOnCancel(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error { return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL) }
}

// configureExecCmd puts an --exec command in its own process group and, when
// the watch is stopped, signals that whole group. sh -c does not forward a
// signal to the program it started, so signalling only the shell leaves that
// program running.
func configureExecCmd(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		err := syscall.Kill(-cmd.Process.Pid, syscall.SIGTERM)
		if err == syscall.ESRCH {
			return os.ErrProcessDone
		}
		return err
	}
}

// killExecGroup SIGKILLs the process group led by pid. A non-positive pid is
// ignored: a released process reports -1, and kill of the negation of that
// would signal pid 1.
func killExecGroup(pid int) {
	if pid <= 0 {
		return
	}
	_ = syscall.Kill(-pid, syscall.SIGKILL)
}

// awaitExecGroup waits up to execGrace for the process group led by pid to
// exit. sh dies as soon as it is signaled and does not wait for the program
// it started; that program was signaled too, and gets this long to finish
// exiting before the group is killed. A group that is already gone returns
// at once.
func awaitExecGroup(pid int) {
	if pid <= 0 {
		return
	}
	deadline := time.Now().Add(execGrace)
	for groupAlive(pid) && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
}

// groupAlive reports whether any process in the group led by pid still
// exists. Signal 0 checks without delivering anything.
func groupAlive(pid int) bool {
	return syscall.Kill(-pid, 0) != syscall.ESRCH
}
