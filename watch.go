package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"syscall"
	"time"
)

// readTimeout bounds a single clipboard read. Paste tools wait for the app
// that owns the clipboard to hand it over, and block for as long as that app
// is frozen; a few seconds is far longer than a healthy read takes.
const readTimeout = 5 * time.Second

// watcher polls a Backend and hands each new clip to a sink.
type watcher struct {
	backend        Backend
	interval       time.Duration
	readTimeout    time.Duration
	includeCurrent bool
	filters        filters
	recent         *history
	sink           sink
	errOut         io.Writer
	now            func() time.Time

	// outputGone, if set, reports that nobody is reading stdout any more.
	outputGone func() bool

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
		readTimeout:    readTimeout,
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

// run polls until ctx is cancelled or the output is closed. The first read
// only records what is on the clipboard (and emits it with includeCurrent);
// every later change is emitted once.
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
		if w.outputGone != nil && w.outputGone() {
			return nil
		}
		data, err := w.read(ctx)
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
					return stopError(ctx, err)
				}
			}
		case !bytes.Equal(data, last):
			last = data
			if err := w.emit(ctx, data); err != nil {
				return stopError(ctx, err)
			}
		}

		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
		}
	}
}

// read reads the clipboard once, giving up after readTimeout so that one
// hung paste command cannot stall the watch for good.
func (w *watcher) read(ctx context.Context) ([]byte, error) {
	rctx, cancel := context.WithTimeout(ctx, w.readTimeout)
	defer cancel()
	data, err := w.backend.Read(rctx)
	if err != nil && ctx.Err() == nil && errors.Is(err, context.DeadlineExceeded) {
		return nil, fmt.Errorf("%s: no answer after %v", w.backend.Name(), w.readTimeout)
	}
	return data, err
}

// stopError filters an error that ended the watch. A broken pipe is how a
// pipeline normally tells its writer to stop ("clipwatch | head -3"), so
// it is not an error, and neither is a write cut short by ctx ending.
func stopError(ctx context.Context, err error) error {
	if errors.Is(err, syscall.EPIPE) || ctx.Err() != nil {
		return nil
	}
	return err
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
