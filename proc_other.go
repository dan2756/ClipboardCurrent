//go:build !unix

package main

import (
	"os/exec"
	"syscall"
)

// killGroupOnCancel is only implemented on Unix; elsewhere cancelling cmd
// kills just cmd itself.
func killGroupOnCancel(*exec.Cmd) {}

// configureExecCmd asks the command to exit when the watch is stopped.
// There is no process-group kill off Unix, so only the command itself is
// signalled.
func configureExecCmd(cmd *exec.Cmd) {
	cmd.Cancel = func() error { return cmd.Process.Signal(syscall.SIGTERM) }
}

// killExecGroup is only implemented on Unix.
func killExecGroup(int) {}
