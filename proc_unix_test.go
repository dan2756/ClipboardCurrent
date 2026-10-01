//go:build unix

package main

import (
	"context"
	"errors"
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
