package render

import "github.com/ai-fn/logpretty/internal/entry"

// Theme holds the escape sequence that starts each styled part of a line and the one that ends any of them.
// The zero Theme prints no color.
type Theme struct {
	Time   string
	Level  [entry.LevelFatal + 1]string // by entry.Level
	Key    string                       // the key of a field and its equals sign
	Prefix string                       // what a log collector put in front of the line
	Marker string                       // the note that a line was cut
	Reset  string
	Traces []string // the colors of trace ids, one picked by the id; none leaves them uncolored
}

// DefaultTheme uses the 16 basic colors and faint: they follow the palette of the terminal, and a terminal
// without faint shows that text as usual.
func DefaultTheme() Theme {
	const faint = "\x1b[2m"
	return Theme{
		Time: faint,
		Level: [...]string{
			entry.LevelTrace: faint,
			entry.LevelDebug: "\x1b[36m",
			entry.LevelInfo:  "\x1b[32m",
			entry.LevelWarn:  "\x1b[33m",
			entry.LevelError: "\x1b[31m",
			entry.LevelFatal: "\x1b[1;31m",
		},
		Key:    faint,
		Prefix: faint,
		Marker: "\x1b[33m",
		Reset:  "\x1b[0m",
		// 12 of the 256 colors, apart from the levels' and readable on light and dark: the basic ones left are too few.
		Traces: []string{
			"\x1b[38;5;168m", "\x1b[38;5;130m", "\x1b[38;5;31m", "\x1b[38;5;33m", "\x1b[38;5;27m", "\x1b[38;5;104m",
			"\x1b[38;5;99m", "\x1b[38;5;171m", "\x1b[38;5;128m", "\x1b[38;5;133m", "\x1b[38;5;201m", "\x1b[38;5;162m",
		},
	}
}

// BasicTracePalette colors trace ids where the 256 colors become the basic 16, as on the Linux console: blue and
// magenta, the hues the levels leave.
func BasicTracePalette() []string {
	return []string{"\x1b[34m", "\x1b[35m", "\x1b[94m", "\x1b[95m"}
}

// traceStyle picks the color of a trace id by FNV-1a of its text, so an id keeps its color across lines and runs.
func (t *Theme) traceStyle(id []byte) string {
	if len(t.Traces) == 0 {
		return ""
	}
	// Not hash/fnv: its hash is an interface value, which allocates.
	h := uint32(2166136261)
	for _, b := range id {
		h = (h ^ uint32(b)) * 16777619
	}
	return t.Traces[h%uint32(len(t.Traces))]
}

// end closes a part that style opened; a part without a style was never opened.
func (t *Theme) end(dst []byte, style string) []byte {
	if style == "" {
		return dst
	}
	return append(dst, t.Reset...)
}
