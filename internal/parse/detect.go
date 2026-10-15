package parse

import (
	"bytes"
	"time"

	"github.com/ai-fn/logpretty/internal/entry"
)

const (
	// maxJSONPrefix is the longest text allowed in front of a JSON object. Past it, or after text that no
	// log collector adds, a brace is more likely a part of a sentence, and most such lines are not JSON.
	maxJSONPrefix = 64
	// maxCollectorName bounds the name of a pod, a container or a service, so that a long word is not walked
	// to its end on every line.
	maxCollectorName = 253
	// shortName is the length of a name that is scanned byte by byte; a longer one is looked at as a whole first.
	shortName = 64
	// maxStamp bounds the timestamp of a collector the same way; RFC 3339 with nanoseconds takes 35 bytes.
	maxStamp = 64
)

const byteOrderMark = "\xef\xbb\xbf"

// Parser turns log lines into entries. It reuses one Entry, so it is not safe for concurrent use.
type Parser struct {
	Loc *time.Location // the zone of times written without one; nil means time.Local

	entry entry.Entry // elements of Fields past its length are scratch that still point into old lines
}

// Parse classifies the line and splits it into fields; the result is valid until the next call.
func (p *Parser) Parse(line []byte) *entry.Entry {
	e := &p.entry
	e.Reset()
	e.Line = line

	// Windows tools start a file with a byte order mark, which would hide the first brace or key.
	start := bytes.TrimPrefix(line, []byte(byteOrderMark))
	var body []byte
	var stampTime time.Time
	e.Collector, e.Source, e.Stamp, body, stampTime = splitCollector(start)
	e.Body = body
	fields, prefix, split, ok := appendJSONLineFields(e.Fields, body)
	format := entry.FormatJSON
	// Cheaper than the tokenizer for the plain lines that have no pair at all.
	if !ok && !isIndented(body) && bytes.IndexByte(body, '=') >= 0 {
		fields, prefix, ok = appendLogfmtFields(fields, body)
		format = entry.FormatLogfmt
		// The text before logfmt pairs is often the logger's own, as in klog or slog's "log" handler.
		split.ok = ok && len(prefix) > 0 && len(prefix) <= maxJSONPrefix
		if split.ok {
			split = splitLenientPrefix(prefix)
		}
	}
	// Kept for a plain line too: a rejected one may have grown the slice.
	e.Fields = fields
	if ok {
		e.Format = format
		switch {
		case split.ok:
			stampTime = addCollector(e, start, body, prefix, split, p.Loc)
		case len(prefix) > 0:
			e.Prefix = prefix
		}
		assignRoles(e, p.Loc)
	}
	// A stack trace has no time of its own, and the time a collector wrote is often the only one an entry has.
	if e.TimeField() < 0 {
		e.Time = stampTime
	}
	return e
}

// isIndented reports whether the line starts with a space or a tab. Such a line is a frame of a stack trace,
// even when it holds key=value pairs, so it is never logfmt.
func isIndented(line []byte) bool {
	return len(line) > 0 && (line[0] == ' ' || line[0] == '\t')
}

// splitCollector splits off what kubectl logs --prefix --timestamps or docker compose logs -t wrote in front
// of the line; the forms are strict, since a plain line has nothing else to vouch for them.
func splitCollector(line []byte) (collector, source, stamp, body []byte, when time.Time) {
	body = line
	// The first bytes are tested here, so that most lines make no call at all.
	if len(body) > 0 && body[0] == '[' {
		if n, name := podPrefix(body); n > 0 {
			collector, source, body = body[:n:n], name, afterSeparator(body[n:])
		}
	} else if len(body) > 0 && isAlnum(body[0]) {
		if n, name := composePrefix(body); n > 0 {
			collector, source, body = body[:n:n], name, afterSeparator(body[n:])
		}
	}
	if len(body) >= 20 && body[10] == 'T' {
		if n, t := stampLen(body); n > 0 {
			stamp, body, when = body[:n:n], afterSeparator(body[n:]), t
		}
	}
	return collector, source, stamp, body[:len(body):len(body)], when
}

// afterSeparator drops the one whitespace byte between what a collector wrote and the line; the ones after it are the line's.
func afterSeparator(rest []byte) []byte {
	if len(rest) > 0 && isSpace(rest[0]) {
		return rest[1:]
	}
	return rest
}

// podPrefix returns the length of "[pod/NAME/CONTAINER]" at the start of line, and the text in the brackets,
// if a space or the end of the line follows; 0 otherwise.
func podPrefix(line []byte) (n int, name []byte) {
	const head = "[pod/"
	if !bytes.HasPrefix(line, []byte(head)) {
		return 0, nil
	}
	slash := scanName(line, len(head))
	if slash == len(head) || slash == len(line) || line[slash] != '/' {
		return 0, nil
	}
	end := scanName(line, slash+1)
	if end == slash+1 || end == len(line) || line[end] != ']' {
		return 0, nil
	}
	if n = end + 1; n < len(line) && line[n] != ' ' {
		return 0, nil
	}
	return n, line[1:end:end]
}

// composePrefix returns the length of "NAME  |" at the start of line, and the name, if a space or the end of
// the line follows. docker compose pads every name to the longest one and one space more.
func composePrefix(line []byte) (n int, name []byte) {
	end := scanNameTo(line, 0, shortName)
	if end == shortName {
		// A long word without a space, such as a hex dump, is no prefix: one vectorized search tells. Index 253
		// is in the window, since a name of 253 bytes may end there.
		if bytes.IndexByte(line[shortName:min(len(line), maxCollectorName+1)], ' ') < 0 {
			return 0, nil
		}
		end = scanNameTo(line, shortName, maxCollectorName)
	}
	// end == 0 guards line[0] on an empty line; a name may hold '.', '_' and '-', but not start with them.
	if end == 0 || !isAlnum(line[0]) || end == len(line) || line[end] != ' ' {
		return 0, nil
	}
	bar := end
	for bar < len(line) && line[bar] == ' ' && bar-end < maxCollectorName {
		bar++
	}
	if bar == len(line) || line[bar] != '|' {
		return 0, nil
	}
	if n = bar + 1; n < len(line) && line[n] != ' ' {
		return 0, nil
	}
	return n, line[:end:end]
}

// scanName returns the end of the name that starts at line[i]: letters, digits, '.', '_' and '-'.
func scanName(line []byte, i int) int { return scanNameTo(line, i, i+maxCollectorName) }

// scanNameTo is scanName that stops before line[limit].
func scanNameTo(line []byte, i, limit int) int {
	limit = min(len(line), limit)
	for i < limit && nameBytes[line[i]] {
		i++
	}
	return i
}

// nameBytes marks the bytes of a name; a table, since the name of every line that starts with a word is scanned.
var nameBytes = func() (t [256]bool) {
	for c := range t {
		t[c] = isAlnum(byte(c)) || c == '.' || c == '_' || c == '-'
	}
	return t
}()

func isAlnum(c byte) bool {
	return isDigit(c) || 'a' <= c|0x20 && c|0x20 <= 'z'
}

// stampLen returns the length and the time of the timestamp at the start of line, as kubectl and docker write it:
// RFC 3339 with "Z" or "±hh:mm", followed by a space or the end of the line; 0 otherwise.
func stampLen(line []byte) (int, time.Time) {
	// The caller tests the same first; this guards the indexing below.
	if len(line) < 20 || line[10] != 'T' {
		return 0, time.Time{}
	}
	n := bytes.IndexByte(line[:min(len(line), maxStamp+1)], ' ')
	if n < 0 {
		if len(line) > maxStamp {
			return 0, time.Time{}
		}
		n = len(line)
	}
	// A date, a time and a zone take 20 bytes at the least.
	if n < 20 {
		return 0, time.Time{}
	}
	stamp := line[:n]
	zoned := stamp[n-1] == 'Z' || (stamp[n-6] == '+' || stamp[n-6] == '-') && stamp[n-3] == ':'
	if !zoned {
		return 0, time.Time{}
	}
	t, ok := parseDateTime(stamp, time.UTC)
	if !ok {
		return 0, time.Time{}
	}
	return n, t
}

// lenientPrefix is the text before a structured part, split as a log collector writes it.
type lenientPrefix struct {
	bar   int    // the length of the text up to and with its last bar; 0 if it has none
	stamp []byte // the timestamp after the bar, or the whole text without one; nil if there is none
	ok    bool   // the text is a timestamp, a name with its bar, or both
}

// splitLenientPrefix splits the text before a JSON object or logfmt pairs, looser than the strict forms, since the
// structured part vouches for the line; docker compose -t puts the timestamp after the bar.
func splitLenientPrefix(prefix []byte) lenientPrefix {
	var split lenientPrefix
	stamp := prefix
	if bar := bytes.LastIndexByte(prefix, '|'); bar >= 0 {
		split.bar, stamp = bar+1, prefix[bar+1:]
		for len(stamp) > 0 && isSpace(stamp[0]) {
			stamp = stamp[1:]
		}
		if len(stamp) == 0 {
			split.ok = true
			return split
		}
	}
	if _, ok := parseDateTime(stamp, time.UTC); !ok {
		return lenientPrefix{}
	}
	split.stamp, split.ok = stamp, true
	return split
}

// addCollector adds a vetted prefix, which stands in body past the strict forms, to what the collector wrote, and returns
// the time of its timestamp. The collector's text runs to the last timestamp, or to the bar when none follows it.
func addCollector(e *entry.Entry, start, body, prefix []byte, split lenientPrefix, loc *time.Location) (stampTime time.Time) {
	at := len(start) - len(body)
	for isSpace(start[at]) {
		at++
	}
	end := at + len(prefix) - len(split.stamp)
	if split.bar > 0 {
		end = at + split.bar
	}
	e.Collector = clipped(bytes.Trim(start[:end], spaces))
	if e.Source == nil && split.bar > 0 {
		e.Source = clipped(bytes.Trim(prefix[:split.bar-1], spaces))
	}
	e.Stamp = clipped(split.stamp)
	body = afterSeparator(start[at+len(prefix):])
	e.Body = body[:len(body):len(body)]
	// Vetted by splitLenientPrefix; unlike a strict one, it may lack a zone and so be in loc.
	if e.Stamp != nil {
		stampTime, _ = parseDateTime(e.Stamp, loc)
	}
	return stampTime
}

// clipped returns b without spare capacity, so that an append cannot overwrite the line, or nil if it is empty.
func clipped(b []byte) []byte {
	if len(b) == 0 {
		return nil
	}
	return b[:len(b):len(b)]
}

// appendJSONLineFields appends the fields of a line that is a JSON object, alone or after the prefix of
// a log collector: the timestamp of "kubectl logs --timestamps" or the "name |" of "docker compose logs".
func appendJSONLineFields(dst []entry.Field, line []byte) (fields []entry.Field, prefix []byte, split lenientPrefix, ok bool) {
	// The cheapest test goes first: few lines that are not JSON end in a brace.
	end := len(line)
	for end > 0 && isSpace(line[end-1]) {
		end--
	}
	if end == 0 || line[end-1] != '}' {
		return dst, nil, split, false
	}
	start := 0
	for isSpace(line[start]) {
		start++
	}
	if line[start] == '{' {
		fields, ok = appendJSONFields(dst, line[start:])
		return fields, nil, split, ok
	}

	open := bytes.IndexByte(line, '{')
	if open < 0 {
		return dst, nil, split, false
	}
	prefixEnd := open
	for isSpace(line[prefixEnd-1]) {
		prefixEnd--
	}
	// Capacity is capped, so that an append cannot overwrite the line.
	prefix = line[start:prefixEnd:prefixEnd]
	if len(prefix) > maxJSONPrefix {
		return dst, nil, split, false
	}
	if split = splitLenientPrefix(prefix); !split.ok {
		return dst, nil, split, false
	}
	if fields, ok = appendJSONFields(dst, line[open:]); !ok {
		return dst, nil, lenientPrefix{}, false
	}
	return fields, prefix, split, true
}
