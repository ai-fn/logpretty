package render

import (
	"strings"
	"testing"

	"github.com/ai-fn/logpretty/internal/entry"
)

func str(raw string) entry.Value { return entry.Value{Raw: []byte(raw), Kind: entry.KindString} }
func esc(raw string) entry.Value {
	return entry.Value{Raw: []byte(raw), Kind: entry.KindString, Escaped: true}
}
func num(raw string) entry.Value  { return entry.Value{Raw: []byte(raw), Kind: entry.KindNumber} }
func lit(raw string) entry.Value  { return entry.Value{Raw: []byte(raw), Kind: entry.KindBool} }
func obj(raw string) entry.Value  { return entry.Value{Raw: []byte(raw), Kind: entry.KindObject} }
func list(raw string) entry.Value { return entry.Value{Raw: []byte(raw), Kind: entry.KindArray} }

func TestAppendValue(t *testing.T) {
	tests := []struct {
		name string
		v    entry.Value
		want string
	}{
		{name: "word", v: str("a"), want: "a"},
		{name: "two words are quoted", v: str("hello world"), want: `"hello world"`},
		{name: "empty", v: str(""), want: `""`},
		{name: "quote inside", v: str(`say "hi"`), want: `"say \"hi\""`},
		{name: "quote alone", v: str(`"`), want: `"\""`},
		{name: "tab is a space", v: str("a\tb"), want: "\"a\tb\""},
		{name: "equals sign needs no quotes in a value", v: str("a=b"), want: "a=b"},
		{name: "path with a backslash", v: str(`C:\dir`), want: `C:\dir`},
		{name: "path with a backslash and a space", v: str(`C:\my dir`), want: `"C:\my dir"`},
		{name: "no-break space is not a space", v: str("a\u00a0b"), want: "a\u00a0b"},
		{name: "Cyrillic word", v: str("привет"), want: "привет"},
		{name: "Cyrillic sentence", v: str("привет мир"), want: `"привет мир"`},
		{name: "raw escape character", v: str("a\x1bb"), want: `a\x1bb`},
		{name: "raw escape character in a quoted value", v: str("a \x1b b"), want: `"a \x1b b"`},
		{name: "invalid UTF-8", v: str("a\xffb"), want: "a\ufffdb"},
		{name: "invalid UTF-8 in a quoted value", v: str("a \xff"), want: "\"a \ufffd\""},
		{name: "bidi override", v: str("a\u202eb"), want: `a\u202eb`},

		{name: "decoded newline", v: esc(`a\nb`), want: `a\nb`},
		{name: "decoded newline in a quoted value", v: esc(`a\n b`), want: `"a\n b"`},
		{name: "decoded escape sequence", v: esc(`\u001b[31m`), want: `\x1b[31m`},
		{name: "decoded quotes", v: esc(`say \"hi\"`), want: `"say \"hi\""`},
		{name: "decoded backslash", v: esc(`a\\b`), want: `a\b`},
		{name: "decoded accent", v: esc(`caf\u00e9`), want: "café"},
		{name: "decoded surrogate pair", v: esc(`\ud83d\ude80`), want: "🚀"},
		{name: "decoded bidi override", v: esc(`\u202e`), want: `\u202e`},
		{name: "decoded NUL", v: esc(`\u0000`), want: `\x00`},
		{name: "escape that logfmt writes as text looks like the decoded one", v: str(`a\x1bb`), want: `a\x1bb`},

		{name: "integer", v: num("42"), want: "42"},
		{name: "exponent as written", v: num("1e3"), want: "1e3"},
		{name: "negative zero", v: num("-0"), want: "-0"},
		{name: "trailing zero", v: num("1.50"), want: "1.50"},
		{name: "beyond 2^53", v: num("9007199254740993"), want: "9007199254740993"},
		{name: "true", v: lit("true"), want: "true"},
		{name: "null", v: entry.Value{Raw: []byte("null"), Kind: entry.KindNull}, want: "null"},
		{name: "a number holding an escape character is still shown safe", v: num("1\x1b[0m"), want: `1\x1b[0m`},
		{name: "a literal holding a line break is still shown safe", v: lit("tr\nue"), want: `tr\nue`},
		{name: "bare logfmt key has no value", v: entry.Value{Kind: entry.KindNone}, want: ""},

		{name: "object", v: obj(`{"a":1}`), want: `{"a":1}`},
		{name: "object with spaces", v: obj(`{ "a" : 1 , "b" : [ 1 , 2 ] }`), want: `{"a":1,"b":[1,2]}`},
		{name: "object with line breaks", v: obj("{\n\t\"a\": 1,\r\n\t\"b\": 2\n}"), want: `{"a":1,"b":2}`},
		{name: "spaces inside strings are kept", v: obj(`{"msg":"a  b"}`), want: `{"msg":"a  b"}`},
		{name: "escaped quote inside a string", v: obj(`{ "a" : "x\"y z" }`), want: `{"a":"x\"y z"}`},
		{name: "string that ends in an escaped backslash", v: obj(`{ "a" : "x\\" , "b" : 1 }`), want: `{"a":"x\\","b":1}`},
		{name: "escape sequence inside a string stays text", v: obj(`{"a":"\u001b[0m"}`), want: `{"a":"\u001b[0m"}`},
		{name: "raw DEL inside a string", v: obj("{\"a\":\"x\x7fy\"}"), want: `{"a":"x\x7fy"}`},
		{name: "raw C1 inside a string", v: obj("{\"a\":\"x\u009by\"}"), want: `{"a":"x\x9by"}`},
		{name: "invalid UTF-8 inside a string", v: obj("{\"a\":\"x\xffy\"}"), want: "{\"a\":\"x\ufffdy\"}"},
		{name: "raw bidi override inside a string", v: obj("{\"a\":\"x\u202ey\"}"), want: `{"a":"x\u202ey"}`},
		{name: "empty array", v: list(`[ ]`), want: `[]`},
		{name: "array", v: list(`[ "a b" , null , {"c" : true} ]`), want: `["a b",null,{"c":true}]`},
		// The parser hands out only what json.Valid accepted, but nothing here may panic or loop on the rest.
		{name: "object cut inside a string", v: obj(`{"a":"x\`), want: `{"a":"x\`},
		{name: "object cut inside a string after a space", v: obj(`{ "a" : "open `), want: `{"a":"open `},
		{name: "array cut inside a string", v: list(`[ "x`), want: `["x`},
		{name: "closing before opening", v: obj(`}{`), want: `}{`},
		{name: "a lone backslash", v: obj(`\`), want: `\`},
		{name: "a lone quote", v: list(`"`), want: `"`},
		{name: "nothing", v: obj(``), want: ``},
		{name: "deep nesting", v: list(strings.Repeat("[ ", 5000) + strings.Repeat(" ]", 5000)), want: strings.Repeat("[", 5000) + strings.Repeat("]", 5000)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var f formatter
			if got := string(f.appendValue(nil, tt.v)); got != tt.want {
				t.Errorf("appendValue(%q) = %q, want %q", tt.v.Raw, got, tt.want)
			}
		})
	}
}

// One formatter serves every line: what a long value left in its scratch space must not show in the next one.
func TestAppendValueReusesScratch(t *testing.T) {
	var f formatter
	for _, v := range []entry.Value{esc(`first value \n with a long text`), esc(`short`), esc(`a\tb`), str("plain"), esc(`x`)} {
		var fresh formatter
		if got, want := string(f.appendValue(nil, v)), string(fresh.appendValue(nil, v)); got != want {
			t.Errorf("appendValue(%q) after other values = %q, want %q", v.Raw, got, want)
		}
	}
}

func TestAppendKey(t *testing.T) {
	tests := []struct {
		name, key, want string
	}{
		{name: "word", key: "msg", want: "msg"},
		{name: "dotted", key: "http.status", want: "http.status"},
		{name: "with a space", key: "user id", want: `"user id"`},
		{name: "with an equals sign", key: "a=b", want: `"a=b"`},
		{name: "empty", key: "", want: `""`},
		{name: "with a quote", key: `a"b`, want: `"a\"b"`},
		{name: "with a tab", key: "a\tb", want: "\"a\tb\""},
		{name: "with an escape character", key: "a\x1bb", want: `a\x1bb`},
		{name: "with an escape character and a space", key: "a \x1bb", want: `"a \x1bb"`},
		{name: "Cyrillic", key: "ключ", want: "ключ"},
		{name: "invalid UTF-8", key: "a\xff", want: "a\ufffd"},
		{name: "bidi override", key: "a\u202e", want: `a\u202e`},
		{name: "with a backslash", key: `a\b`, want: `a\b`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := string(appendKey(nil, []byte(tt.key))); got != tt.want {
				t.Errorf("appendKey(%q) = %q, want %q", tt.key, got, tt.want)
			}
		})
	}
}

func TestAppendKeepsDst(t *testing.T) {
	var f formatter
	if got, want := string(f.appendValue([]byte("k="), str("a b"))), `k="a b"`; got != want {
		t.Errorf("appendValue after k= gave %q, want %q", got, want)
	}
	if got, want := string(appendKey([]byte(">"), []byte("a b"))), `>"a b"`; got != want {
		t.Errorf("appendKey after > gave %q, want %q", got, want)
	}
}

// Values are windows into the line: nothing may be read past their end.
func TestAppendValueStopsAtTheEnd(t *testing.T) {
	line := []byte(`{"a" : "x\"" }"trailing`)
	var f formatter
	if got, want := string(f.appendValue(nil, entry.Value{Raw: line[: len(line)-9 : len(line)-9], Kind: entry.KindObject})), `{"a":"x\""}`; got != want {
		t.Errorf("appendValue = %q, want %q", got, want)
	}
}

func TestAppendValueDoesNotAllocate(t *testing.T) {
	values := map[string]entry.Value{
		"word":              str("request"),
		"quoted":            str("request done in 5ms"),
		"quoted with quote": str(`say "hi" to me`),
		"escaped":           esc(`line one\nline two \u001b[0m \u00e9 \ud83d\ude80`),
		"number":            num("12.50"),
		"object":            obj(`{ "method" : "GET" , "path" : "/api/v1/users" , "tags" : [ "a" , "b" ] }`),
		"binary":            str("\x00\xff\x80 \x1b"),
	}
	for name, v := range values {
		var f formatter
		dst := make([]byte, 0, 1024)
		// The first call grows the scratch space of the formatter.
		dst = f.appendValue(dst[:0], v)
		if allocs := testing.AllocsPerRun(100, func() { dst = f.appendValue(dst[:0], v) }); allocs != 0 {
			t.Errorf("appendValue allocates %v times for %s, want 0", allocs, name)
		}
	}

	key := []byte("user id")
	dst := make([]byte, 0, 64)
	if allocs := testing.AllocsPerRun(100, func() { dst = appendKey(dst[:0], key) }); allocs != 0 {
		t.Errorf("appendKey allocates %v times, want 0", allocs)
	}
}
