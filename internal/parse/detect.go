package parse

import (
	"bytes"
	"time"

	"github.com/ai-fn/logpretty/internal/entry"
)

// maxJSONPrefix is the longest text allowed in front of a JSON object. Past it, or after text that no
// log collector adds, a brace is more likely a part of a sentence, and most such lines are not JSON.
const maxJSONPrefix = 64

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
	body := bytes.TrimPrefix(line, []byte(byteOrderMark))
	fields, prefix, ok := appendJSONLineFields(e.Fields, body)
	format := entry.FormatJSON
	// Cheaper than the tokenizer for the plain lines that have no pair at all.
	if !ok && bytes.IndexByte(body, '=') >= 0 {
		fields, prefix, ok = appendLogfmtFields(fields, body)
		format = entry.FormatLogfmt
	}
	// Kept for a plain line too: a rejected one may have grown the slice.
	e.Fields = fields
	if !ok {
		return e
	}
	e.Prefix, e.Format = prefix, format
	// appendJSONLineFields has vetted a JSON prefix; the text before logfmt pairs is often the logger's
	// own, as in klog or slog's "log" handler.
	e.CollectorPrefix = len(prefix) > 0 &&
		(format == entry.FormatJSON || (len(prefix) <= maxJSONPrefix && isCollectorPrefix(prefix)))
	assignRoles(e, p.Loc)
	return e
}

// appendJSONLineFields appends the fields of a line that is a JSON object, alone or after the prefix of
// a log collector: the timestamp of "kubectl logs --timestamps" or the "name |" of "docker compose logs".
func appendJSONLineFields(dst []entry.Field, line []byte) (fields []entry.Field, prefix []byte, ok bool) {
	// The cheapest test goes first: few lines that are not JSON end in a brace.
	end := len(line)
	for end > 0 && isSpace(line[end-1]) {
		end--
	}
	if end == 0 || line[end-1] != '}' {
		return dst, nil, false
	}
	start := 0
	for isSpace(line[start]) {
		start++
	}
	if line[start] == '{' {
		fields, ok = appendJSONFields(dst, line[start:])
		return fields, nil, ok
	}

	open := bytes.IndexByte(line, '{')
	if open < 0 {
		return dst, nil, false
	}
	prefixEnd := open
	for isSpace(line[prefixEnd-1]) {
		prefixEnd--
	}
	// Capacity is capped, so that an append cannot overwrite the line.
	prefix = line[start:prefixEnd:prefixEnd]
	if len(prefix) > maxJSONPrefix || !isCollectorPrefix(prefix) {
		return dst, nil, false
	}
	if fields, ok = appendJSONFields(dst, line[open:]); !ok {
		return dst, nil, false
	}
	return fields, prefix, true
}

// isCollectorPrefix reports whether the text is a timestamp, a container name with its bar, or both.
// "docker compose logs --timestamps" puts the timestamp after the bar, so that is where it is looked for.
func isCollectorPrefix(prefix []byte) bool {
	if bar := bytes.LastIndexByte(prefix, '|'); bar >= 0 {
		prefix = prefix[bar+1:]
		for len(prefix) > 0 && isSpace(prefix[0]) {
			prefix = prefix[1:]
		}
		if len(prefix) == 0 {
			return true
		}
	}
	_, ok := parseDateTime(prefix, time.UTC)
	return ok
}
