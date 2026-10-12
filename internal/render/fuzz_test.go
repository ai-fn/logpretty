package render

import (
	"bytes"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"testing"
	"unicode"
	"unicode/utf8"

	"github.com/ai-fn/logpretty/internal/entry"
	"github.com/ai-fn/logpretty/internal/parse"
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

// referenceQuoted is the rule for a key or a value stated apart from the code: in quotes when empty or when
// it holds one of the characters in special, with \" for a quote, and written safely either way.
func referenceQuoted(text, special string) string {
	safe := referenceSafe(text)
	if text == "" || strings.ContainsAny(text, special) {
		return `"` + strings.ReplaceAll(safe, `"`, `\"`) + `"`
	}
	return safe
}

// checkShown compares what was written with the rule and checks that nothing in it can reach the terminal.
func checkShown(t *testing.T, what, got, text, special string) {
	t.Helper()
	if want := referenceQuoted(text, special); got != want {
		t.Fatalf("%s %q\n got %q\nwant %q", what, text, got, want)
	}
	checkNothingUnsafe(t, what, text, got)
}

// checkNothingUnsafe checks that what was shown is valid UTF-8 and holds no rune the terminal acts on.
func checkNothingUnsafe(t *testing.T, what, text, got string) {
	t.Helper()
	if !utf8.ValidString(got) {
		t.Fatalf("%s %q is shown as %q, which is not valid UTF-8", what, text, got)
	}
	for _, r := range got {
		if unsafeRune(r) {
			t.Fatalf("%s %q is shown as %q, which holds %U", what, text, got, r)
		}
	}
}

func FuzzFormat(f *testing.F) {
	for _, seed := range []string{
		"", "a", "a b", `say "hi"`, "k=v", "a\tb", "\xff", "\x1b[31m", `a\nb`, `\u202e`, `\ud83d\ude80`, `C:\my dir`, `"`,
		`{"a":1}`, `[ 1 , {"b" : "c d"} ]`, `{"a":"\u001b[0m"}`, `{ "a" : "x\\" , "b" : 1 }`, "{\"a\":\"x\x7f\xff\"}", `[ ]`, `{"a":"x\`, `[ "x`, `}{`, `\`,
	} {
		f.Add([]byte(seed))
	}

	f.Fuzz(func(t *testing.T, data []byte) {
		// Clipped, so that a read past the end panics instead of finding spare capacity.
		raw := slices.Clip(slices.Clone(data))
		var fm formatter

		// The text as written in a logfmt value, and as the escapes of a JSON string would decode.
		for _, escaped := range []bool{false, true} {
			v := entry.Value{Raw: raw, Kind: entry.KindString, Escaped: escaped}
			checkShown(t, "value", string(fm.appendValue(nil, v)), string(v.AppendText(nil)), " \t\"")
		}
		checkShown(t, "key", string(appendKey(nil, raw)), string(raw), " \t\"=")

		// Whatever the bytes are, as an object or an array: no panic, nothing unsafe in the output.
		for _, kind := range []entry.Kind{entry.KindObject, entry.KindArray} {
			checkNothingUnsafe(t, "composite", string(raw), string(fm.appendValue(nil, entry.Value{Raw: raw, Kind: kind})))
		}

		// A JSON object or array comes out as the compact form of the same JSON.
		object := bytes.Trim(raw, " \t\r\n")
		if len(object) == 0 || object[0] != '{' && object[0] != '[' || !json.Valid(object) {
			return
		}
		var compact bytes.Buffer
		if err := json.Compact(&compact, object); err != nil {
			t.Fatalf("json.Compact(%q): %v", object, err)
		}
		kind := entry.KindObject
		if object[0] == '[' {
			kind = entry.KindArray
		}
		got := string(fm.appendValue(nil, entry.Value{Raw: object, Kind: kind}))
		if want := referenceSafe(compact.String()); got != want {
			t.Fatalf("JSON %q\n got %q\nwant %q", object, got, want)
		}
		checkNothingUnsafe(t, "JSON", string(object), got)
	})
}

// checkLine checks that a rendered line is one line of valid UTF-8 with nothing the terminal acts on.
func checkLine(t *testing.T, what string, line []byte, got string) {
	t.Helper()
	text, ok := strings.CutSuffix(got, "\n")
	if !ok || strings.Contains(text, "\n") {
		t.Fatalf("%s of %q is %q, want one line that ends in a newline", what, line, got)
	}
	checkNothingUnsafe(t, what, string(line), text)
}

func FuzzRender(f *testing.F) {
	for _, seed := range []string{
		`{"time":"2026-10-11T12:00:00.123Z","level":"warn","msg":"slow request","latency":"12ms","status":200}`,
		`{"msg":"a\u001b[31mb\nc","http":{"a": [1, 2]},"user id":5,"a=b":""}`, `{}`, ` {} `, `svc | {}`, `{"level":"info"}`,
		`{"@t":"2026-10-11T12:00:00Z","@mt":"x"}`, `web-1    | {"level":"info","msg":"started"}`,
		`2026-10-11T12:00:00.123456789Z {"level":"error","msg":"boom"}`, `time=2026-10-11T12:00:00Z level=info msg="a b" x=1`,
		`level=info msg=x verbose a=1`, `a= b=2`, `E1011 12:00:02.000003 x.go:10] "Failed" err="timeout" key="ns/x"`,
		"hello world", "", "\tat main.go:30", "\ufeffhello", "\ufeff{\"msg\":\"x\"}", "a\x1b[31mb\rc", "a\xffb", "\u202e{\"a\":1}",
		"{\"a\":\"x\x7f\u009b\xff\"}", "a=\x1b b=\u2028", "k\x00=1 b=2", "a\nb", `{"msg":"x"}` + "\n" + `{"msg":"y"}`,
	} {
		f.Add([]byte(seed), uint32(0))
	}
	f.Add([]byte("abc"), uint32(3<<20))

	f.Fuzz(func(t *testing.T, data []byte, n uint32) {
		dropped := int(n>>2) + 1 // a quarter, so that it fits an int of 32 bits with the one added
		// Clipped, so that a read past the end panics instead of finding spare capacity.
		line := slices.Clip(slices.Clone(data))
		// The parser and the renderer have seen other lines: nothing of them may show in this one.
		p := parse.Parser{Loc: testLoc}
		r := Renderer{Loc: testLoc}
		for _, earlier := range []string{
			`svc | {"time":"2026-10-11T12:00:00Z","level":"error","msg":"a \"long\" message\n","a":{"b":[1,2]},"c":"d\te"}`,
			`a=1 ` + strings.Repeat("word ", 20),
		} {
			r.AppendEntry(nil, p.Parse([]byte(earlier)))
		}

		e := p.Parse(line)
		plain := e.Format == entry.FormatPlain
		got := string(r.AppendEntry([]byte(">"), e))

		fresh := Renderer{Loc: testLoc}
		freshParser := parse.Parser{Loc: testLoc}
		if want := ">" + string(fresh.AppendEntry(nil, freshParser.Parse(line))); got != want {
			t.Fatalf("line %q after other lines\n got %q\nwant %q", line, got, want)
		}
		checkLine(t, "the entry", line, got[1:])
		// A plain line is the line itself, made safe.
		if want := referenceSafe(strings.TrimPrefix(string(line), "\ufeff")) + "\n"; plain && got[1:] != want {
			t.Fatalf("plain line %q\n got %q\nwant %q", line, got[1:], want)
		}

		cut := string(r.AppendTruncated(nil, line, dropped))
		checkLine(t, "the truncated line", line, cut)
		// A truncated line is the line itself, made safe, and the size that was dropped; size_test.go pins its format.
		want := referenceSafe(strings.TrimPrefix(string(line), "\ufeff")) + "…[+" + string(appendSize(nil, dropped)) + "]\n"
		if cut != want {
			t.Fatalf("truncated line %q\n got %q\nwant %q", line, cut, want)
		}
	})
}
