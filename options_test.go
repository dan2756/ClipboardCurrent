package main

import (
	"bytes"
	"strings"
	"testing"
	"time"
)

func TestParseArgsDefaults(t *testing.T) {
	cfg, err := parseArgs(nil)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.interval != 500*time.Millisecond {
		t.Errorf("interval = %v, want 500ms", cfg.interval)
	}
	if cfg.backend != "auto" {
		t.Errorf("backend = %q, want auto", cfg.backend)
	}
	if cfg.includeCurrent {
		t.Error("includeCurrent should default to false")
	}
}

func TestParseInterval(t *testing.T) {
	tests := []struct {
		args []string
		want time.Duration
	}{
		{[]string{"-i", "250ms"}, 250 * time.Millisecond},
		{[]string{"--interval", "2s"}, 2 * time.Second},
		{[]string{"--interval=1.5s"}, 1500 * time.Millisecond},
		{[]string{"-i", "100"}, 100 * time.Millisecond},
	}
	for _, tt := range tests {
		cfg, err := parseArgs(tt.args)
		if err != nil {
			t.Errorf("%v: %v", tt.args, err)
			continue
		}
		if cfg.interval != tt.want {
			t.Errorf("%v: interval = %v, want %v", tt.args, cfg.interval, tt.want)
		}
	}
}

func TestParseArgsErrors(t *testing.T) {
	tests := []struct {
		args []string
		want string
	}{
		{[]string{"-i", "fast"}, "want a duration"},
		{[]string{"-i", "1ms"}, "at least 10ms"},
		{[]string{"-i", "-5"}, "out of range"},
		{[]string{"extra"}, `unexpected argument "extra"`},
		{[]string{"--bogus"}, "not defined"},
		{[]string{"--backend", "klipper"}, `unknown backend "klipper"`},
	}
	for _, tt := range tests {
		_, err := parseArgs(tt.args)
		if err == nil || !strings.Contains(err.Error(), tt.want) {
			t.Errorf("%v: err = %v, want it to contain %q", tt.args, err, tt.want)
		}
	}
}

func TestRunHelpAndVersion(t *testing.T) {
	for _, arg := range []string{"-h", "--help"} {
		var stdout, stderr bytes.Buffer
		if code := run([]string{arg}, &stdout, &stderr); code != exitOK {
			t.Errorf("%s: exit %d, want 0", arg, code)
		}
		if !strings.Contains(stdout.String(), "Usage:") {
			t.Errorf("%s: usage not printed to stdout: %q", arg, stdout.String())
		}
	}

	var stdout, stderr bytes.Buffer
	if code := run([]string{"--version"}, &stdout, &stderr); code != exitOK {
		t.Errorf("--version: exit %d", code)
	}
	if !strings.HasPrefix(stdout.String(), "clipwatch ") {
		t.Errorf("--version printed %q", stdout.String())
	}
}

func TestRunUsageError(t *testing.T) {
	for _, args := range [][]string{{"--nope"}, {"--backend", "klipper"}} {
		var stdout, stderr bytes.Buffer
		if code := run(args, &stdout, &stderr); code != exitUsage {
			t.Errorf("%v: exit %d, want %d", args, code, exitUsage)
		}
		if !strings.Contains(stderr.String(), "clipwatch --help") {
			t.Errorf("%v: stderr = %q, want a pointer to --help", args, stderr.String())
		}
	}
}

func TestParseFraming(t *testing.T) {
	tests := []struct {
		args []string
		want framing
	}{
		{nil, frameLines},
		{[]string{"-0"}, frameNull},
		{[]string{"--null"}, frameNull},
		{[]string{"--json"}, frameJSON},
	}
	for _, tt := range tests {
		cfg, err := parseArgs(tt.args)
		if err != nil {
			t.Errorf("%v: %v", tt.args, err)
			continue
		}
		if cfg.framing != tt.want {
			t.Errorf("%v: framing = %v, want %v", tt.args, cfg.framing, tt.want)
		}
	}

	for _, args := range [][]string{{"-0", "--json"}, {"--json", "--timestamp"}} {
		if _, err := parseArgs(args); err == nil {
			t.Errorf("%v: want an error", args)
		}
	}
}

func TestParseFilters(t *testing.T) {
	cfg, err := parseArgs([]string{"--filter", `^https?://`, "--max-bytes", "1MB", "--trim", "--ignore-whitespace-only"})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.filter == nil || !cfg.filter.MatchString("https://go.dev") {
		t.Errorf("filter = %v", cfg.filter)
	}
	if cfg.maxBytes != 1<<20 {
		t.Errorf("maxBytes = %d, want %d", cfg.maxBytes, 1<<20)
	}
	if !cfg.trim || !cfg.ignoreWhitespaceOnly {
		t.Errorf("trim = %v, ignoreWhitespaceOnly = %v", cfg.trim, cfg.ignoreWhitespaceOnly)
	}

	if _, err := parseArgs([]string{"--filter", "("}); err == nil || !strings.Contains(err.Error(), "--filter") {
		t.Errorf("bad regex: err = %v", err)
	}
	if _, err := parseArgs([]string{"--max-bytes", "lots"}); err == nil || !strings.Contains(err.Error(), "invalid size") {
		t.Errorf("bad size: err = %v", err)
	}
}

func TestParseDedupe(t *testing.T) {
	cfg, err := parseArgs(nil)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.dedupe != 1 {
		t.Errorf("default dedupe = %d, want 1", cfg.dedupe)
	}
	if cfg, _ := parseArgs([]string{"--dedupe", "20"}); cfg.dedupe != 20 {
		t.Errorf("dedupe = %d, want 20", cfg.dedupe)
	}
	if _, err := parseArgs([]string{"--dedupe", "-1"}); err == nil {
		t.Error("negative --dedupe should be rejected")
	}
}

func TestParseExec(t *testing.T) {
	cfg, err := parseArgs([]string{"--exec", "cat >> log"})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.exec != "cat >> log" {
		t.Errorf("exec = %q", cfg.exec)
	}
	for _, extra := range []string{"-0", "--json", "--timestamp"} {
		if _, err := parseArgs([]string{"--exec", "cat", extra}); err == nil {
			t.Errorf("--exec with %s: want an error", extra)
		}
	}
}

func TestParsePasteCmd(t *testing.T) {
	cfg, err := parseArgs([]string{"--paste-cmd", "tmux show-buffer"})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.pasteCmd != "tmux show-buffer" {
		t.Errorf("pasteCmd = %q", cfg.pasteCmd)
	}
	if _, err := parseArgs([]string{"--paste-cmd", "x", "--backend", "auto"}); err != nil {
		t.Errorf("--backend auto with --paste-cmd: %v", err)
	}
	if _, err := parseArgs([]string{"--paste-cmd", "x", "--backend", "xclip"}); err == nil {
		t.Error("--paste-cmd with --backend xclip: want an error")
	}
}
