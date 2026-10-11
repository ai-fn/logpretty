package entry

import "testing"

func TestParseLevel(t *testing.T) {
	tests := []struct {
		in   string
		want Level
	}{
		{in: "trace", want: LevelTrace},
		{in: "trc", want: LevelTrace},
		{in: "trce", want: LevelTrace},
		{in: "Verbose", want: LevelTrace},
		{in: "vrb", want: LevelTrace},

		{in: "debug", want: LevelDebug},
		{in: "DEBUG", want: LevelDebug},
		{in: "dbg", want: LevelDebug},
		{in: "dbug", want: LevelDebug},

		{in: "info", want: LevelInfo},
		{in: "INFO", want: LevelInfo},
		{in: "inf", want: LevelInfo},
		{in: "Information", want: LevelInfo},
		{in: "notice", want: LevelInfo},

		{in: "warn", want: LevelWarn},
		{in: "WARN", want: LevelWarn},
		{in: "wrn", want: LevelWarn},
		{in: "Warning", want: LevelWarn},

		{in: "error", want: LevelError},
		{in: "ERROR", want: LevelError},
		{in: "err", want: LevelError},
		{in: "eror", want: LevelError},
		{in: "fail", want: LevelError},

		{in: "fatal", want: LevelFatal},
		{in: "ftl", want: LevelFatal},
		{in: "panic", want: LevelFatal},
		{in: "pnc", want: LevelFatal},
		{in: "dpanic", want: LevelFatal},
		{in: "critical", want: LevelFatal},
		{in: "crit", want: LevelFatal},
		{in: "alert", want: LevelFatal},
		{in: "emerg", want: LevelFatal},
		{in: "EMERGENCY", want: LevelFatal},

		// slog prints a custom level as an offset from a named one
		{in: "INFO+2", want: LevelInfo},
		{in: "INFO+0", want: LevelInfo},
		{in: "INFO-1", want: LevelInfo},
		{in: "WARN-9", want: LevelWarn},
		{in: "WARN+2", want: LevelWarn},
		{in: "WARN+4", want: LevelWarn},
		{in: "warn-1", want: LevelWarn},
		{in: "DEBUG+2", want: LevelDebug},
		{in: "DEBUG-0", want: LevelDebug},
		{in: "DEBUG-4", want: LevelTrace},
		{in: "DEBUG-10", want: LevelTrace},
		{in: "ERROR+0", want: LevelError},
		{in: "ERROR-1", want: LevelError},
		{in: "ERROR+4", want: LevelFatal},
		{in: "ERROR+12", want: LevelFatal},
		{in: "FATAL+1", want: LevelFatal},
		{in: "+2", want: LevelUnknown},
		{in: "+", want: LevelUnknown},
		{in: "-", want: LevelUnknown},
		{in: "--1", want: LevelUnknown},
		{in: "1-1", want: LevelUnknown},
		{in: "a-1", want: LevelUnknown},
		{in: "INFO+", want: LevelUnknown},
		{in: "INFO+x", want: LevelUnknown},
		{in: "INFO+/", want: LevelUnknown},
		{in: "INFO+:", want: LevelUnknown},
		{in: "INFO-", want: LevelUnknown},

		{in: "  warn\t", want: LevelWarn},
		{in: "wArNiNg", want: LevelWarn},

		{in: "", want: LevelUnknown},
		{in: "   ", want: LevelUnknown},
		{in: "notaword", want: LevelUnknown},
		{in: "informationx", want: LevelUnknown}, // one byte longer than the longest name
		{in: "information-overload-is-not-a-level", want: LevelUnknown},
		{in: "warn error", want: LevelUnknown},
		{in: "30", want: LevelUnknown},
		{in: "inf\xff", want: LevelUnknown},
		{in: "ИНФО", want: LevelUnknown},
	}

	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			if got := ParseLevel([]byte(tt.in)); got != tt.want {
				t.Errorf("ParseLevel(%q) = %v, want %v", tt.in, got, tt.want)
			}
		})
	}
}

func TestParseLevelDoesNotAllocate(t *testing.T) {
	for _, in := range []string{"info", "Warning", "ERROR+4", "DEBUG-4", "  debug ", "notaword", "a-very-long-word-that-is-not-a-level"} {
		text := []byte(in)
		if allocs := testing.AllocsPerRun(100, func() { _ = ParseLevel(text) }); allocs != 0 {
			t.Errorf("ParseLevel(%q) allocates %v times per call, want 0", in, allocs)
		}
	}
}

// A minimum-level filter compares levels, so their order is part of the contract.
func TestLevelOrder(t *testing.T) {
	order := []Level{LevelUnknown, LevelTrace, LevelDebug, LevelInfo, LevelWarn, LevelError, LevelFatal}
	for i := 1; i < len(order); i++ {
		if order[i-1] >= order[i] {
			t.Errorf("%v must be below %v", order[i-1], order[i])
		}
	}
}

func TestLevelString(t *testing.T) {
	tests := map[Level]string{
		LevelUnknown: "unknown",
		LevelTrace:   "trace",
		LevelDebug:   "debug",
		LevelInfo:    "info",
		LevelWarn:    "warn",
		LevelError:   "error",
		LevelFatal:   "fatal",
		Level(100):   "unknown",
		Level(-1):    "unknown",
	}
	for level, want := range tests {
		if got := level.String(); got != want {
			t.Errorf("Level(%d).String() = %q, want %q", int8(level), got, want)
		}
	}
}

// Every name a level prints must parse back to that level.
func TestLevelStringRoundTrips(t *testing.T) {
	for level := LevelTrace; level <= LevelFatal; level++ {
		if got := ParseLevel([]byte(level.String())); got != level {
			t.Errorf("ParseLevel(%q) = %v, want %v", level.String(), got, level)
		}
	}
}
