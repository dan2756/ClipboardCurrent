package main

import "testing"

func TestParseSize(t *testing.T) {
	tests := []struct {
		in   string
		want int64
	}{
		{"0", 0},
		{"4096", 4096},
		{"512B", 512},
		{"64k", 64 << 10},
		{"64KB", 64 << 10},
		{"64KiB", 64 << 10},
		{"1MB", 1 << 20},
		{"1 MiB", 1 << 20},
		{"1.5M", 3 << 19},
		{"2gb", 2 << 30},
	}
	for _, tt := range tests {
		got, err := parseSize(tt.in)
		if err != nil {
			t.Errorf("parseSize(%q): %v", tt.in, err)
			continue
		}
		if got != tt.want {
			t.Errorf("parseSize(%q) = %d, want %d", tt.in, got, tt.want)
		}
	}
}

func TestParseSizeErrors(t *testing.T) {
	for _, in := range []string{"", "MB", "-1", "1TB", "1.2.3", "ten", "1e3", "99999999999G"} {
		if n, err := parseSize(in); err == nil {
			t.Errorf("parseSize(%q) = %d, want an error", in, n)
		}
	}
}
