package render

import (
	"fmt"
	"testing"
	"unicode/utf8"
)

func TestAppendSafe(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{name: "empty", in: "", want: ""},
		{name: "plain ASCII", in: "hello, world: {a=1} [x] ~", want: "hello, world: {a=1} [x] ~"},
		{name: "tab stays", in: "a\tb\t", want: "a\tb\t"},
		{name: "newline", in: "a\nb", want: `a\nb`},
		{name: "carriage return", in: "a\rb", want: `a\rb`},
		{name: "CRLF", in: "a\r\nb", want: `a\r\nb`},
		{name: "NUL", in: "a\x00b", want: `a\x00b`},
		{name: "bell", in: "a\x07b", want: `a\x07b`},
		{name: "backspace", in: "a\x08b", want: `a\x08b`},
		{name: "vertical tab and form feed", in: "\x0b\x0c", want: `\x0b\x0c`},
		{name: "last C0", in: "\x1f", want: `\x1f`},
		{name: "space is the first printable", in: " ", want: " "},
		{name: "last printable ASCII", in: "~", want: "~"},
		{name: "DEL", in: "a\x7fb", want: `a\x7fb`},
		{name: "backslash stays", in: `C:\dir \x1b`, want: `C:\dir \x1b`},

		{name: "escape sequence that sets the window title", in: "\x1b]0;x\x07", want: `\x1b]0;x\x07`},
		{name: "color sequence", in: "\x1b[31mred\x1b[0m", want: `\x1b[31mred\x1b[0m`},
		{name: "8-bit CSI", in: "\u009b31m", want: `\x9b31m`},
		{name: "first C1", in: "\u0080", want: `\x80`},
		{name: "NEL", in: "a\u0085b", want: `a\x85b`},
		{name: "last C1", in: "\u009f", want: `\x9f`},
		{name: "no-break space is not a control", in: "a\u00a0b", want: "a\u00a0b"},
		{name: "letter with accent", in: "é", want: "é"},

		{name: "lone continuation byte", in: "\x80", want: "\ufffd"},
		{name: "continuation byte that would be CSI", in: "a\x9bb", want: "a\ufffdb"},
		{name: "0xff", in: "a\xffb", want: "a\ufffdb"},
		{name: "lead byte at the end, as truncation leaves it", in: "ab\xd0", want: "ab\ufffd"},
		{name: "euro sign cut short", in: "ab\xe2\x82", want: "ab\ufffd\ufffd"},
		{name: "overlong slash", in: "\xc0\xaf", want: "\ufffd\ufffd"},
		{name: "UTF-16 surrogate", in: "\xed\xa0\x80", want: "\ufffd\ufffd\ufffd"},
		{name: "above the last code point", in: "\xf4\x90\x80\x80", want: "\ufffd\ufffd\ufffd\ufffd"},
		{name: "valid rune after a broken one", in: "\xe2\x82é", want: "\ufffd\ufffdé"},
		{name: "replacement character stays", in: "a\ufffdb", want: "a\ufffdb"},

		{name: "left-to-right embedding", in: "\u202a", want: `\u202a`},
		{name: "right-to-left override", in: "a\u202eb", want: `a\u202eb`},
		{name: "last of the embeddings and overrides", in: "\u202e", want: `\u202e`},
		{name: "first isolate", in: "\u2066", want: `\u2066`},
		{name: "last isolate", in: "\u2069", want: `\u2069`},
		{name: "line separator", in: "\u2028", want: `\u2028`},
		{name: "paragraph separator", in: "\u2029", want: `\u2029`},
		{name: "byte order mark", in: "\ufeffa", want: `\ufeffa`},
		{name: "just below the embeddings", in: "\u2029\u2029", want: `\u2029\u2029`},
		{name: "neighbors of the escaped ranges", in: "\u2027\u202f\u2065\u206a\ufefe\uff00", want: "\u2027\u202f\u2065\u206a\ufefe\uff00"},
		{name: "direction marks are text", in: "\u200e\u200f\u061c\u200b\u200d", want: "\u200e\u200f\u061c\u200b\u200d"},

		{name: "Cyrillic", in: "привет, мир", want: "привет, мир"},
		{name: "Cyrillic letter whose second byte is 0x9b", in: "\u045b", want: "\u045b"},
		{name: "CJK character whose last byte is 0x9b", in: "\u4e9b", want: "\u4e9b"},
		{name: "CJK", in: "日本語", want: "日本語"},
		{name: "emoji", in: "ok 🚀", want: "ok 🚀"},
		{name: "family emoji with joiners", in: "👨\u200d👩\u200d👧", want: "👨\u200d👩\u200d👧"},
		{name: "flag", in: "🇷🇺", want: "🇷🇺"},

		{name: "escape first and last", in: "\x01abc\x02", want: `\x01abc\x02`},
		{name: "escapes next to each other", in: "\x01\x02\u202e\xff", want: "\\x01\\x02\\u202e\ufffd"},
		{name: "every kind in a row", in: "a\tb\nc\x1bd\u009be\xfff\u202eg\u2028h", want: "a\tb\\nc\\x1bd\\x9be\ufffdf\\u202eg\\u2028h"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := string(appendSafe(nil, []byte(tt.in))); got != tt.want {
				t.Errorf("appendSafe(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

// Every byte on its own: the C0 range, DEL, and every byte from 0x80 up, none of which is a whole rune.
func TestAppendSafeEveryByte(t *testing.T) {
	for b := range 256 {
		var want string
		switch {
		case b == '\t' || ' ' <= b && b < 0x7f:
			want = string(rune(b))
		case b == '\n':
			want = `\n`
		case b == '\r':
			want = `\r`
		case b < 0x80:
			want = fmt.Sprintf(`\x%02x`, b)
		default:
			want = "\ufffd"
		}
		if got := string(appendSafe(nil, []byte{byte(b)})); got != want {
			t.Errorf("appendSafe(%#02x) = %q, want %q", b, got, want)
		}
	}
}

// Every code point that can be written in UTF-8, against the rule stated one rune at a time.
func TestAppendSafeEveryRune(t *testing.T) {
	for r := rune(0); r <= utf8.MaxRune; r++ {
		if 0xd800 <= r && r <= 0xdfff {
			continue // surrogates have no UTF-8 form
		}
		in := string(r)
		got := string(appendSafe(nil, []byte(in)))
		if want := referenceSafe(in); got != want {
			t.Fatalf("appendSafe(%U) = %q, want %q", r, got, want)
		}
		// The oracle is built from the Unicode tables, not from the ranges the implementation and the reference share.
		for _, o := range got {
			if unsafeRune(o) {
				t.Fatalf("appendSafe(%U) = %q holds %U", r, got, o)
			}
		}
	}
}

func TestAppendSafeKeepsDst(t *testing.T) {
	dst := []byte("key=")
	got := appendSafe(dst, []byte("a\x1bb"))
	if want := `key=a\x1bb`; string(got) != want {
		t.Errorf("appendSafe(%q, ...) = %q, want %q", dst, got, want)
	}

	if got := appendSafe([]byte("x"), nil); string(got) != "x" {
		t.Errorf("appendSafe with nothing to add = %q, want %q", got, "x")
	}
}

// The line may be the last thing the reader holds: appendSafe must not read past the slice it was given.
func TestAppendSafeStopsAtTheEnd(t *testing.T) {
	buf := []byte("ab\xe2\x82\xac\x1b")
	for end := range len(buf) + 1 {
		got := appendSafe(nil, buf[:end:end])
		if want := referenceSafe(string(buf[:end])); string(got) != want {
			t.Errorf("appendSafe(%q) = %q, want %q", buf[:end], got, want)
		}
	}
}

func TestAppendSafeDoesNotAllocate(t *testing.T) {
	inputs := map[string]string{
		"ASCII":                "2026-10-11 12:00:00 INFO request done in 5ms from 10.0.0.1",
		"Cyrillic":             "привет, мир, это строка лога",
		"escapes":              "a\x1b[31mb\nc\u009bd\xffe\u202ef",
		"nothing but escapes":  "\x00\x01\x02\u2028\u2029\ufeff",
		"binary":               "\x00\xff\x80\xfe\x1b\x7f",
		"truncated at the end": "text \xe2\x82",
	}
	for name, in := range inputs {
		src := []byte(in)
		dst := make([]byte, 0, 6*len(src))
		if allocs := testing.AllocsPerRun(100, func() { dst = appendSafe(dst[:0], src) }); allocs != 0 {
			t.Errorf("appendSafe allocates %v times for %s, want 0", allocs, name)
		}
	}
}
