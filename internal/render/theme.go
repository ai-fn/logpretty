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

// end closes a part that style opened; a part without a style was never opened.
func (t *Theme) end(dst []byte, style string) []byte {
	if style == "" {
		return dst
	}
	return append(dst, t.Reset...)
}
