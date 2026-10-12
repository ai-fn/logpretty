package cli

import (
	"errors"
	"math"
)

var (
	errSize         = errors.New("want a positive size in bytes, such as 4096, 64K or 1M")
	errSizeTooLarge = errors.New("the size is too large")
)

// parseSize reads a size that fits in an int.
func parseSize(s string) (int, error) {
	n, err := sizeOf(s, math.MaxInt)
	return int(n), err
}

// sizeOf reads a positive number of bytes with an optional unit, at most limit. The units are binary, as
// in the note of a cut line; "KB" is turned down, because it would mean 1000 to some and 1024 to others.
func sizeOf(s string, limit uint64) (uint64, error) {
	var n uint64
	digits, overflow := 0, false
	for ; digits < len(s) && '0' <= s[digits] && s[digits] <= '9'; digits++ {
		d := uint64(s[digits] - '0')
		if n > (math.MaxUint64-d)/10 {
			overflow = true
			continue
		}
		n = n*10 + d
	}
	shift, ok := unitShift(s[digits:])
	switch {
	case digits == 0 || !ok:
		return 0, errSize
	case overflow || n > limit>>shift:
		return 0, errSizeTooLarge
	case n == 0:
		return 0, errSize
	}
	return n << shift, nil
}

// unitShift returns the power of two a unit stands for. Letter case is ignored for ASCII only: Unicode
// folding would take the Kelvin sign for a K.
func unitShift(unit string) (shift uint, ok bool) {
	var buf [3]byte
	if len(unit) > len(buf) {
		return 0, false
	}
	for i := range len(unit) {
		c := unit[i]
		if 'A' <= c && c <= 'Z' {
			c += 'a' - 'A'
		}
		buf[i] = c
	}
	switch string(buf[:len(unit)]) {
	case "":
		return 0, true
	case "k", "kib":
		return 10, true
	case "m", "mib":
		return 20, true
	case "g", "gib":
		return 30, true
	}
	return 0, false
}
