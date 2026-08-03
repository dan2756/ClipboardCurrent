package main

import (
	"bytes"
	"context"
	"errors"
	"strings"
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
}

func (f *fakeBackend) Name() string { return "fake" }

func (f *fakeBackend) Read(ctx context.Context) ([]byte, error) {
	if f.reads >= len(f.states) {
		f.cancel()
		return nil, ctx.Err()
	}
	s := f.states[f.reads]
	f.reads++
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
	return watchResult{stdout: stdout.String(), stderr: stderr.String(), err: err}
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
