package render

import (
	"bytes"

	"github.com/ai-fn/logpretty/internal/entry"
)

// formatter holds the scratch space that decoding the escapes of a value needs.
type formatter struct {
	scratch []byte
}

// appendValue appends the value as it is shown: a string safe, quoted if empty or holding a space, tab or quote;
// an object or array as compact JSON; the rest as written. dst must not share memory with v.Raw.
func (f *formatter) appendValue(dst []byte, v entry.Value) []byte {
	switch v.Kind {
	case entry.KindNone: // a bare logfmt key
		return dst
	case entry.KindObject, entry.KindArray:
		return appendCompact(dst, v.Raw)
	case entry.KindString:
		return appendText(dst, f.text(v), " \t\"")
	}
	return appendSafe(dst, v.Raw)
}

// text returns the value with its escapes decoded; the result is valid until the next call.
func (f *formatter) text(v entry.Value) []byte {
	if !v.Escaped {
		return v.Raw
	}
	f.scratch = v.AppendText(f.scratch[:0])
	return f.scratch
}

// appendKey appends a key, in quotes if it is empty or holds a space, a tab, a quote or an equals sign.
func appendKey(dst, key []byte) []byte {
	return appendText(dst, key, " \t\"=")
}

// appendText appends text safe, in quotes if it is empty or holds a byte of special.
func appendText(dst, text []byte, special string) []byte {
	if len(text) > 0 && !bytes.ContainsAny(text, special) {
		return appendSafe(dst, text)
	}
	dst = append(dst, '"')
	// Splitting at a quote cannot cut a UTF-8 sequence: no byte of one is ASCII.
	for {
		i := bytes.IndexByte(text, '"')
		if i < 0 {
			break
		}
		dst = append(appendSafe(dst, text[:i]), '\\', '"')
		text = text[i+1:]
	}
	return append(appendSafe(dst, text), '"')
}

// appendCompact appends JSON without the whitespace between its tokens. Strings are kept as written, with
// their escapes, so that what they decode to cannot reach the terminal.
func appendCompact(dst, raw []byte) []byte {
	start := 0 // raw[start:i] holds no whitespace outside strings and is not appended yet
	inString := false
	for i := 0; i < len(raw); i++ {
		switch c := raw[i]; {
		case inString:
			switch c {
			case '\\':
				i++ // the escaped byte cannot end the string
			case '"':
				inString = false
			}
		case c == '"':
			inString = true
		case isJSONSpace(c):
			dst = appendSafe(dst, raw[start:i])
			start = i + 1
		}
	}
	return appendSafe(dst, raw[start:])
}

func isJSONSpace(c byte) bool {
	return c == ' ' || c == '\t' || c == '\n' || c == '\r'
}
