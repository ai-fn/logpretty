// Package render formats parsed log entries for a terminal.
package render

import "unicode/utf8"

const hexDigits = "0123456789abcdef"

// appendSafe appends src with what a terminal would act on written out as text: control characters except
// the tab, bidi overrides, line separators. Invalid UTF-8 becomes U+FFFD per byte; dst must not share memory with src.
func appendSafe(dst, src []byte) []byte { return appendEscaped(dst, src, true) }

// AppendInline is appendSafe for a part of a line, of the output or of a message: there a tab would break
// the layout, so it is written out as well. dst must not share memory with src.
func AppendInline(dst, src []byte) []byte { return appendEscaped(dst, src, false) }

func appendEscaped(dst, src []byte, keepTab bool) []byte {
	start := 0 // src[start:i] is clean and not appended yet
	for i := 0; i < len(src); {
		c := src[i]
		if ' ' <= c && c < 0x7f || c == '\t' && keepTab {
			i++
			continue
		}

		size := 1
		if c < utf8.RuneSelf {
			dst = appendEscape(append(dst, src[start:i]...), rune(c))
		} else {
			r, n := utf8.DecodeRune(src[i:])
			switch {
			case n == 1: // a multi-byte rune is two bytes or more, so this is a byte that is not UTF-8
				dst = utf8.AppendRune(append(dst, src[start:i]...), utf8.RuneError)
			case isUnsafeRune(r):
				dst = appendEscape(append(dst, src[start:i]...), r)
				size = n
			default:
				i += n
				continue
			}
		}
		i += size
		start = i
	}
	return append(dst, src[start:]...)
}

// isUnsafeRune reports the multi-byte runes to write out: C1 controls (8-bit CSI), bidi overrides and
// isolates, line and paragraph separators, and the byte order mark.
func isUnsafeRune(r rune) bool {
	return 0x80 <= r && r <= 0x9f || 0x202a <= r && r <= 0x202e || 0x2066 <= r && r <= 0x2069 ||
		r == 0x2028 || r == 0x2029 || r == 0xfeff
}

// appendEscape writes r as \n, \r, \t, \xNN or \uNNNN.
func appendEscape(dst []byte, r rune) []byte {
	switch {
	case r == '\n':
		return append(dst, '\\', 'n')
	case r == '\t':
		return append(dst, '\\', 't')
	case r == '\r':
		return append(dst, '\\', 'r')
	case r < 0x100:
		return append(dst, '\\', 'x', hexDigits[r>>4], hexDigits[r&0xf])
	}
	return append(dst, '\\', 'u', hexDigits[r>>12], hexDigits[r>>8&0xf], hexDigits[r>>4&0xf], hexDigits[r&0xf])
}
