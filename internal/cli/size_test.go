package cli

import (
	"math"
	"strings"
	"testing"
)

func TestSizeOf(t *testing.T) {
	const (
		syntax   = "want a positive size in bytes"
		tooLarge = "is too large"
	)

	tests := []struct {
		name    string
		in      string
		limit   uint64
		want    uint64
		wantErr string
	}{
		{name: "bytes", in: "4096", want: 4096},
		{name: "one byte", in: "1", want: 1},
		{name: "leading zeros", in: "007", want: 7},
		{name: "K", in: "64K", want: 64 << 10},
		{name: "KiB", in: "64KiB", want: 64 << 10},
		{name: "M", in: "1M", want: 1 << 20},
		{name: "MiB", in: "1MiB", want: 1 << 20},
		{name: "G", in: "1G", want: 1 << 30},
		{name: "GiB", in: "2GiB", want: 2 << 30},
		{name: "lower case", in: "64k", want: 64 << 10},
		{name: "lower case with the unit spelled out", in: "3mib", want: 3 << 20},
		{name: "upper case", in: "1KIB", want: 1 << 10},
		{name: "mixed case", in: "2gIb", want: 2 << 30},
		{name: "a unit is 1024 of the one below", in: "1024K", want: 1 << 20},
		{name: "the limit itself", in: "2147483647", limit: math.MaxInt32, want: math.MaxInt32},
		{name: "the largest number of 64 bits", in: "18446744073709551615", want: math.MaxUint64},
		{name: "a product that is the limit", in: "16777216K", limit: 1 << 34, want: 1 << 34},

		{name: "empty", in: "", wantErr: syntax},
		{name: "zero", in: "0", wantErr: syntax},
		{name: "zero with a unit", in: "0K", wantErr: syntax},
		{name: "negative", in: "-1", wantErr: syntax},
		{name: "plus sign", in: "+1", wantErr: syntax},
		{name: "unit without a number", in: "K", wantErr: syntax},
		{name: "not a number", in: "abc", wantErr: syntax},
		{name: "decimal unit", in: "1KB", wantErr: syntax},
		{name: "bytes spelled out", in: "1B", wantErr: syntax},
		{name: "unknown unit", in: "1T", wantErr: syntax},
		{name: "fraction", in: "1.5M", wantErr: syntax},
		{name: "space before the unit", in: "1 M", wantErr: syntax},
		{name: "space around", in: " 1M", wantErr: syntax},
		{name: "unit twice", in: "1KK", wantErr: syntax},
		{name: "digits after the unit", in: "1K5", wantErr: syntax},
		{name: "exponent", in: "1e3", wantErr: syntax},
		{name: "hexadecimal", in: "0x10", wantErr: syntax},
		{name: "underscore", in: "1_000", wantErr: syntax},
		{name: "unit longer than any", in: "1KiBs", wantErr: syntax},
		{name: "the Kelvin sign is not a K", in: "1\u212a", wantErr: syntax},
		{name: "digits that are not ASCII", in: "١٢", wantErr: syntax},

		{name: "one over the limit", in: "2147483648", limit: math.MaxInt32, wantErr: tooLarge},
		{name: "a product over the limit", in: "2G", limit: math.MaxInt32, wantErr: tooLarge},
		{name: "a product one over the limit", in: "16777217K", limit: 1<<34 - 1, wantErr: tooLarge},
		{name: "one over 64 bits", in: "18446744073709551616", wantErr: tooLarge},
		{name: "a product over 64 bits", in: "18446744073709551615G", wantErr: tooLarge},
		{name: "digits without end", in: strings.Repeat("9", 100), wantErr: tooLarge},
		{name: "digits without end and a unit", in: strings.Repeat("9", 100) + "K", wantErr: tooLarge},
		// What is not a size at all is told so, whatever its number; and a zero is not large.
		{name: "digits without end and a unit that is not one", in: strings.Repeat("9", 100) + "KB", wantErr: syntax},
		{name: "zeros without end", in: strings.Repeat("0", 100), wantErr: syntax},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			limit := tt.limit
			if limit == 0 {
				limit = math.MaxUint64
			}

			got, err := sizeOf(tt.in, limit)

			if tt.wantErr == "" {
				if err != nil || got != tt.want {
					t.Errorf("sizeOf(%q) = %d, %v; want %d", tt.in, got, err, tt.want)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) || got != 0 {
				t.Errorf("sizeOf(%q) = %d, %v; want 0 and an error with %q", tt.in, got, err, tt.wantErr)
			}
		})
	}
}

// The limit of parseSize is the largest int, whatever its width on the platform.
func TestParseSize(t *testing.T) {
	if got, err := parseSize("1M"); err != nil || got != 1<<20 {
		t.Errorf(`parseSize("1M") = %d, %v; want %d`, got, err, 1<<20)
	}
	if _, err := parseSize("18446744073709551615"); err == nil || !strings.Contains(err.Error(), "is too large") {
		t.Errorf("parseSize of a number larger than any int: error = %v, want too large", err)
	}
}
