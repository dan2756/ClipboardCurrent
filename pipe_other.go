//go:build !unix

package main

import "os"

// readerGone is only implemented on Unix; elsewhere a closed reader is
// noticed when the next clip fails to write.
func readerGone(*os.File) func() bool { return nil }
