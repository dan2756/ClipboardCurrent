package main

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"time"
)

// watcher polls a Backend and writes each new clip to out.
type watcher struct {
	backend        Backend
	interval       time.Duration
	includeCurrent bool
	out            io.Writer
	errOut         io.Writer
}

func newWatcher(cfg config, b Backend, out, errOut io.Writer) *watcher {
	return &watcher{
		backend:        b,
		interval:       cfg.interval,
		includeCurrent: cfg.includeCurrent,
		out:            out,
		errOut:         errOut,
	}
}

// run polls until ctx is cancelled. The first read only records what is on
// the clipboard (and emits it with includeCurrent); every later change is
// emitted once.
//
// A failing first read is returned as an error, since it almost always
// means the backend cannot work at all. Later failures are reported on
// errOut and polling carries on.
func (w *watcher) run(ctx context.Context) error {
	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()

	var last []byte
	started := false
	for {
		data, err := w.backend.Read(ctx)
		switch {
		case ctx.Err() != nil:
			return nil
		case err != nil && !started:
			return err
		case err != nil:
			fmt.Fprintf(w.errOut, "clipwatch: %v\n", err)
		case !started:
			started = true
			last = data
			if w.includeCurrent {
				if err := w.emit(data); err != nil {
					return err
				}
			}
		case !bytes.Equal(data, last):
			last = data
			if err := w.emit(data); err != nil {
				return err
			}
		}

		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
		}
	}
}

// emit prints one clip followed by a newline, unless it already ends with
// one. Empty clips (a cleared clipboard) are never printed.
func (w *watcher) emit(data []byte) error {
	if len(data) == 0 {
		return nil
	}
	buf := make([]byte, 0, len(data)+1)
	buf = append(buf, data...)
	if data[len(data)-1] != '\n' {
		buf = append(buf, '\n')
	}
	// One Write per clip, straight to the file: a reader on the other end
	// of a pipe sees each clip as soon as it is copied.
	_, err := w.out.Write(buf)
	return err
}
