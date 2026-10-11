package parse

import (
	"time"

	"github.com/ai-fn/logpretty/internal/entry"
)

// Keys that name each role, the most trusted first: "msg" wins over "event" wherever it stands in the line.
var (
	timeKeys  = [...]string{"time", "timestamp", "@timestamp", "asctime", "ts", "@t", "t"}
	levelKeys = [...]string{"level", "lvl", "severity", "log.level", "levelname", "log_level", "loglevel", "@l"}
	msgKeys   = [...]string{"msg", "message", "@message", "@m", "event", "@mt"}
)

// A time outside these years is taken for some other number: a counter, a duration, an id.
const (
	minYear = 2000
	maxYear = 2099
	minUnix = 946684800  // 2000-01-01T00:00:00Z
	maxUnix = 4102444800 // 2100-01-01T00:00:00Z
)

// assignRoles marks the fields that hold the time, the level and the message, and parses the first two.
// A field whose value does not fit its role stays an ordinary one. A time without a zone is in loc, or local if nil.
func assignRoles(e *entry.Entry, loc *time.Location) {
	// Roles left from another line would point past the fields of this one.
	e.Time, e.Level = time.Time{}, entry.LevelUnknown
	e.SetTimeField(-1)
	e.SetLevelField(-1)
	e.SetMsgField(-1)

	timeRank, levelRank, msgRank := len(timeKeys), len(levelKeys), len(msgKeys)
	for i, f := range e.Fields {
		if r := rank(timeKeys[:], f.Key); r < timeRank {
			if t, ok := parseTime(f.Value, loc); ok {
				e.Time, timeRank = t, r
				e.SetTimeField(i)
			}
		}
		if r := rank(levelKeys[:], f.Key); r < levelRank {
			if level := parseLevel(f.Value); level != entry.LevelUnknown {
				e.Level, levelRank = level, r
				e.SetLevelField(i)
			}
		}
		// An empty message gives way to the next key, as a time that cannot be read does.
		if r := rank(msgKeys[:], f.Key); r < msgRank && f.Value.Kind == entry.KindString && len(f.Value.Raw) > 0 {
			msgRank = r
			e.SetMsgField(i)
		}
	}
}

// rank returns the position of key in keys, or len(keys). Letter case is ignored: .NET writes "Level".
func rank(keys []string, key []byte) int {
	for i, k := range keys {
		if equalFold(key, k) {
			return i
		}
	}
	return len(keys)
}

// equalFold reports whether b is the lower-case ASCII string lower, whatever the case of its own letters.
func equalFold(b []byte, lower string) bool {
	if len(b) != len(lower) {
		return false
	}
	for i, c := range b {
		if 'A' <= c && c <= 'Z' {
			c += 'a' - 'A'
		}
		if c != lower[i] {
			return false
		}
	}
	return true
}

// parseLevel reads a level written as a name or as a pino and bunyan number.
func parseLevel(v entry.Value) entry.Level {
	if v.Kind != entry.KindString && v.Kind != entry.KindNumber {
		return entry.LevelUnknown
	}
	if level := entry.ParseLevel(v.Raw); level != entry.LevelUnknown {
		return level
	}
	// pino and bunyan count in tens, from 10 for trace to 60 for fatal.
	if len(v.Raw) == 2 && '1' <= v.Raw[0] && v.Raw[0] <= '6' && v.Raw[1] == '0' {
		return entry.LevelTrace + entry.Level(v.Raw[0]-'1')
	}
	return entry.LevelUnknown
}

// parseTime reads a timestamp: a date and time, or a Unix time in seconds, milli-, micro- or nanoseconds.
// A time without a zone is in loc, any other comes back in UTC.
func parseTime(v entry.Value, loc *time.Location) (time.Time, bool) {
	switch v.Kind {
	case entry.KindString:
		if t, ok := parseDateTime(v.Raw, loc); ok {
			return t, true
		}
		return parseUnixTime(v.Raw) // logfmt has no numbers
	case entry.KindNumber:
		return parseUnixTime(v.Raw)
	}
	return time.Time{}, false
}

// parseDateTime reads "2006-01-02T15:04:05", with "T" or a space, an optional fraction and an optional zone,
// which may follow a space, as time.Parse would. It is here because time.Parse allocates: a string, and a Location.
func parseDateTime(b []byte, loc *time.Location) (time.Time, bool) {
	if len(b) < 19 || b[4] != '-' || b[7] != '-' || (b[10] != 'T' && b[10] != ' ') || b[13] != ':' || b[16] != ':' {
		return time.Time{}, false
	}
	year, month, day := number(b[0:4]), number(b[5:7]), number(b[8:10])
	hour, minute, sec := number(b[11:13]), number(b[14:16]), number(b[17:19])
	// number gives -1 for what is not digits; the conversions turn it into a number too large to pass.
	if year < minYear || year > maxYear || month < 1 || month > 12 || day < 1 || day > daysIn(month, year) ||
		uint(hour) > 23 || uint(minute) > 59 || uint(sec) > 59 {
		return time.Time{}, false
	}

	rest := b[19:]
	nsec := 0
	if len(rest) >= 2 && (rest[0] == '.' || rest[0] == ',') && isDigit(rest[1]) {
		n := 1
		// The scale runs out after nine digits: the ones past nanoseconds add nothing.
		for scale := 100_000_000; n < len(rest) && isDigit(rest[n]); n, scale = n+1, scale/10 {
			nsec += int(rest[n]-'0') * scale
		}
		rest = rest[n:]
	}

	if len(rest) == 0 {
		if loc == nil {
			loc = time.Local
		}
		return time.Date(year, time.Month(month), day, hour, minute, sec, nsec, loc), true
	}
	// Ruby's Time#to_s writes a space before a numeric offset; "Z" never gets one.
	if len(rest) > 1 && rest[0] == ' ' && (rest[1] == '+' || rest[1] == '-') {
		rest = rest[1:]
	}
	offset, ok := zoneOffset(rest)
	if !ok {
		return time.Time{}, false
	}
	utc := time.Date(year, time.Month(month), day, hour, minute, sec, nsec, time.UTC)
	return utc.Add(-time.Duration(offset) * time.Second), true
}

// zoneOffset reads "Z", "+07:00", "+0700" or "+07" as seconds east of UTC.
func zoneOffset(b []byte) (seconds int, ok bool) {
	if len(b) == 1 && b[0] == 'Z' {
		return 0, true
	}
	if b[0] != '+' && b[0] != '-' {
		return 0, false
	}
	hours, minutes := -1, 0
	switch {
	case len(b) == 3:
		hours = number(b[1:3])
	case len(b) == 5:
		hours, minutes = number(b[1:3]), number(b[3:5])
	case len(b) == 6 && b[3] == ':':
		hours, minutes = number(b[1:3]), number(b[4:6])
	}
	// time.Parse lets 24 hours and 60 minutes through, "as some people do write" them.
	if uint(hours) > 24 || uint(minutes) > 60 {
		return 0, false
	}
	seconds = (hours*60 + minutes) * 60
	if b[0] == '-' {
		seconds = -seconds
	}
	return seconds, true
}

// parseUnixTime reads a decimal number as a Unix time. The unit is the one that puts it between the
// years 2000 and 2100: the four ranges are far apart, so at most one fits.
func parseUnixTime(b []byte) (time.Time, bool) {
	i := 0
	var whole uint64
	for ; i < len(b) && isDigit(b[i]); i++ {
		// Nineteen digits fit in 64 bits and cover the nanoseconds of the year 2100.
		if i == 19 {
			return time.Time{}, false
		}
		whole = whole*10 + uint64(b[i]-'0')
	}
	// The fraction in billionths of the unit; zap writes seconds as "1791720000.1234567".
	var frac uint64
	if i < len(b) && (b[i] != '.' || i+1 == len(b)) {
		return time.Time{}, false
	}
	for j, scale := i+1, uint64(100_000_000); j < len(b); j, scale = j+1, scale/10 {
		if !isDigit(b[j]) {
			return time.Time{}, false
		}
		frac += uint64(b[j]-'0') * scale
	}

	// perSecond is how many of the unit make a second: seconds, then milli-, micro- and nanoseconds.
	for perSecond := uint64(1); perSecond <= 1e9; perSecond *= 1000 {
		if whole >= minUnix*perSecond && whole < maxUnix*perSecond {
			nanos := (whole%perSecond)*(1e9/perSecond) + frac/perSecond
			return time.Unix(int64(whole/perSecond), int64(nanos)).UTC(), true
		}
	}
	return time.Time{}, false
}

// number reads a string of digits that is short enough not to overflow, or returns -1.
func number(b []byte) int {
	n := 0
	for _, c := range b {
		if !isDigit(c) {
			return -1
		}
		n = n*10 + int(c-'0')
	}
	return n
}

func isDigit(c byte) bool {
	return '0' <= c && c <= '9'
}

// daysIn returns the length of a month. Every fourth year between minYear and maxYear is a leap one:
// 2000 is, and the next century year that is not lies past maxYear.
func daysIn(month, year int) int {
	switch month {
	case 2:
		if year%4 == 0 {
			return 29
		}
		return 28
	case 4, 6, 9, 11:
		return 30
	}
	return 31
}
