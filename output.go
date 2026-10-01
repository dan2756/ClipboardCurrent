package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os/exec"
	"strconv"
	"syscall"
	"time"
)

// clip is one clipboard change that is about to be emitted.
type clip struct {
	time time.Time
	text []byte
}

// sink receives the clips that should be emitted. An error from emit
// stops the watch.
type sink interface {
	emit(ctx context.Context, c clip) error
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

func (s *writerSink) emit(ctx context.Context, c clip) error {
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
	//
	// The write blocks for as long as a stalled reader leaves the pipe
	// full, so it runs on its own and is abandoned if the watch is stopped
	// meanwhile; otherwise Ctrl-C and SIGTERM would wait for the reader.
	done := make(chan error, 1)
	go func() {
		_, err := s.w.Write(s.buf.Bytes())
		done <- err
	}()
	select {
	case err := <-done:
		return err
	case <-ctx.Done():
		return ctx.Err()
	}
}

// execGrace is how long an --exec command gets to exit after clipwatch is
// asked to stop, before it is killed.
const execGrace = 2 * time.Second

// execSink runs a shell command once per clip, with the clip on stdin and
// its size in CLIPWATCH_BYTES. Commands run one at a time: polling pauses
// until each one exits, so they never overlap or reorder.
type execSink struct {
	command string
	stdout  io.Writer
	stderr  io.Writer
}

func (s *execSink) emit(ctx context.Context, c clip) error {
	cmd := exec.CommandContext(ctx, "sh", "-c", s.command)
	cmd.Stdin = bytes.NewReader(c.text)
	cmd.Stdout = s.stdout
	cmd.Stderr = s.stderr
	cmd.Env = append(cmd.Environ(), "CLIPWATCH_BYTES="+strconv.Itoa(len(c.text)))
	// Ctrl-C already reaches the command through the terminal's process
	// group; for a plain SIGTERM to clipwatch, pass it on politely first.
	cmd.Cancel = func() error { return cmd.Process.Signal(syscall.SIGTERM) }
	cmd.WaitDelay = execGrace

	// A failing command is reported but does not stop the watch: the next
	// clip may well succeed.
	if err := cmd.Run(); err != nil && ctx.Err() == nil {
		fmt.Fprintf(s.stderr, "clipwatch: --exec: %v\n", err)
	}
	return nil
}
