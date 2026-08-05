package main

import (
	"bytes"
	"crypto/sha256"
	"regexp"
)

// filters decide which clips are emitted, and in what form.
type filters struct {
	trim           bool           // strip leading and trailing whitespace first
	skipWhitespace bool           // drop clips that are only whitespace
	maxBytes       int64          // drop clips larger than this; 0 means no limit
	match          *regexp.Regexp // keep only clips matching this, if set
}

// apply returns the text to emit for a raw clipboard value, or ok=false if
// the clip should be skipped. Limits apply to the text after trimming, i.e.
// to what would actually be emitted. Empty clips are always skipped.
func (f *filters) apply(raw []byte) (text []byte, ok bool) {
	text = raw
	if f.trim {
		text = bytes.TrimSpace(text)
	}
	switch {
	case len(text) == 0:
		return nil, false
	case f.skipWhitespace && len(bytes.TrimSpace(text)) == 0:
		return nil, false
	case f.maxBytes > 0 && int64(len(text)) > f.maxBytes:
		return nil, false
	case f.match != nil && !f.match.Match(text):
		return nil, false
	}
	return text, true
}

// history remembers the last n emitted clips, by hash, so repeats can be
// skipped without keeping every clip in memory.
type history struct {
	size int
	sums [][sha256.Size]byte
	next int // slot to overwrite once sums is full
}

func newHistory(n int) *history {
	return &history{size: n, sums: make([][sha256.Size]byte, 0, min(n, 64))}
}

// seen reports whether text is among the remembered clips, and remembers
// it if it is not. Repeats do not refresh their position: the window is
// the last n clips that were actually emitted.
func (h *history) seen(text []byte) bool {
	if h.size <= 0 {
		return false
	}
	sum := sha256.Sum256(text)
	for _, s := range h.sums {
		if s == sum {
			return true
		}
	}
	if len(h.sums) < h.size {
		h.sums = append(h.sums, sum)
	} else {
		h.sums[h.next] = sum
		h.next = (h.next + 1) % h.size
	}
	return false
}
