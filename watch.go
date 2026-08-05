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
}

func newWatcher(cfg config, b Backend, out, errOut io.Writer) *watcher {
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
		sink:   &writerSink{w: out, framing: cfg.framing, timestamp: cfg.timestamp},
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

// emit passes one clipboard change through the filters and the dedupe
// window, and on to the sink.
func (w *watcher) emit(data []byte) error {
	text, ok := w.filters.apply(data)
	if !ok || w.recent.seen(text) {
		return nil
	}
	return w.sink.emit(clip{time: w.now(), text: text})
}
