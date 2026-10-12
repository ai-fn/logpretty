package render

import (
	"math"
	"strconv"
	"testing"
)

func TestAppendSize(t *testing.T) {
	// int64, so that the sizes an int of 32 bits cannot hold still compile there; they are skipped below.
	tests := []struct {
		n    int64
		want string
	}{
		{-1, "0B"},
		{0, "0B"},
		{1, "1B"},
		{1023, "1023B"},
		{1024, "1.0KiB"},
		{1075, "1.0KiB"}, // 1.0498
		{1076, "1.1KiB"}, // 1.0508
		{1280, "1.3KiB"}, // 1.25 exactly: half goes up
		{1536, "1.5KiB"},
		{2047, "2.0KiB"},
		{1024*1024 - 52, "1023.9KiB"},
		{1024*1024 - 51, "1.0MiB"}, // 1023.95 KiB rounds to 1024.0, which is the next unit
		{1024 * 1024, "1.0MiB"},
		{2202009, "2.1MiB"},
		{1073689395, "1023.9MiB"},
		{1073689396, "1.0GiB"},
		{1 << 30, "1.0GiB"},
		{1<<31 - 1, "2.0GiB"},
		{1 << 40, "1.0TiB"},
		{1 << 50, "1.0PiB"},
		{1 << 60, "1.0EiB"},
		{math.MaxInt64, "8.0EiB"},
	}

	for _, tt := range tests {
		if int64(int(tt.n)) != tt.n {
			continue
		}
		if got := string(appendSize(nil, int(tt.n))); got != tt.want {
			t.Errorf("appendSize(%d) = %q, want %q", tt.n, got, tt.want)
		}
	}

	if got := string(appendSize([]byte("+"), 5)); got != "+5B" {
		t.Errorf("appendSize after a plus sign = %q, want %q", got, "+5B")
	}
}

// Every size up to a few units, against arithmetic in floating point.
func TestAppendSizeAgainstFloat(t *testing.T) {
	for n := 1024; n < 4<<20; n += 7 {
		unit, name := 1024.0, "KiB"
		// Half up to one decimal; what reaches 1024.0 is written in the next unit.
		if math.Floor(float64(n)/unit*10+0.5) >= 10240 {
			unit, name = 1024.0*1024.0, "MiB"
		}
		want := strconv.FormatFloat(math.Floor(float64(n)/unit*10+0.5)/10, 'f', 1, 64) + name
		if got := string(appendSize(nil, n)); got != want {
			t.Fatalf("appendSize(%d) = %q, want %q", n, got, want)
		}
	}
}
