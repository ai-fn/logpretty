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
	}
}

// end closes a part that style opened; a part without a style was never opened.
func (t *Theme) end(dst []byte, style string) []byte {
	if style == "" {
		return dst
	}
	return append(dst, t.Reset...)
}
