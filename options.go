package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"math"
	"runtime/debug"
	"strconv"
	"time"
)

const usage = `clipwatch - stream new clipboard contents to stdout

Usage:
  clipwatch [flags]

Polls the clipboard and prints each new clip once, as it changes. Whatever
is already on the clipboard at startup is skipped unless --include-current
is given. Stop with Ctrl-C.

Flags:
  -i, --interval DUR         poll interval, e.g. 250ms or 2s; a bare number is
                             milliseconds (default 500ms)
      --backend NAME         auto, pbpaste, wl-paste, xclip or xsel (default auto)
      --include-current      also print what is on the clipboard at startup
  -h, --help                 show this help
      --version              print the version

Examples:
  clipwatch | grep -E '^https?://' >> links.txt
  clipwatch --backend xsel -i 1s
`

// minInterval keeps a typo like "-i 1" from becoming a busy loop of
// subprocesses; 10ms is already far faster than anyone copies.
const minInterval = 10 * time.Millisecond

// config is the parsed command line.
type config struct {
	interval       time.Duration
	backend        string
	includeCurrent bool
	showVersion    bool
}

func parseArgs(args []string) (config, error) {
	cfg := config{
		interval: 500 * time.Millisecond,
		backend:  "auto",
	}

	fs := flag.NewFlagSet("clipwatch", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.Usage = func() {}

	interval := (*durationValue)(&cfg.interval)
	fs.Var(interval, "i", "")
	fs.Var(interval, "interval", "")
	fs.StringVar(&cfg.backend, "backend", cfg.backend, "")
	fs.BoolVar(&cfg.includeCurrent, "include-current", false, "")
	fs.BoolVar(&cfg.showVersion, "version", false, "")

	if err := fs.Parse(args); err != nil {
		return cfg, err
	}
	if fs.NArg() > 0 {
		return cfg, fmt.Errorf("unexpected argument %q", fs.Arg(0))
	}
	if cfg.interval < minInterval {
		return cfg, fmt.Errorf("--interval must be at least %v", minInterval)
	}
	return cfg, nil
}

// durationValue is a flag.Value for durations that also accepts a bare
// number of milliseconds ("-i 250").
type durationValue time.Duration

func (d *durationValue) String() string { return time.Duration(*d).String() }

func (d *durationValue) Set(s string) error {
	if n, err := strconv.ParseInt(s, 10, 64); err == nil {
		if n < 0 || n > math.MaxInt64/int64(time.Millisecond) {
			return errors.New("out of range")
		}
		*d = durationValue(time.Duration(n) * time.Millisecond)
		return nil
	}
	v, err := time.ParseDuration(s)
	if err != nil {
		return errors.New("want a duration like 500ms or 2s")
	}
	*d = durationValue(v)
	return nil
}

// version is set at build time with -ldflags "-X main.version=v1.2.3".
// Without it, the module version recorded by "go install" is used.
var version = ""

func versionString() string {
	if version != "" {
		return version
	}
	if bi, ok := debug.ReadBuildInfo(); ok && bi.Main.Version != "" && bi.Main.Version != "(devel)" {
		return bi.Main.Version
	}
	return "dev"
}
