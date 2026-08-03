package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

var fixedTime = time.Date(2026, 8, 3, 10, 32, 52, 123_000_000, time.UTC)

// frozenClock makes every clip carry fixedTime.
func frozenClock(w *watcher) { w.now = func() time.Time { return fixedTime } }

func TestNullFraming(t *testing.T) {
	r := watch(t, config{framing: frameNull}, states("", "one", "two\nlines\n", "three"))
	if want := "one\x00two\nlines\n\x00three\x00"; r.stdout != want {
		t.Errorf("stdout = %q, want %q", r.stdout, want)
	}
}

func TestJSONFraming(t *testing.T) {
	r := watch(t, config{framing: frameJSON}, states("", "https://example.com/?a=1&b=<2>", "multi\nline\t\"quoted\""), frozenClock)
	if r.err != nil {
		t.Fatal(r.err)
	}

	sc := bufio.NewScanner(strings.NewReader(r.stdout))
	var got []jsonClip
	for sc.Scan() {
		var c jsonClip
		if err := json.Unmarshal(sc.Bytes(), &c); err != nil {
			t.Fatalf("line %q is not JSON: %v", sc.Text(), err)
		}
		got = append(got, c)
	}
	want := []jsonClip{
		{Time: "2026-08-03T10:32:52.123Z", Bytes: 30, Text: "https://example.com/?a=1&b=<2>"},
		{Time: "2026-08-03T10:32:52.123Z", Bytes: 19, Text: "multi\nline\t\"quoted\""},
	}
	if len(got) != len(want) {
		t.Fatalf("got %d records, want %d:\n%s", len(got), len(want), r.stdout)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("record %d = %+v, want %+v", i, got[i], want[i])
		}
	}
	if strings.Contains(r.stdout, `\u0026`) {
		t.Errorf("JSON should not HTML-escape: %s", r.stdout)
	}
}

func TestJSONBytesCountsRawBytes(t *testing.T) {
	r := watch(t, config{framing: frameJSON}, states("", "héllo ✓"), frozenClock)
	var c jsonClip
	if err := json.Unmarshal([]byte(r.stdout), &c); err != nil {
		t.Fatal(err)
	}
	if c.Bytes != len("héllo ✓") || c.Text != "héllo ✓" {
		t.Errorf("got %+v", c)
	}
}

func TestTimestampPrefix(t *testing.T) {
	r := watch(t, config{timestamp: true}, states("", "a", "b"), frozenClock)
	want := "2026-08-03T10:32:52Z\ta\n2026-08-03T10:32:52Z\tb\n"
	if r.stdout != want {
		t.Errorf("stdout = %q, want %q", r.stdout, want)
	}

	r = watch(t, config{timestamp: true, framing: frameNull}, states("", "a"), frozenClock)
	if want := "2026-08-03T10:32:52Z\ta\x00"; r.stdout != want {
		t.Errorf("null stdout = %q, want %q", r.stdout, want)
	}
}

func TestOneWritePerClip(t *testing.T) {
	var cw countingWriter
	s := &writerSink{w: &cw, framing: frameLines, timestamp: true}
	for _, text := range []string{"a", "b\n", "c"} {
		if err := s.emit(clip{time: fixedTime, text: []byte(text)}); err != nil {
			t.Fatal(err)
		}
	}
	if cw.writes != 3 {
		t.Errorf("%d writes for 3 clips, want 3", cw.writes)
	}
}

type countingWriter struct {
	bytes.Buffer
	writes int
}

func (c *countingWriter) Write(p []byte) (int, error) {
	c.writes++
	return c.Buffer.Write(p)
}
