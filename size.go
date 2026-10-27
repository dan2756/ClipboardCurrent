package main

import (
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"
)

var sizeRE = regexp.MustCompile(`(?i)^\s*(\d+(?:\.\d+)?)\s*(?:([kmg])(?:i?b)?|b)?\s*$`)

// parseSize parses a byte size such as 4096, 512B, 64K, 64KB, 1.5MB or
// 2GiB. Units are powers of 1024 whether or not they are spelled with an i.
func parseSize(s string) (int64, error) {
	m := sizeRE.FindStringSubmatch(s)
	if m == nil {
		return 0, fmt.Errorf("invalid size %q (want e.g. 4096, 64KB or 1MB)", s)
	}
	n, err := strconv.ParseFloat(m[1], 64)
	if err != nil {
		return 0, fmt.Errorf("invalid size %q: %w", s, err)
	}
	switch strings.ToLower(m[2]) {
	case "k":
		n *= 1 << 10
	case "m":
		n *= 1 << 20
	case "g":
		n *= 1 << 30
	}
	// float64(MaxInt64) rounds to 2^63, so this rejects values that do not
	// fit in an int64. A limit of 0 means "no limit" (the flag's zero
	// value), so anything that is not a positive whole number of bytes is
	// an error: truncating 0.5 used to become 0 and silently disable
	// --max-bytes, and truncating 1.9 used to become 1.
	if math.IsNaN(n) || math.IsInf(n, 0) || n >= float64(math.MaxInt64) {
		return 0, fmt.Errorf("size %q is too large", s)
	}
	if n > 0 && math.Trunc(n) != n {
		return 0, fmt.Errorf("size %q is not a whole number of bytes", s)
	}
	if n < 1 {
		return 0, fmt.Errorf("size %q must be at least 1 byte", s)
	}
	return int64(n), nil
}

// sizeValue is a flag.Value for byte sizes.
type sizeValue int64

func (v *sizeValue) String() string { return strconv.FormatInt(int64(*v), 10) }

func (v *sizeValue) Set(s string) error {
	n, err := parseSize(s)
	if err != nil {
		return err
	}
	*v = sizeValue(n)
	return nil
}
