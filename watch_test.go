package main

import (
	"bytes"
	"context"
	"errors"
	"os"
	"strings"
	"syscall"
	"testing"
	"time"
)

// fakeBackend replays a scripted sequence of clipboard states, one per
// poll, and cancels the watch once the script runs out.
type fakeBackend struct {
	states []fakeState
	reads  int
	cancel context.CancelFunc
}

type fakeState struct {
	text string
	err  error
	hang bool // block until the read is cancelled, like a stuck wl-paste
}

func (f *fakeBackend) Name() string { return "fake" }

func (f *fakeBackend) Read(ctx context.Context) ([]byte, error) {
	if f.reads >= len(f.states) {
		f.cancel()
		return nil, ctx.Err()
	}
	s := f.states[f.reads]
	f.reads++
	if s.hang {
		<-ctx.Done()
		return nil, ctx.Err()
	}
	if s.err != nil {
		return nil, s.err
	}
	return []byte(s.text), nil
}

// states turns clipboard texts into a script, one poll each.
func states(texts ...string) []fakeState {
	out := make([]fakeState, len(texts))
	for i, t := range texts {
		out[i] = fakeState{text: t}
	}
	return out
}

type watchResult struct {
	stdout, stderr string
	err            error
	reads          int // states consumed from the script
}

// watch runs a watcher over script with a 1ms interval and returns what it
// printed. The script fully determines the result: each poll consumes one
// state, however fast or slow the ticker is. opts can adjust the watcher
// before it starts.
func watch(t *testing.T, cfg config, script []fakeState, opts ...func(*watcher)) watchResult {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if cfg.interval == 0 {
		cfg.interval = time.Millisecond
	}
	fb := &fakeBackend{states: script, cancel: cancel}
	var stdout, stderr bytes.Buffer
	w := newWatcher(cfg, fb, &stdout, &stderr)
	for _, opt := range opts {
		opt(w)
	}
	err := w.run(ctx)
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		t.Fatalf("watch did not finish: %d of %d states read", fb.reads, len(script))
	}
	return watchResult{stdout: stdout.String(), stderr: stderr.String(), err: err, reads: fb.reads}
}

func TestPrintsEachChangeOnce(t *testing.T) {
	r := watch(t, config{}, states("start", "start", "one", "one", "one", "two", "three", "three"))
	if r.err != nil {
		t.Fatal(r.err)
	}
	if want := "one\ntwo\nthree\n"; r.stdout != want {
		t.Errorf("stdout = %q, want %q", r.stdout, want)
	}
}

func TestSkipsCurrentClipboardByDefault(t *testing.T) {
	r := watch(t, config{}, states("already there", "already there"))
	if r.stdout != "" {
		t.Errorf("stdout = %q, want nothing", r.stdout)
	}
}

func TestIncludeCurrent(t *testing.T) {
	r := watch(t, config{includeCurrent: true}, states("already there", "already there", "new"))
	if want := "already there\nnew\n"; r.stdout != want {
		t.Errorf("stdout = %q, want %q", r.stdout, want)
	}
}

func TestEmptyClipboardIsNotPrinted(t *testing.T) {
	r := watch(t, config{includeCurrent: true}, states("", "a", "", "b"))
	if want := "a\nb\n"; r.stdout != want {
		t.Errorf("stdout = %q, want %q", r.stdout, want)
	}
}

func TestKeepsExistingTrailingNewline(t *testing.T) {
	r := watch(t, config{}, states("", "line\n", "multi\nline"))
	if want := "line\nmulti\nline\n"; r.stdout != want {
		t.Errorf("stdout = %q, want %q", r.stdout, want)
	}
}

func TestFirstReadErrorIsFatal(t *testing.T) {
	boom := errors.New("wl-paste: failed to connect to a Wayland server")
	r := watch(t, config{}, []fakeState{{err: boom}, {text: "never read"}})
	if !errors.Is(r.err, boom) {
		t.Fatalf("err = %v, want %v", r.err, boom)
	}
	if r.stdout != "" {
		t.Errorf("stdout = %q, want nothing", r.stdout)
	}
}

func TestLaterReadErrorsAreReported(t *testing.T) {
	script := []fakeState{{text: "a"}, {err: errors.New("xclip: lost the display")}, {text: "a"}, {text: "b"}}
	r := watch(t, config{}, script)
	if r.err != nil {
		t.Fatal(r.err)
	}
	if want := "b\n"; r.stdout != want {
		t.Errorf("stdout = %q, want %q", r.stdout, want)
	}
	if !strings.Contains(r.stderr, "lost the display") {
		t.Errorf("stderr = %q, want the read error", r.stderr)
	}
}

func TestRepeatedReadErrorsAreReportedOnce(t *testing.T) {
	gone := errors.New("xclip: Error: Can't open display: :0")
	other := errors.New("xclip: something else")
	script := []fakeState{{text: "a"}, {err: gone}, {err: gone}, {err: gone}, {err: other}, {err: other}, {text: "a"}, {err: gone}}
	r := watch(t, config{}, script)
	if r.err != nil {
		t.Fatal(r.err)
	}
	want := "clipwatch: " + gone.Error() + "\nclipwatch: " + other.Error() + "\nclipwatch: " + gone.Error() + "\n"
	if r.stderr != want {
		t.Errorf("stderr =\n%s\nwant\n%s", r.stderr, want)
	}
}

func TestEmptyClipboardAtStartup(t *testing.T) {
	// Backends report an empty clipboard as no data; that must not count
	// as a failed first read.
	r := watch(t, config{}, []fakeState{{text: ""}, {text: ""}, {text: "first copy"}})
	if r.err != nil {
		t.Fatal(r.err)
	}
	if want := "first copy\n"; r.stdout != want {
		t.Errorf("stdout = %q, want %q", r.stdout, want)
	}
}

func TestStopsWhenOutputGone(t *testing.T) {
	checks := 0
	gone := func(w *watcher) {
		w.outputGone = func() bool { checks++; return checks > 3 }
	}
	r := watch(t, config{}, states("", "a", "b", "c", "d"), gone)
	if r.err != nil {
		t.Fatalf("a closed reader should end the watch quietly: %v", r.err)
	}
	if want := "a\nb\n"; r.stdout != want {
		t.Errorf("stdout = %q, want %q", r.stdout, want)
	}
	if r.reads != 3 {
		t.Errorf("kept reading after the output closed: %d reads, want 3", r.reads)
	}
}

// brokenPipe fails every write the way os.Stdout does once its reader exits.
type brokenPipe struct{}

func (brokenPipe) Write([]byte) (int, error) {
	return 0, &os.PathError{Op: "write", Path: "/dev/stdout", Err: syscall.EPIPE}
}

type failingWriter struct{ err error }

func (f failingWriter) Write([]byte) (int, error) { return 0, f.err }

func TestBrokenPipeEndsWatchQuietly(t *testing.T) {
	toBrokenPipe := func(w *watcher) { w.sink = &writerSink{w: brokenPipe{}} }
	r := watch(t, config{}, states("", "a", "b", "c"), toBrokenPipe)
	if r.err != nil {
		t.Fatalf("EPIPE should end the watch without an error: %v", r.err)
	}
	if r.reads != 2 {
		t.Errorf("kept reading after EPIPE: %d reads, want 2", r.reads)
	}
	if r.stderr != "" {
		t.Errorf("unexpected stderr: %q", r.stderr)
	}
}

func TestOtherWriteErrorsAreFatal(t *testing.T) {
	full := &os.PathError{Op: "write", Path: "/mnt/usb/links.txt", Err: syscall.ENOSPC}
	toFullDisk := func(w *watcher) { w.sink = &writerSink{w: failingWriter{full}} }
	r := watch(t, config{}, states("", "a", "b"), toFullDisk)
	if !errors.Is(r.err, syscall.ENOSPC) {
		t.Fatalf("err = %v, want ENOSPC", r.err)
	}
}

func TestHungReadTimesOut(t *testing.T) {
	quickTimeout := func(w *watcher) { w.readTimeout = 20 * time.Millisecond }
	script := []fakeState{{text: ""}, {text: "a"}, {hang: true}, {hang: true}, {text: "b"}}
	r := watch(t, config{}, script, quickTimeout)
	if r.err != nil {
		t.Fatal(r.err)
	}
	if want := "a\nb\n"; r.stdout != want {
		t.Errorf("stdout = %q, want %q", r.stdout, want)
	}
	if want := "clipwatch: fake: no answer after 20ms\n"; r.stderr != want {
		t.Errorf("stderr = %q, want %q", r.stderr, want)
	}
}

func TestHungFirstReadIsFatal(t *testing.T) {
	quickTimeout := func(w *watcher) { w.readTimeout = 20 * time.Millisecond }
	r := watch(t, config{}, []fakeState{{hang: true}, {text: "a"}}, quickTimeout)
	if r.err == nil || !strings.Contains(r.err.Error(), "no answer after 20ms") {
		t.Errorf("err = %v, want a timeout", r.err)
	}
}

// stalledWriter blocks every write until release is closed, like a pipe
// whose reader has stopped reading; the first write also stops the watch,
// as Ctrl-C or SIGTERM would.
type stalledWriter struct {
	stop    context.CancelFunc
	release chan struct{}
}

func (s stalledWriter) Write(p []byte) (int, error) {
	s.stop()
	<-s.release
	return len(p), nil
}

func TestStopDuringBlockedWrite(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	release := make(chan struct{})
	defer close(release)

	fb := &fakeBackend{states: states("a", "a"), cancel: cancel}
	cfg := config{interval: time.Millisecond, includeCurrent: true}
	var stderr bytes.Buffer
	w := newWatcher(cfg, fb, stalledWriter{stop: cancel, release: release}, &stderr)

	done := make(chan error, 1)
	go func() { done <- w.run(ctx) }()
	select {
	case err := <-done:
		if err != nil {
			t.Errorf("stopping during a write should end the watch quietly: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("the watch could not be stopped while a write was blocked")
	}
}
