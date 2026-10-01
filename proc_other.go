//go:build !unix

package main

import "os/exec"

// killGroupOnCancel is only implemented on Unix; elsewhere cancelling cmd
// kills just cmd itself.
func killGroupOnCancel(*exec.Cmd) {}
