package main

import (
	"bytes"
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
