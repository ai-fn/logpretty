// Package parse recognizes the format of a log line and splits it into fields.
package parse

import (
	"bytes"
	"encoding/json"

	"github.com/ai-fn/logpretty/internal/entry"
)

// appendJSONFields appends the top-level pairs of a JSON object to dst in their original order.
// It reports false and returns dst unchanged unless line is exactly one JSON object.
func appendJSONFields(dst []entry.Field, line []byte) ([]entry.Field, bool) {
	b := trimLeftJSONSpace(line)
	// encoding/json decides what is JSON. Everything below trusts that verdict and only finds boundaries.
	if len(b) == 0 || b[0] != '{' || !json.Valid(b) {
		return dst, false
	}

	i := skipJSONSpace(b, 1)
	for b[i] != '}' {
		end, escaped := stringEnd(b, i+1)
		// Capacity is capped on every slice handed out, so that an append cannot overwrite the line.
		key := b[i+1 : end : end]
		if escaped {
			key = entry.Value{Raw: key, Escaped: true}.AppendText(nil)
		}
		i = skipJSONSpace(b, end+1) // at the colon
		i = skipJSONSpace(b, i+1)   // at the value

		var value entry.Value
		value, i = jsonValue(b, i)
		dst = append(dst, entry.Field{Key: key, Value: value})

		i = skipJSONSpace(b, i)
		if b[i] == ',' {
			i = skipJSONSpace(b, i+1)
		}
	}
	return dst, true
}

// jsonValue returns the value that starts at b[i] and the index just past it.
func jsonValue(b []byte, i int) (entry.Value, int) {
	kind := entry.KindNumber
	switch b[i] {
	case '"':
		end, escaped := stringEnd(b, i+1)
		return entry.Value{Raw: b[i+1 : end : end], Kind: entry.KindString, Escaped: escaped}, end + 1
	case '{':
		end := compositeEnd(b, i)
		return entry.Value{Raw: b[i:end:end], Kind: entry.KindObject}, end
	case '[':
		end := compositeEnd(b, i)
		return entry.Value{Raw: b[i:end:end], Kind: entry.KindArray}, end
	case 't', 'f':
		kind = entry.KindBool
	case 'n':
		kind = entry.KindNull
	}
	end := i
	for !isJSONSpace(b[end]) && b[end] != ',' && b[end] != '}' {
		end++
	}
	return entry.Value{Raw: b[i:end:end], Kind: kind}, end
}

// stringEnd returns the index of the quote that closes the string whose text starts at b[start],
// and whether the text has escapes.
func stringEnd(b []byte, start int) (end int, escaped bool) {
	end = start + bytes.IndexByte(b[start:], '"')
	first := bytes.IndexByte(b[start:end], '\\')
	if first < 0 {
		return end, false
	}
	// The quote found above may be an escaped one: walk from the first backslash instead.
	for end = start + first; b[end] != '"'; end++ {
		if b[end] == '\\' {
			end++
		}
	}
	return end, true
}

// compositeEnd returns the index just past the object or array that opens at b[i].
func compositeEnd(b []byte, i int) int {
	for depth := 0; ; i++ {
		switch b[i] {
		case '"':
			i, _ = stringEnd(b, i+1)
		case '{', '[':
			depth++
		case '}', ']':
			if depth--; depth == 0 {
				return i + 1
			}
		}
	}
}

func skipJSONSpace(b []byte, i int) int {
	for isJSONSpace(b[i]) {
		i++
	}
	return i
}

// trimLeftJSONSpace drops only what JSON calls whitespace: bytes.TrimSpace would also drop Unicode
// spaces that encoding/json rejects.
func trimLeftJSONSpace(b []byte) []byte {
	for len(b) > 0 && isJSONSpace(b[0]) {
		b = b[1:]
	}
	return b
}

func isJSONSpace(c byte) bool {
	return c == ' ' || c == '\t' || c == '\n' || c == '\r'
}
