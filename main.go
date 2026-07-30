// Command clipwatch streams new clipboard contents to stdout.
//
// It polls the system clipboard at a fixed interval and prints each new
// clip once, as it changes, so the clipboard can be fed into a pipeline:
//
//	clipwatch | grep -E '^https?://' >> links.txt
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"
)

const (
	exitOK    = 0
	exitError = 1
	exitUsage = 2
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

// run is main without the os.Exit, so the CLI can be exercised from tests.
func run(args []string, stdout, stderr io.Writer) int {
	cfg, err := parseArgs(args)
	if errors.Is(err, flag.ErrHelp) {
		fmt.Fprint(stdout, usage)
		return exitOK
	}
	if err != nil {
		fmt.Fprintf(stderr, "clipwatch: %v\nRun 'clipwatch --help' for usage.\n", err)
		return exitUsage
	}
	if cfg.showVersion {
		fmt.Fprintf(stdout, "clipwatch %s\n", versionString())
		return exitOK
	}

	backend, err := selectBackend(cfg.backend, systemEnv())
	if err != nil {
		fmt.Fprintf(stderr, "clipwatch: %v\n", err)
		return exitError
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	w := newWatcher(cfg, backend, stdout, stderr)
	if err := w.run(ctx); err != nil {
		fmt.Fprintf(stderr, "clipwatch: %v\n", err)
		return exitError
	}
	return exitOK
}
