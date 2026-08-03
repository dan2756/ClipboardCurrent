package main

import (
	"regexp"
	"testing"
)

func TestFilterRegex(t *testing.T) {
	cfg := config{filter: regexp.MustCompile(`^https?://`)}
	r := watch(t, cfg, states("", "https://go.dev", "not a link", "see http://x.org", "http://example.com/a?b=c"))
	if want := "https://go.dev\nhttp://example.com/a?b=c\n"; r.stdout != want {
		t.Errorf("stdout = %q, want %q", r.stdout, want)
	}
}

func TestFilterAppliesToCurrentClipboard(t *testing.T) {
	cfg := config{includeCurrent: true, filter: regexp.MustCompile(`^\d+$`)}
	r := watch(t, cfg, states("hello", "42"))
	if want := "42\n"; r.stdout != want {
		t.Errorf("stdout = %q, want %q", r.stdout, want)
	}
}

func TestTrim(t *testing.T) {
	r := watch(t, config{trim: true}, states("", "  padded \n", "\t\n", "clean"))
	if want := "padded\nclean\n"; r.stdout != want {
		t.Errorf("stdout = %q, want %q", r.stdout, want)
	}
}

func TestTrimRunsBeforeFilter(t *testing.T) {
	cfg := config{trim: true, filter: regexp.MustCompile(`^https://\S+$`)}
	r := watch(t, cfg, states("", "  https://go.dev/doc\n"))
	if want := "https://go.dev/doc\n"; r.stdout != want {
		t.Errorf("stdout = %q, want %q", r.stdout, want)
	}
}

func TestIgnoreWhitespaceOnly(t *testing.T) {
	script := states("", " ", "a", "\n\t \n", "b")

	r := watch(t, config{framing: frameNull}, script)
	if want := " \x00a\x00\n\t \n\x00b\x00"; r.stdout != want {
		t.Errorf("without flag: stdout = %q, want %q", r.stdout, want)
	}

	r = watch(t, config{framing: frameNull, ignoreWhitespaceOnly: true}, script)
	if want := "a\x00b\x00"; r.stdout != want {
		t.Errorf("with flag: stdout = %q, want %q", r.stdout, want)
	}
}

func TestMaxBytes(t *testing.T) {
	r := watch(t, config{maxBytes: 5}, states("", "12345", "123456", "abc"))
	if want := "12345\nabc\n"; r.stdout != want {
		t.Errorf("stdout = %q, want %q", r.stdout, want)
	}
}

func TestMaxBytesCountsTrimmedText(t *testing.T) {
	r := watch(t, config{maxBytes: 3, trim: true}, states("", "  abc  \n"))
	if want := "abc\n"; r.stdout != want {
		t.Errorf("stdout = %q, want %q", r.stdout, want)
	}
}
