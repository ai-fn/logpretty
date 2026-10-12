package render

import (
	"bytes"
	"fmt"
	"slices"
	"strings"
	"testing"
	"unicode"
	"unicode/utf8"
)

// referenceSafe is the obvious implementation appendSafe must agree with: one rune at a time, through fmt.
func referenceSafe(s string) string {
	var b strings.Builder
	for _, r := range s { // an invalid byte comes out as U+FFFD, one for each
		switch {
		case r == '\t':
			b.WriteByte('\t')
		case r == '\n':
			b.WriteString(`\n`)
		case r == '\r':
			b.WriteString(`\r`)
		case r < 0x20 || r == 0x7f || 0x80 <= r && r <= 0x9f:
			fmt.Fprintf(&b, `\x%02x`, r)
		case 0x202a <= r && r <= 0x202e || 0x2066 <= r && r <= 0x2069 || r == 0x2028 || r == 0x2029 || r == 0xfeff:
			fmt.Fprintf(&b, `\u%04x`, r)
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

// unsafeRune says what must never reach the terminal, stated apart from both implementations.
func unsafeRune(r rune) bool {
	return unicode.IsControl(r) && r != '\t' || unicode.In(r, unicode.Bidi_Control) && r != 0x200e && r != 0x200f && r != 0x061c ||
		r == 0x2028 || r == 0x2029 || r == 0xfeff
}

func FuzzAppendSafe(f *testing.F) {
	for _, seed := range []string{
		"", "plain text", "tab\there", "a\nb\r\nc", "\x1b[31mred\x1b[0m", "\x1b]0;title\x07", "\u009b31m", "a\x9bb", "\xff\xfe",
		"\xe2\x82", "\xc0\xaf", "\xed\xa0\x80", "\u202eevil", "\u2066x\u2069", "\u2028\u2029\ufeff", "привет", "\u045b\u4e9b",
		"👨\u200d👩\u200d👧", "\x00\x7f\x80\x9f\xa0", "\\x1b \\u202e", strings.Repeat("a\x1b", 50),
	} {
		f.Add([]byte(seed))
	}

	f.Fuzz(func(t *testing.T, data []byte) {
		// Clipped, so that a read past the end panics instead of finding spare capacity.
		src := slices.Clip(slices.Clone(data))
		got := appendSafe([]byte("x="), src)

		if !bytes.HasPrefix(got, []byte("x=")) {
			t.Fatalf("appendSafe(%q) = %q: the start of dst was changed", src, got)
		}
		out := got[2:]
		if want := referenceSafe(string(src)); string(out) != want {
			t.Fatalf("appendSafe(%q)\n got %q\nwant %q", src, out, want)
		}
		if !utf8.Valid(out) {
			t.Fatalf("appendSafe(%q) = %q is not valid UTF-8", src, out)
		}
		for _, r := range string(out) {
			if unsafeRune(r) {
				t.Fatalf("appendSafe(%q) = %q holds %U", src, out, r)
			}
		}
		if len(out) > 4*len(src) {
			t.Fatalf("appendSafe(%q) is %d bytes for %d, want at most four times as many", src, len(out), len(src))
		}
		if again := appendSafe(nil, out); !bytes.Equal(again, out) {
			t.Fatalf("appendSafe is not idempotent: %q gives %q, then %q", src, out, again)
		}
		// What needs no change comes back as it was.
		needsEscape := func(r rune) bool { return unsafeRune(r) || r == '\n' || r == '\r' }
		if clean := utf8.Valid(src) && !strings.ContainsFunc(string(src), needsEscape); clean && !bytes.Equal(out, src) {
			t.Fatalf("appendSafe(%q) = %q, want a clean text back as it was", src, out)
		}
	})
}
