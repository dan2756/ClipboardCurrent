//go:build unix

package main

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestPasteCmdKilledWithItsChildren(t *testing.T) {
	ticks := filepath.Join(t.TempDir(), "ticks")
	// The inner sh stands in for the paste tool that sh -c starts as its
	// child; the trailing ":" keeps the outer shell from exec'ing it. It
	// appends to ticks for a few seconds unless it is killed.
	loop := `i=0; while [ $i -lt 300 ]; do echo >> "$0"; sleep 0.01; i=$((i+1)); done`
	b := newShellBackend(`sh -c '` + loop + `' '` + ticks + `'; :`)

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	if _, err := b.Read(ctx); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("err = %v, want DeadlineExceeded", err)
	}

	size := func() int64 {
		fi, err := os.Stat(ticks)
		if errors.Is(err, os.ErrNotExist) {
			return 0
		}
		if err != nil {
			t.Fatal(err)
		}
		return fi.Size()
	}
	before := size()
	time.Sleep(200 * time.Millisecond)
	if after := size(); after != before {
		t.Errorf("the paste command's child kept running after the read was cancelled (%d -> %d bytes written)", before, after)
	}
}

func TestExecCmdKilledWithItsChildren(t *testing.T) {
	ticks := filepath.Join(t.TempDir(), "ticks")
	// Same shape as the paste-cmd case: the inner sh is the program
	// --exec's shell started, and it must die when the watch is stopped.
	loop := `i=0; while [ $i -lt 300 ]; do echo >> "$0"; sleep 0.01; i=$((i+1)); done`
	command := `sh -c '` + loop + `' '` + ticks + `'; :`

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var stderr bytes.Buffer
	s := &execSink{command: command, stdout: io.Discard, stderr: &stderr}

	done := make(chan struct{})
	go func() {
		_ = s.emit(ctx, clip{text: []byte("x")})
		close(done)
	}()

	deadline := time.Now().Add(2 * time.Second)
	for {
		fi, err := os.Stat(ticks)
		if err == nil && fi.Size() > 0 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("exec command never started")
		}
		time.Sleep(10 * time.Millisecond)
	}
	cancel()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("--exec did not return after the watch was stopped")
	}
	if stderr.Len() != 0 {
		t.Errorf("stopping --exec should be quiet, stderr: %s", stderr.String())
	}

	size := func() int64 {
		fi, err := os.Stat(ticks)
		if err != nil {
			t.Fatal(err)
		}
		return fi.Size()
	}
	before := size()
	time.Sleep(200 * time.Millisecond)
	if after := size(); after != before {
		t.Errorf("--exec child kept running after the watch stopped (%d -> %d bytes written)", before, after)
	}
}
