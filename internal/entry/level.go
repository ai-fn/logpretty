package entry

import "bytes"

// Level is the severity of an entry; a higher value is more severe.
type Level int8

// Levels, from least to most severe. LevelUnknown is below all of them.
const (
	LevelUnknown Level = iota
	LevelTrace
	LevelDebug
	LevelInfo
	LevelWarn
	LevelError
	LevelFatal
)

var levelNames = [...]string{"unknown", "trace", "debug", "info", "warn", "error", "fatal"}

func (l Level) String() string {
	if l < 0 || int(l) >= len(levelNames) {
		return levelNames[LevelUnknown]
	}
	return levelNames[l]
}

// maxLevelName is the length of the longest name ParseLevel knows, "information".
const maxLevelName = 11

// ParseLevel recognizes a level name in any letter case. slog prints a custom level as an offset from a
// named one: "DEBUG-4" is below debug and reads as trace, "ERROR+4" is above error and reads as fatal.
func ParseLevel(text []byte) Level {
	text = bytes.TrimSpace(text)
	var below, above bool
	if i := bytes.LastIndexAny(text, "+-"); i >= 0 && isDigits(text[i+1:]) {
		nonZero := len(bytes.Trim(text[i+1:], "0")) > 0
		below, above = nonZero && text[i] == '-', nonZero && text[i] == '+'
		text = text[:i]
	}

	switch level := parseLevelName(text); {
	case level == LevelDebug && below:
		return LevelTrace
	case level == LevelError && above:
		return LevelFatal
	default:
		return level
	}
}

func parseLevelName(name []byte) Level {
	if len(name) > maxLevelName {
		return LevelUnknown
	}
	var buf [maxLevelName]byte
	for i, c := range name {
		if 'A' <= c && c <= 'Z' {
			c += 'a' - 'A'
		}
		buf[i] = c
	}

	// The names cover zap, logrus, zerolog, slog, log15, Serilog, .NET and syslog, with their short forms.
	switch string(buf[:len(name)]) {
	case "trace", "trc", "trce", "verbose", "vrb":
		return LevelTrace
	case "debug", "dbg", "dbug":
		return LevelDebug
	case "info", "inf", "information", "notice":
		return LevelInfo
	case "warn", "wrn", "warning":
		return LevelWarn
	case "error", "err", "eror", "fail":
		return LevelError
	case "fatal", "ftl", "panic", "pnc", "dpanic", "critical", "crit", "alert", "emerg", "emergency":
		return LevelFatal
	}
	return LevelUnknown
}

func isDigits(b []byte) bool {
	for _, c := range b {
		if c < '0' || c > '9' {
			return false
		}
	}
	return len(b) > 0
}
