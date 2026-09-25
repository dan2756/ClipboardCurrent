//go:build unix

package main

import (
	"os"

	"golang.org/x/sys/unix"
)

// readerGone returns a check reporting whether f is a pipe whose read end
// has been closed, or nil if f is not a pipe at all.
//
// A write to such a pipe fails with EPIPE, but clipwatch may have nothing
// to write for hours. Checking the descriptor on every poll lets
// "clipwatch | head -3" finish as soon as head exits, instead of on the
// next copy.
func readerGone(f *os.File) func() bool {
	fi, err := f.Stat()
	if err != nil || fi.Mode()&os.ModeNamedPipe == 0 {
		return nil
	}
	fd := int32(f.Fd())
	return func() bool {
		// Linux flags a widowed pipe with POLLERR, macOS and the BSDs with
		// POLLHUP. POLLOUT is requested because poll on macOS reports
		// nothing at all for a descriptor polled with no events.
		fds := []unix.PollFd{{Fd: fd, Events: unix.POLLOUT}}
		n, err := unix.Poll(fds, 0)
		return err == nil && n > 0 && fds[0].Revents&(unix.POLLERR|unix.POLLHUP) != 0
	}
}
