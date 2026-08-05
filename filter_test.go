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

func TestDedupePrevious(t *testing.T) {
	r := watch(t, config{dedupe: 1}, states("", "a", "", "a", "b", "a"))
	if want := "a\nb\na\n"; r.stdout != want {
		t.Errorf("stdout = %q, want %q", r.stdout, want)
	}
}

func TestDedupeWindow(t *testing.T) {
	r := watch(t, config{dedupe: 2}, states("", "a", "b", "a", "c", "a", "b"))
	// "a" is skipped while it is one of the last two clips; once "b" and
	// "c" have pushed it out it is printed again.
	if want := "a\nb\nc\na\nb\n"; r.stdout != want {
		t.Errorf("stdout = %q, want %q", r.stdout, want)
	}
}

func TestDedupeOff(t *testing.T) {
	r := watch(t, config{dedupe: 0}, states("", "a", "", "a"))
	if want := "a\na\n"; r.stdout != want {
		t.Errorf("stdout = %q, want %q", r.stdout, want)
	}
}

func TestDedupeComparesTrimmedText(t *testing.T) {
	r := watch(t, config{dedupe: 1, trim: true}, states("", "x", "  x\n"))
	if want := "x\n"; r.stdout != want {
		t.Errorf("stdout = %q, want %q", r.stdout, want)
	}
}

func TestDedupeOnlyCountsEmittedClips(t *testing.T) {
	cfg := config{dedupe: 1, filter: regexp.MustCompile(`^\d+$`)}
	r := watch(t, cfg, states("", "1", "not a number", "1", "2"))
	if want := "1\n2\n"; r.stdout != want {
		t.Errorf("stdout = %q, want %q", r.stdout, want)
	}
}

func TestDedupeIncludesCurrent(t *testing.T) {
	r := watch(t, config{dedupe: 1, includeCurrent: true}, states("a", "", "a", "b"))
	if want := "a\nb\n"; r.stdout != want {
		t.Errorf("stdout = %q, want %q", r.stdout, want)
	}
}

func TestHistoryRing(t *testing.T) {
	h := newHistory(3)
	for _, s := range []string{"a", "b", "c"} {
		if h.seen([]byte(s)) {
			t.Fatalf("%q reported as seen on first sight", s)
		}
	}
	if !h.seen([]byte("a")) {
		t.Error("a should still be in the window")
	}
	h.seen([]byte("d")) // evicts a
	if h.seen([]byte("a")) {
		t.Error("a should have been evicted by d")
	}
	if len(h.sums) != 3 {
		t.Errorf("history holds %d sums, want 3", len(h.sums))
	}
}
