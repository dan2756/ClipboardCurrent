package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"math"
	"regexp"
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

Polling:
  -i, --interval DUR         poll interval, e.g. 250ms or 2s; a bare number is
                             milliseconds (default 500ms)
      --backend NAME         auto, pbpaste, wl-paste, xclip or xsel (default auto)
      --paste-cmd CMD        read the clipboard by running sh -c CMD
      --include-current      also print what is on the clipboard at startup

Output:
  -0, --null                 end each clip with NUL instead of a newline
      --json                 one JSON object per line: {"time","bytes","text"}
      --timestamp            prefix each clip with an RFC 3339 time and a tab
      --exec CMD             run sh -c CMD once per clip with the clip on stdin
                             and CLIPWATCH_BYTES set, instead of printing

Filters:
      --filter REGEX         only emit clips matching REGEX (Go syntax)
      --max-bytes SIZE       skip clips larger than SIZE, e.g. 64KB or 1MB
      --trim                 strip leading and trailing whitespace
      --ignore-whitespace-only
                             skip clips made only of whitespace
      --dedupe N             skip clips identical to one of the last N emitted
                             (default 1; 0 disables)

  -h, --help                 show this help
      --version              print the version

Examples:
  clipwatch | grep -E '^https?://' >> links.txt
  clipwatch -0 | xargs -0 -n1 notify-send Copied
  clipwatch --json | jq -r .text
  clipwatch --trim --filter '^[0-9a-f]{40}$' --max-bytes 1KB
  clipwatch --exec 'notify-send "Copied $CLIPWATCH_BYTES bytes"'
  clipwatch --paste-cmd 'xclip -selection primary -o'
`

// minInterval keeps a typo like "-i 1" from becoming a busy loop of
// subprocesses; 10ms is already far faster than anyone copies.
const minInterval = 10 * time.Millisecond

// config is the parsed command line.
type config struct {
	interval       time.Duration
	backend        string
	pasteCmd       string
	includeCurrent bool
	framing        framing
	timestamp      bool
	exec           string

	filter               *regexp.Regexp
	maxBytes             int64
	trim                 bool
	ignoreWhitespaceOnly bool
	dedupe               int

	showVersion bool
}

func parseArgs(args []string) (config, error) {
	cfg := config{
		interval: 500 * time.Millisecond,
		backend:  "auto",
		dedupe:   1,
	}

	fs := flag.NewFlagSet("clipwatch", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.Usage = func() {}

	interval := (*durationValue)(&cfg.interval)
	fs.Var(interval, "i", "")
	fs.Var(interval, "interval", "")
	fs.StringVar(&cfg.backend, "backend", cfg.backend, "")
	fs.StringVar(&cfg.pasteCmd, "paste-cmd", "", "")
	fs.BoolVar(&cfg.includeCurrent, "include-current", false, "")
	var null, jsonOut bool
	fs.BoolVar(&null, "0", false, "")
	fs.BoolVar(&null, "null", false, "")
	fs.BoolVar(&jsonOut, "json", false, "")
	fs.BoolVar(&cfg.timestamp, "timestamp", false, "")
	fs.StringVar(&cfg.exec, "exec", "", "")
	var filter string
	fs.StringVar(&filter, "filter", "", "")
	fs.Var((*sizeValue)(&cfg.maxBytes), "max-bytes", "")
	fs.BoolVar(&cfg.trim, "trim", false, "")
	fs.BoolVar(&cfg.ignoreWhitespaceOnly, "ignore-whitespace-only", false, "")
	fs.IntVar(&cfg.dedupe, "dedupe", cfg.dedupe, "")
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
	if cfg.dedupe < 0 {
		return cfg, errors.New("--dedupe must be 0 or more")
	}
	if cfg.pasteCmd != "" && cfg.backend != "auto" {
		return cfg, errors.New("--paste-cmd and --backend cannot be used together")
	}
	switch {
	case cfg.exec != "" && (null || jsonOut || cfg.timestamp):
		return cfg, errors.New("--exec replaces printing; it cannot be combined with --null, --json or --timestamp")
	case null && jsonOut:
		return cfg, errors.New("--null and --json cannot be used together")
	case jsonOut && cfg.timestamp:
		return cfg, errors.New("--timestamp has no effect with --json, which always includes the time")
	case jsonOut:
		cfg.framing = frameJSON
	case null:
		cfg.framing = frameNull
	}
	if filter != "" {
		re, err := regexp.Compile(filter)
		if err != nil {
			return cfg, fmt.Errorf("--filter: %v", err)
		}
		cfg.filter = re
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
