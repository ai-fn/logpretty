package entry

import (
	"slices"
	"testing"
)

func TestValueAppendText(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want string
	}{
		{name: "empty", raw: ``, want: ""},
		{name: "plain text", raw: `request done`, want: "request done"},
		{name: "quote", raw: `say \"hi\"`, want: `say "hi"`},
		{name: "backslash", raw: `C:\\temp`, want: `C:\temp`},
		{name: "slash", raw: `a\/b`, want: "a/b"},
		{name: "backspace and form feed", raw: `\b\f`, want: "\b\f"},
		{name: "newline, return, tab", raw: `a\nb\rc\td`, want: "a\nb\rc\td"},
		{name: "escape at both ends", raw: `\nmid\n`, want: "\nmid\n"},
		{name: "adjacent escapes", raw: `\\\\\"`, want: `\\"`},
		{name: "unicode escape", raw: `caf\u00e9`, want: "café"},
		{name: "unicode escape in upper case hex", raw: `\u00E9\u00C9`, want: "éÉ"},
		{name: "three-byte rune", raw: `\u20ac`, want: "€"},
		{name: "null", raw: `a\u0000b`, want: "a\x00b"},
		// The decoded text holds a real ESC: output must be sanitized after decoding, not before.
		{name: "escape character", raw: `\u001b[31mred`, want: "\x1b[31mred"},
		{name: "surrogate pair", raw: `\ud83d\ude00`, want: "😀"},
		{name: "surrogate pair between text", raw: `a\uD83D\uDE00b`, want: "a😀b"},
		{name: "lone high surrogate", raw: `a\ud83db`, want: "a\ufffdb"},
		{name: "lone high surrogate at the end", raw: `\ud83d`, want: "\ufffd"},
		{name: "lone low surrogate", raw: `\ude00`, want: "\ufffd"},
		{name: "high surrogate before a plain escape", raw: `\ud83d\n`, want: "\ufffd\n"},
		{name: "high surrogate before a non-surrogate", raw: `\ud83d\u0041`, want: "\ufffdA"},
		{name: "two high surrogates then a low one", raw: `\ud83d\ud83d\ude00`, want: "\ufffd😀"},
		{name: "high surrogate before a lone backslash", raw: `\ud83d\`, want: "\ufffd\\"},
		{name: "high surrogate before text that only looks like an escape", raw: `\ud83dxude00`, want: "\ufffdxude00"},
		{name: "high surrogate before a newline escape and hex digits", raw: `\ud83d\nde00`, want: "\ufffd\nde00"},
		{name: "replacement character before another escape", raw: `\ufffd\u0041`, want: "\ufffdA"},
		{name: "raw multi-byte text is kept", raw: `привет \u2713`, want: "привет ✓"},
		{name: "invalid UTF-8 is kept as is", raw: "a\xffb\\n", want: "a\xffb\n"},

		// logfmt is looser than JSON: what is not a known escape stays as written.
		{name: "unknown escape", raw: `a\qb`, want: `a\qb`},
		{name: "trailing backslash", raw: `abc\`, want: `abc\`},
		{name: "short unicode escape", raw: `\u12`, want: `\u12`},
		{name: "unicode escape one digit short", raw: `\u123`, want: `\u123`},
		{name: "unicode escape with the last digit bad", raw: `\u123g`, want: `\u123g`},
		{name: "hex digits at the edges of their ranges", raw: `\u0aF9\uAf09`, want: "\u0af9\uaf09"},
		{name: "unicode escape cut by the end", raw: `x\u`, want: `x\u`},
		{name: "unicode escape with a bad digit", raw: `\u12g4z`, want: `\u12g4z`},
		{name: "escaped backslash before u", raw: `\\u0041`, want: `\u0041`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Clipped, so that a read past the end of the value panics instead of finding spare capacity.
			v := Value{Raw: slices.Clip([]byte(tt.raw)), Kind: KindString, Escaped: true}

			if got := string(v.AppendText(nil)); got != tt.want {
				t.Errorf("AppendText(%q) = %q, want %q", tt.raw, got, tt.want)
			}
			if got := string(v.AppendText([]byte("dst:"))); got != "dst:"+tt.want {
				t.Errorf("AppendText(%q) onto a prefix = %q, want %q", tt.raw, got, "dst:"+tt.want)
			}
		})
	}
}

// A value is a window into a line: the bytes after it belong to the rest of the line, not to its escapes.
func TestValueAppendTextStopsAtTheEnd(t *testing.T) {
	tests := []struct {
		line string
		end  int
		want string
	}{
		{line: `\u1234`, end: 5, want: `\u123`},
		{line: `\ud83d\ude00`, end: 6, want: "\ufffd"},
		{line: `\ud83d\ude00`, end: 11, want: "\ufffd" + `\ude0`},
		{line: `abc\n`, end: 4, want: `abc\`},
	}
	for _, tt := range tests {
		v := Value{Raw: []byte(tt.line)[:tt.end], Kind: KindString, Escaped: true}
		if got := string(v.AppendText(nil)); got != tt.want {
			t.Errorf("AppendText(%q of %q) = %q, want %q", tt.line[:tt.end], tt.line, got, tt.want)
		}
	}
}

// Without the Escaped mark the bytes are the text, whatever they look like.
func TestValueAppendTextUnescaped(t *testing.T) {
	tests := []Value{
		{Raw: []byte(`a\nb`), Kind: KindString},
		{Raw: []byte(`12345678901234567891`), Kind: KindNumber},
		{Raw: []byte(`true`), Kind: KindBool},
		{Raw: []byte(`null`), Kind: KindNull},
		{Raw: []byte(`{"a":"\n"}`), Kind: KindObject},
		{Raw: []byte(`["\u0041"]`), Kind: KindArray},
		{Kind: KindNone},
	}
	for _, v := range tests {
		if got := string(v.AppendText([]byte(">"))); got != ">"+string(v.Raw) {
			t.Errorf("AppendText of %q = %q, want the bytes as written", v.Raw, got)
		}
	}
}

func TestValueAppendTextDoesNotAllocate(t *testing.T) {
	dst := make([]byte, 0, 64)
	tests := []Value{
		{Raw: []byte("request done"), Kind: KindString},
		{Raw: []byte(`say \"hi\"\n\u00e9\ud83d\ude00`), Kind: KindString, Escaped: true},
	}
	for _, v := range tests {
		if allocs := testing.AllocsPerRun(100, func() { _ = v.AppendText(dst) }); allocs != 0 {
			t.Errorf("AppendText(%q) allocates %v times per call, want 0", v.Raw, allocs)
		}
	}
}
