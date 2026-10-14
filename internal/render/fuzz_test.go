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
		want := referenceSafe(string(src))
		if string(out) != want {
			t.Fatalf("appendSafe(%q)\n got %q\nwant %q", src, out, want)
		}
		// Inside a rendered part the tab is written out too; nothing else differs.
		inline := string(AppendInline([]byte("x="), src))
		if wantInline := "x=" + strings.ReplaceAll(want, "\t", `\t`); inline != wantInline {
			t.Fatalf("AppendInline(%q)\n got %q\nwant %q", src, inline, wantInline)
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
	safe := strings.ReplaceAll(referenceSafe(text), "\t", `\t`)
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
		"", "a", "a b", `say "hi"`, "k=v", "a\tb", "a\tb\"c", "a \tb", "{\"a\":\"x\ty\", \"b\":1}", "[\t1,\t\"x\ty\"]", "\xff", "\x1b[31m", `a\nb`, `\u202e`, `\ud83d\ude80`, `C:\my dir`, `"`,
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
			checkShown(t, "value", string(fm.appendValue(nil, v)), string(v.AppendText(nil)), " \"")
		}
		checkShown(t, "key", string(appendKey(nil, raw)), string(raw), " \"=")

		// Whatever the bytes are, as an object or an array: no panic, nothing unsafe in the output.
		for _, kind := range []entry.Kind{entry.KindObject, entry.KindArray} {
			composite := string(fm.appendValue(nil, entry.Value{Raw: raw, Kind: kind}))
			checkNothingUnsafe(t, "composite", string(raw), composite)
			if strings.Contains(composite, "\t") {
				t.Fatalf("composite %q is shown as %q, which holds a tab", raw, composite)
			}
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

// stripStyles removes the styles of the default theme and checks that each one wraps some text and is
// closed before the next one opens and before the line ends.
func stripStyles(t *testing.T, what string, line []byte, got string) string {
	t.Helper()
	th := DefaultTheme()
	reset := th.Reset
	styles := append([]string{th.Time, th.Key, th.Prefix, th.Marker}, th.Level[entry.LevelTrace:]...)
	var text strings.Builder
	rest, ok := strings.CutSuffix(got, "\n")
	if !ok {
		t.Fatalf("%s of %q is %q, want a newline at the very end", what, line, got)
	}
	open := false
	for {
		start := strings.IndexByte(rest, 0x1b)
		if start < 0 {
			break
		}
		end := strings.IndexByte(rest[start:], 'm')
		if end < 0 {
			t.Fatalf("%s of %q is %q, want every escape character to start a style", what, line, got)
		}
		style := rest[start : start+end+1]
		switch {
		case style != reset && !slices.Contains(styles, style):
			t.Fatalf("%s of %q is %q: %q is neither a style of the theme nor %q", what, line, got, style, reset)
		case style == reset && !open:
			t.Fatalf("%s of %q is %q: a style is closed that was not opened", what, line, got)
		case style == reset && start == 0:
			t.Fatalf("%s of %q is %q: a style wraps nothing", what, line, got)
		case style != reset && open:
			t.Fatalf("%s of %q is %q: a style opens inside another", what, line, got)
		}
		open = style != reset
		text.WriteString(rest[:start])
		rest = rest[start+end+1:]
	}
	if open {
		t.Fatalf("%s of %q is %q: the last style is left open", what, line, got)
	}
	text.WriteString(rest)
	return text.String() + "\n"
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
	// A tab in each part a structured line is built from, and in its prefix, where it stays.
	for _, seed := range []string{`{"msg":"a\tb","k\t":"c\td"}`, "aaaaaaaa=1 b=\"x\ty\"", "aaaaaaaaaa=1 bbbbbbbbbb=2 c=\"x\ty", "one\ttwo three a=1 b=2"} {
		f.Add([]byte(seed), uint32(0))
	}
	// What a log collector wrote, with an empty line after it and before a frame of a stack trace.
	for _, seed := range []string{
		"web-1 | ", "web-1 |", "[pod/web-1/app] 2026-10-11T12:00:05Z \tat x", "[pod/a/b] 2026-10-11T12:00:05Z ",
		`[pod/web-1/app] {"msg":"x"}`, "a\t| 2026-10-11T12:00:00Z\t{\"a\":1}", "a\t| {}",
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
		structured := !plain && len(e.Fields) > 0
		prefixTabs := bytes.Count(e.Collector, []byte("\t")) + bytes.Count(e.Stamp, []byte("\t")) + bytes.Count(e.Prefix, []byte("\t"))
		got := string(r.AppendEntry([]byte(">"), e))

		fresh := Renderer{Loc: testLoc}
		freshParser := parse.Parser{Loc: testLoc}
		if want := ">" + string(fresh.AppendEntry(nil, freshParser.Parse(line))); got != want {
			t.Fatalf("line %q after other lines\n got %q\nwant %q", line, got, want)
		}
		checkLine(t, "the entry", line, got[1:])
		// The parts a structured line is built from write a tab out; only its prefix is printed as written.
		if tabs := strings.Count(got, "\t"); structured && tabs != prefixTabs {
			t.Fatalf("line %q is shown as %q with %d tabs, want the %d of its prefix", line, got, tabs, prefixTabs)
		}
		// A plain line is the line itself, made safe, but the space after what a collector wrote when nothing follows it.
		want := referenceSafe(strings.TrimPrefix(string(line), "\ufeff"))
		if (e.Collector != nil || e.Stamp != nil) && len(e.Body) == 0 {
			want = strings.TrimSuffix(want, " ")
		}
		if want += "\n"; plain && got[1:] != want {
			t.Fatalf("plain line %q\n got %q\nwant %q", line, got[1:], want)
		}
		// An entry without fields is what the collector wrote and the rest of the line, one space apart.
		if !plain && !structured {
			want := referenceSafe(strings.TrimPrefix(string(line), "\ufeff"))
			if e.Collector != nil || e.Stamp != nil {
				parts := []string{}
				for _, part := range [][]byte{e.Collector, e.Stamp, e.Body} {
					if len(part) > 0 {
						parts = append(parts, string(part))
					}
				}
				want = referenceSafe(strings.Join(parts, " "))
			}
			if got[1:] != want+"\n" {
				t.Fatalf("entry without fields %q\n got %q\nwant %q", line, got[1:], want+"\n")
			}
		}

		cut := string(r.AppendTruncated(nil, line, dropped))
		checkLine(t, "the truncated line", line, cut)
		// A truncated line is the line itself, made safe, and the size that was dropped; size_test.go pins its format.
		want = referenceSafe(strings.TrimPrefix(string(line), "\ufeff")) + "…[+" + string(appendSize(nil, dropped)) + "]\n"
		if cut != want {
			t.Fatalf("truncated line %q\n got %q\nwant %q", line, cut, want)
		}

		// Color adds styles and nothing else.
		colored := Renderer{Theme: DefaultTheme(), Loc: testLoc}
		styled := string(colored.AppendEntry(nil, p.Parse(line)))
		if text := stripStyles(t, "the colored entry", line, styled); text != got[1:] {
			t.Fatalf("line %q in color, styles removed\n got %q\nwant %q", line, text, got[1:])
		}
		styled = string(colored.AppendTruncated(nil, line, dropped))
		if text := stripStyles(t, "the colored truncated line", line, styled); text != cut {
			t.Fatalf("truncated line %q in color, styles removed\n got %q\nwant %q", line, text, cut)
		}

		// A choice of fields and a hidden time: names that are roles, names the seeds have, and two of the entry's own.
		names := []string{"msg", "nope", "a", "http.a", "level", "status", "time"}
		for i := 0; i < len(e.Fields) && i < 2; i++ {
			names = append(names, string(e.Fields[i].Key))
		}
		for _, noTime := range []bool{false, true} {
			for _, fields := range [][]string{nil, names} {
				chosen := Renderer{Loc: testLoc, NoTime: noTime, Fields: Select(fields)}
				chosen.AppendEntry(nil, p.Parse([]byte(`{"http":{"a":"other"},"status":"other","msg":"other"}`)))
				part := string(chosen.AppendEntry([]byte(">"), p.Parse(line)))[1:]
				what := fmt.Sprintf("the entry with fields %q and no time %v", fields, noTime)

				switch {
				case !structured:
					// A line that is printed as it is stays so, whatever was chosen.
					if part != got[1:] {
						t.Fatalf("%s of %q\n got %q\nwant %q", what, line, part, got[1:])
					}
				case part != "":
					// Nothing at all is how an entry with nothing left is shown; anything else is one safe line.
					checkLine(t, what, line, part)
					if tabs := strings.Count(part, "\t"); tabs != prefixTabs {
						t.Fatalf("%s of %q is %q with %d tabs, want the %d of its prefix", what, line, part, tabs, prefixTabs)
					}
				}
				if fields == nil && !noTime && part != got[1:] {
					t.Fatalf("%s of %q\n got %q\nwant %q", what, line, part, got[1:])
				}
				// Without a time to hide, hiding it changes nothing.
				if e.Time.IsZero() && noTime {
					shown := Renderer{Loc: testLoc, Fields: Select(fields)}
					if want := string(shown.AppendEntry(nil, p.Parse(line))); part != want {
						t.Fatalf("%s of %q, which has no time\n got %q\nwant %q", what, line, part, want)
					}
				}

				inColor := Renderer{Theme: DefaultTheme(), Loc: testLoc, NoTime: noTime, Fields: Select(fields)}
				styled := string(inColor.AppendEntry(nil, p.Parse(line)))
				if styled == "" && part == "" {
					continue
				}
				if text := stripStyles(t, what+" in color", line, styled); text != part {
					t.Fatalf("%s of %q in color, styles removed\n got %q\nwant %q", what, line, text, part)
				}
			}
		}
	})
}
