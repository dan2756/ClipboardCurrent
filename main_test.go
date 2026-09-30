package main

import (
	"bufio"
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// setClip replaces the fake clipboard file atomically, so a concurrent
// "cat" never sees it half written.
func setClip(t *testing.T, path, text string) {
	t.Helper()
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(tmp, path); err != nil {
		t.Fatal(err)
	}
}

// TestRunPipeline drives the whole CLI the way "clipwatch | head -2" would:
// a real paste command, a real pipe on stdout, and a reader that leaves.
func TestRunPipeline(t *testing.T) {
	clipFile := filepath.Join(t.TempDir(), "clipboard")
	setClip(t, clipFile, "https://go.dev")

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { r.Close(); w.Close() })

	var stderr bytes.Buffer
	done := make(chan int, 1)
	go func() {
		args := []string{"--paste-cmd", "cat '" + clipFile + "'", "-i", "10ms", "--include-current", "--filter", "^https://"}
		done <- run(args, w, &stderr)
	}()

	lines := bufio.NewReader(r)
	readLine := func() string {
		t.Helper()
		line, err := lines.ReadString('\n')
		if err != nil {
			t.Fatalf("reading clipwatch output: %v", err)
		}
		return line
	}

	if got := readLine(); got != "https://go.dev\n" {
		t.Fatalf("first line = %q", got)
	}
	setClip(t, clipFile, "not a link")
	setClip(t, clipFile, "https://pkg.go.dev")
	if got := readLine(); got != "https://pkg.go.dev\n" && got != "not a link\n" {
		t.Fatalf("second line = %q", got)
	} else if got == "not a link\n" {
		t.Fatal("--filter let a non-matching clip through")
	}

	r.Close() // the reader goes away, as head does
	select {
	case code := <-done:
		if code != exitOK {
			t.Errorf("exit %d after the reader closed, want 0; stderr: %s", code, stderr.String())
		}
	case <-time.After(5 * time.Second):
		t.Fatal("clipwatch kept running after its reader closed")
	}
}

func TestRunPasteCmdFailingAtStartup(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run([]string{"--paste-cmd", "echo 'clipboard service not running' >&2; exit 1"}, &stdout, &stderr)
	if code != exitError {
		t.Errorf("exit %d, want %d", code, exitError)
	}
	if !strings.Contains(stderr.String(), "paste-cmd: clipboard service not running") {
		t.Errorf("stderr = %q", stderr.String())
	}
}
