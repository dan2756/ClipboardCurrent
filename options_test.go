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
	var stdout, stderr bytes.Buffer
	if code := run([]string{"--nope"}, &stdout, &stderr); code != exitUsage {
		t.Errorf("exit %d, want %d", code, exitUsage)
	}
	if !strings.Contains(stderr.String(), "clipwatch --help") {
		t.Errorf("stderr = %q, want a pointer to --help", stderr.String())
	}
}
