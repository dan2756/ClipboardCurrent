package main

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"time"
)

// watcher polls a Backend and hands each new clip to a sink.
type watcher struct {
	backend        Backend
	interval       time.Duration
	includeCurrent bool
	filters        filters
	recent         *history
	sink           sink
	errOut         io.Writer
	now            func() time.Time

	lastWarn string // last read error reported, until a read succeeds
}

func newWatcher(cfg config, b Backend, out, errOut io.Writer) *watcher {
	var sk sink = &writerSink{w: out, framing: cfg.framing, timestamp: cfg.timestamp}
	if cfg.exec != "" {
		sk = &execSink{command: cfg.exec, stdout: out, stderr: errOut}
	}
	return &watcher{
		backend:        b,
		interval:       cfg.interval,
		includeCurrent: cfg.includeCurrent,
		filters: filters{
			trim:           cfg.trim,
			skipWhitespace: cfg.ignoreWhitespaceOnly,
			maxBytes:       cfg.maxBytes,
			match:          cfg.filter,
		},
		recent: newHistory(cfg.dedupe),
		sink:   sk,
		errOut: errOut,
		now:    time.Now,
	}
}

// run polls until ctx is cancelled. The first read only records what is on
// the clipboard (and emits it with includeCurrent); every later change is
// emitted once.
//
// A failing first read is returned as an error, since it almost always
// means the backend cannot work at all. Later failures are reported on
// errOut (once per run of identical errors) and polling carries on.
func (w *watcher) run(ctx context.Context) error {
	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()

	var last []byte
	started := false
	for {
		data, err := w.backend.Read(ctx)
		if err == nil {
			w.lastWarn = ""
		}
		switch {
		case ctx.Err() != nil:
			return nil
		case err != nil && !started:
			return err
		case err != nil:
			w.warn(err)
		case !started:
			started = true
			last = data
			if w.includeCurrent {
				if err := w.emit(ctx, data); err != nil {
					return err
				}
			}
		case !bytes.Equal(data, last):
			last = data
			if err := w.emit(ctx, data); err != nil {
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

// warn reports a read error unless it is the same one as last time, so a
// backend that keeps failing does not print a line every interval.
func (w *watcher) warn(err error) {
	msg := err.Error()
	if msg == w.lastWarn {
		return
	}
	w.lastWarn = msg
	fmt.Fprintf(w.errOut, "clipwatch: %s\n", msg)
}

// emit passes one clipboard change through the filters and the dedupe
// window, and on to the sink.
func (w *watcher) emit(ctx context.Context, data []byte) error {
	text, ok := w.filters.apply(data)
	if !ok || w.recent.seen(text) {
		return nil
	}
	return w.sink.emit(ctx, clip{time: w.now(), text: text})
}
