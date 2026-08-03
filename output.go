package main

import (
	"bytes"
	"encoding/json"
	"io"
	"time"
)

// clip is one clipboard change that is about to be emitted.
type clip struct {
	time time.Time
	text []byte
}

// sink receives the clips that should be emitted.
type sink interface {
	emit(c clip) error
}

// framing decides how clips are delimited on stdout.
type framing int

const (
	frameLines framing = iota // clip, then a newline unless it already ends with one
	frameNull                 // clip, then a NUL byte
	frameJSON                 // one JSON object per line
)

// jsonTimeLayout is RFC 3339 with milliseconds, so clips copied within the
// same second still sort correctly.
const jsonTimeLayout = "2006-01-02T15:04:05.000Z07:00"

type jsonClip struct {
	Time  string `json:"time"`
	Bytes int    `json:"bytes"`
	Text  string `json:"text"`
}

// writerSink frames clips onto an io.Writer.
type writerSink struct {
	w         io.Writer
	framing   framing
	timestamp bool // prefix plain and NUL records with the time and a tab
	buf       bytes.Buffer
}

func (s *writerSink) emit(c clip) error {
	s.buf.Reset()
	switch s.framing {
	case frameJSON:
		enc := json.NewEncoder(&s.buf)
		enc.SetEscapeHTML(false) // keep URLs with & readable
		err := enc.Encode(jsonClip{
			Time:  c.time.Format(jsonTimeLayout),
			Bytes: len(c.text),
			Text:  string(c.text),
		})
		if err != nil {
			return err
		}
	default:
		if s.timestamp {
			s.buf.WriteString(c.time.Format(time.RFC3339))
			s.buf.WriteByte('\t')
		}
		s.buf.Write(c.text)
		switch {
		case s.framing == frameNull:
			s.buf.WriteByte(0)
		case !bytes.HasSuffix(c.text, []byte{'\n'}):
			s.buf.WriteByte('\n')
		}
	}
	// One Write per clip, straight to the file: a reader on the other end
	// of a pipe sees each record whole, as soon as it is copied.
	_, err := s.w.Write(s.buf.Bytes())
	return err
}
