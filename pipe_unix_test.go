//go:build unix

package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestReaderGone(t *testing.T) {
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer w.Close()

	gone := readerGone(w)
	if gone == nil {
		t.Fatal("readerGone returned nil for a pipe")
	}
	if gone() {
		t.Fatal("reported gone while the reader is open")
	}
	if _, err := w.Write([]byte("unread data does not count\n")); err != nil {
		t.Fatal(err)
	}
	if gone() {
		t.Fatal("reported gone with unread data in the pipe")
	}
	r.Close()
	if !gone() {
		t.Fatal("not reported gone after the reader closed")
	}
}

func TestReaderGoneIgnoresFiles(t *testing.T) {
	f, err := os.Create(filepath.Join(t.TempDir(), "out"))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if readerGone(f) != nil {
		t.Error("readerGone should not watch a regular file")
	}
}
