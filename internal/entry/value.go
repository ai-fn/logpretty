package entry

import (
	"bytes"
	"unicode/utf16"
	"unicode/utf8"
)

// AppendText appends the text of the value to dst: a string with its escapes decoded, anything else as written.
func (v Value) AppendText(dst []byte) []byte {
	if !v.Escaped {
		return append(dst, v.Raw...)
	}
	return appendUnescaped(dst, v.Raw)
}

// appendUnescaped decodes JSON string escapes the way encoding/json does. logfmt shares the syntax but
// not the strictness, so anything that is not a complete escape is kept as written.
func appendUnescaped(dst, s []byte) []byte {
	for {
		i := bytes.IndexByte(s, '\\')
		if i < 0 || i+1 == len(s) {
			return append(dst, s...)
		}
		dst = append(dst, s[:i]...)
		s = s[i:]

		switch c := s[1]; c {
		case '"', '\\', '/':
			dst = append(dst, c)
		case 'b':
			dst = append(dst, '\b')
		case 'f':
			dst = append(dst, '\f')
		case 'n':
			dst = append(dst, '\n')
		case 'r':
			dst = append(dst, '\r')
		case 't':
			dst = append(dst, '\t')
		case 'u':
			r, ok := hex4(s[2:])
			if !ok {
				dst = append(dst, s[:2]...)
				break
			}
			s = s[6:]
			if low, ok := unicodeEscape(s); ok {
				if pair := utf16.DecodeRune(r, low); pair != utf8.RuneError {
					dst = utf8.AppendRune(dst, pair)
					s = s[6:]
					continue
				}
			}
			dst = utf8.AppendRune(dst, r) // half of a surrogate pair on its own comes out as U+FFFD
			continue
		default:
			dst = append(dst, s[:2]...)
		}
		s = s[2:]
	}
}

// unicodeEscape reads a \uXXXX escape from the start of s.
func unicodeEscape(s []byte) (rune, bool) {
	if len(s) < 2 || s[0] != '\\' || s[1] != 'u' {
		return 0, false
	}
	return hex4(s[2:])
}

// hex4 reads four hexadecimal digits from the start of s.
func hex4(s []byte) (rune, bool) {
	if len(s) < 4 {
		return 0, false
	}
	var r rune
	for _, c := range s[:4] {
		switch {
		case '0' <= c && c <= '9':
			c -= '0'
		case 'a' <= c && c <= 'f':
			c -= 'a' - 10
		case 'A' <= c && c <= 'F':
			c -= 'A' - 10
		default:
			return 0, false
		}
		r = r<<4 | rune(c)
	}
	return r, true
}
