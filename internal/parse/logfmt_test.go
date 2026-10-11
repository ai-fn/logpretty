package parse

import (
	"slices"
	"strings"
	"testing"

	"github.com/ai-fn/logpretty/internal/entry"
)

// token is a logfmtToken in a form that is easy to write and compare in tests.
type token struct {
	text    string
	key     string
	value   string
	pair    bool
	escaped bool
}

func tokensOf(line []byte) []token {
	out := []token{}
	for tok, ok := nextLogfmtToken(line, 0); ok; tok, ok = nextLogfmtToken(line, tok.end) {
		tt := token{text: string(line[tok.start:tok.end]), pair: tok.pair, escaped: tok.escaped}
		if tok.pair {
			tt.key, tt.value = string(line[tok.start:tok.keyEnd]), string(line[tok.valStart:tok.valEnd])
		}
		out = append(out, tt)
	}
	return out
}

func TestNextLogfmtToken(t *testing.T) {
	kv := func(text, key, value string) token { return token{text: text, key: key, value: value, pair: true} }
	bare := func(text string) token { return token{text: text} }

	tests := []struct {
		name string
		line string
		want []token
	}{
		{name: "empty line", line: "", want: []token{}},
		{name: "blank line", line: " \t ", want: []token{}},
		{name: "pair", line: `k=v`, want: []token{kv(`k=v`, "k", "v")}},
		{name: "empty value", line: `k=`, want: []token{kv(`k=`, "k", "")}},
		{name: "empty quoted value", line: `k=""`, want: []token{kv(`k=""`, "k", "")}},
		{name: "empty value before another pair", line: `k= b=1`, want: []token{kv(`k=`, "k", ""), kv(`b=1`, "b", "1")}},
		{name: "quoted value with a space", line: `k="a b"`, want: []token{kv(`k="a b"`, "k", "a b")}},
		{
			name: "quoted value with escaped quotes",
			line: `k="a \"b\"" n=1`,
			want: []token{{text: `k="a \"b\""`, key: "k", value: `a \"b\"`, pair: true, escaped: true}, kv(`n=1`, "n", "1")},
		},
		{
			name: "quoted value ending in an escaped backslash",
			line: `k="C:\\" n=1`,
			want: []token{{text: `k="C:\\"`, key: "k", value: `C:\\`, pair: true, escaped: true}, kv(`n=1`, "n", "1")},
		},
		{name: "a backslash outside quotes is plain text", line: `path=C:\dir`, want: []token{kv(`path=C:\dir`, "path", `C:\dir`)}},
		{name: "a quote inside a bare value is plain text", line: `k=a"b c`, want: []token{kv(`k=a"b`, "k", `a"b`), bare("c")}},
		{name: "a quoted value ends at its closing quote", line: `k="a"b`, want: []token{kv(`k="a"`, "k", "a"), bare("b")}},
		{name: "equals signs in a value", line: `k==v q=a=b`, want: []token{kv(`k==v`, "k", "=v"), kv(`q=a=b`, "q", "a=b")}},
		{name: "key with dots, dashes and at signs", line: `a.b-c@d=1`, want: []token{kv(`a.b-c@d=1`, "a.b-c@d", "1")}},
		{
			name: "key starting with an underscore or an at sign",
			line: `_x=1 @timestamp=2`,
			want: []token{kv(`_x=1`, "_x", "1"), kv(`@timestamp=2`, "@timestamp", "2")},
		},
		{name: "upper-case key with digits", line: `Level2=INFO`, want: []token{kv(`Level2=INFO`, "Level2", "INFO")}},
		{name: "every digit in a key", line: `a0=1 b9=2`, want: []token{kv(`a0=1`, "a0", "1"), kv(`b9=2`, "b9", "2")}},
		{name: "non-ASCII key and value", line: `ключ=значение`, want: []token{kv(`ключ=значение`, "ключ", "значение")}},
		{name: "tabs between pairs", line: "a=1\tb=2", want: []token{kv(`a=1`, "a", "1"), kv(`b=2`, "b", "2")}},
		{name: "whitespace around", line: "  a=1  b=2 \t", want: []token{kv(`a=1`, "a", "1"), kv(`b=2`, "b", "2")}},
		{name: "carriage return ends a value", line: "a=1\r", want: []token{kv(`a=1`, "a", "1")}},
		{
			name: "vertical tab and form feed are not whitespace",
			line: "a=1\vb=2 c\fd",
			want: []token{kv("a=1\vb=2", "a", "1\vb=2"), bare("c\fd")},
		},
		{name: "no-break space is not whitespace", line: "a=1\u00a0b=2", want: []token{kv("a=1\u00a0b=2", "a", "1\u00a0b=2")}},
		{name: "byte order mark is part of the key", line: "\ufeffa=1", want: []token{kv("\ufeffa=1", "\ufeffa", "1")}},
		{name: "NUL in a value and in a key", line: "a=x\x00y b\x00=1", want: []token{kv("a=x\x00y", "a", "x\x00y"), bare("b\x00=1")}},
		{name: "word", line: `started`, want: []token{bare("started")}},
		{name: "URL with a query is not a pair", line: `http://x?a=1`, want: []token{bare(`http://x?a=1`)}},
		{name: "no key", line: `=v`, want: []token{bare(`=v`)}},
		{name: "key starting with a digit", line: `1a=2`, want: []token{bare(`1a=2`)}},
		{name: "key starting with a dash or a dot", line: `-a=1 .b=2`, want: []token{bare(`-a=1`), bare(`.b=2`)}},
		{name: "key with a colon", line: `a:b=1`, want: []token{bare(`a:b=1`)}},
		{name: "key with brackets", line: `(retries=3, a[0]=1`, want: []token{bare(`(retries=3,`), bare(`a[0]=1`)}},
		{name: "quoted key", line: `"k"=v`, want: []token{bare(`"k"=v`)}},
		{name: "key at the end of the line", line: `a=1 key`, want: []token{kv(`a=1`, "a", "1"), bare("key")}},
		{name: "unclosed quote takes the rest of the line", line: `a=1 k="b c=2 d`, want: []token{kv(`a=1`, "a", "1"), bare(`k="b c=2 d`)}},
		{name: "unclosed quote without the whitespace at the end", line: "k=\"b c \t", want: []token{bare(`k="b c`)}},
		{name: "only an opening quote", line: `k="`, want: []token{bare(`k="`)}},
		{name: "escaped closing quote", line: `k="abc\"`, want: []token{bare(`k="abc\"`)}},
		{name: "backslash at the end of a quoted value", line: `k="abc\`, want: []token{bare(`k="abc\`)}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Clipped, so that a read past the end of the line panics instead of finding spare capacity.
			if got := tokensOf(slices.Clip([]byte(tt.line))); !slices.Equal(got, tt.want) {
				t.Errorf("tokens of %q\n got %+v\nwant %+v", tt.line, got, tt.want)
			}
		})
	}
}

func TestAppendLogfmtFields(t *testing.T) {
	str := func(key, raw string) pair { return pair{key, raw, entry.KindString, false} }
	none := func(key string) pair { return pair{key, "", entry.KindNone, false} }

	tests := []struct {
		name       string
		line       string
		wantPrefix string
		want       []pair
	}{
		{
			name: "pairs only",
			line: `level=info msg="hello world" port=8080`,
			want: []pair{str("level", "info"), str("msg", "hello world"), str("port", "8080")},
		},
		{name: "two pairs are enough", line: `a=1 b=2`, want: []pair{str("a", "1"), str("b", "2")}},
		{
			name: "escapes in a value are kept raw and marked",
			line: `msg="say \"hi\"" n=1`,
			want: []pair{{"msg", `say \"hi\"`, entry.KindString, true}, str("n", "1")},
		},
		{name: "numbers and literals are strings", line: `n=1 ok=true z=null`, want: []pair{str("n", "1"), str("ok", "true"), str("z", "null")}},
		{name: "duplicate keys are both kept", line: `a=1 a=2`, want: []pair{str("a", "1"), str("a", "2")}},
		{name: "empty values", line: `a= b=""`, want: []pair{str("a", ""), str("b", "")}},
		{
			name:       "date and time before the pairs",
			line:       `2026/10/11 12:00:00 level=info msg=x`,
			wantPrefix: `2026/10/11 12:00:00`,
			want:       []pair{str("level", "info"), str("msg", "x")},
		},
		{
			name:       "container name before the pairs",
			line:       `svc | level=info msg=x`,
			wantPrefix: `svc |`,
			want:       []pair{str("level", "info"), str("msg", "x")},
		},
		{
			name:       "the prefix has no whitespace around it",
			line:       "  svc-1  |  level=info  msg=x  ",
			wantPrefix: `svc-1  |`,
			want:       []pair{str("level", "info"), str("msg", "x")},
		},
		{
			name:       "a long prefix does not count against the pairs",
			line:       `Oct 11 12:00:00 host app[123]: a=1 b=2`,
			wantPrefix: `Oct 11 12:00:00 host app[123]:`,
			want:       []pair{str("a", "1"), str("b", "2")},
		},
		{
			name: "a word after the first pair is a field without a value",
			line: `level=info ready msg=x`,
			want: []pair{str("level", "info"), none("ready"), str("msg", "x")},
		},
		{
			name:       "non-ASCII prefix",
			line:       `сервис | a=1 b=2`,
			wantPrefix: `сервис |`,
			want:       []pair{str("a", "1"), str("b", "2")},
		},
		{
			name:       "NUL in the prefix and in a value",
			line:       "p\x00=1 a=x\x00y b=2",
			wantPrefix: "p\x00=1",
			want:       []pair{str("a", "x\x00y"), str("b", "2")},
		},
		{
			name:       "the prefix has no length limit, so prose that ends in two pairs is logfmt",
			line:       `took 5ms (retries=3, status=ok, code=5)`,
			wantPrefix: `took 5ms (retries=3,`,
			want:       []pair{str("status", "ok,"), str("code", "5)")},
		},
		{name: "CRLF between pairs", line: "a=1\r\nb=2\r", want: []pair{str("a", "1"), str("b", "2")}},
		{
			name: "text glued to a closing quote is a field without a value",
			line: `k="a"b x=1`,
			want: []pair{str("k", "a"), none("b"), str("x", "1")},
		},
		{
			name: "exactly at the threshold",
			line: `a=1 bc=2 xyz`, // 7 bytes of pairs out of 10
			want: []pair{str("a", "1"), str("bc", "2"), none("xyz")},
		},
		{
			name: "an unclosed quote is a field without a value",
			line: `level=info msg=started k="x y `,
			want: []pair{str("level", "info"), str("msg", "started"), none(`k="x y`)},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fields, prefix, ok := appendLogfmtFields(nil, slices.Clip([]byte(tt.line)))
			if !ok {
				t.Fatalf("appendLogfmtFields(%q) rejected a logfmt line", tt.line)
			}
			if string(prefix) != tt.wantPrefix || (tt.wantPrefix == "") != (prefix == nil) {
				t.Errorf("appendLogfmtFields(%q): prefix = %q (nil: %v), want %q", tt.line, prefix, prefix == nil, tt.wantPrefix)
			}
			if got := pairsOf(fields); !slices.Equal(got, tt.want) {
				t.Errorf("appendLogfmtFields(%q)\n got %+v\nwant %+v", tt.line, got, tt.want)
			}
		})
	}
}

func TestAppendLogfmtFieldsRejects(t *testing.T) {
	tests := []struct {
		name string
		line string
	}{
		{name: "empty line", line: ""},
		{name: "blank line", line: " \t "},
		{name: "plain text", line: "hello world"},
		{name: "one pair", line: "a=1"},
		{name: "one pair in a sentence", line: "Starting server on port=8080"},
		{name: "one pair and a bracketed one", line: "took 5ms (retries=3, status=ok)"},
		{name: "one byte under the threshold", line: "a=1 bc=2 wxyz"}, // 7 bytes of pairs out of 11
		{name: "two thirds of the bytes in pairs", line: "a=1 b=2 xyz"},
		{name: "a fraction of a percent under the threshold", line: "ab=1 cde=2 wxyz"}, // 9 bytes of pairs out of 13
		{name: "pairs scattered in a sentence", line: "user=bob logged in from ip=1.2.3.4 at noon"},
		{name: "URLs with queries", line: "http://x?a=1 http://y?b=2"},
		{name: "values without keys", line: "=v =w"},
		{name: "an unclosed quote swallows the pairs after it", line: `a=1 k="b c=2 d=3 e=4 f=5`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			kept := []entry.Field{{Key: []byte("kept")}}

			fields, prefix, ok := appendLogfmtFields(kept, slices.Clip([]byte(tt.line)))

			if ok {
				t.Errorf("appendLogfmtFields(%q) accepted the line as %+v with prefix %q", tt.line, pairsOf(fields[1:]), prefix)
			}
			if len(fields) != 1 || string(fields[0].Key) != "kept" {
				t.Errorf("appendLogfmtFields(%q) changed dst to %+v", tt.line, pairsOf(fields))
			}
			if prefix != nil {
				t.Errorf("appendLogfmtFields(%q) rejected the line but returned the prefix %q", tt.line, prefix)
			}
		})
	}
}

func TestAppendLogfmtFieldsAppends(t *testing.T) {
	dst := []entry.Field{{Key: []byte("kept")}}

	fields, _, ok := appendLogfmtFields(dst, []byte(`a=1 b=2`))

	want := []pair{{key: "kept"}, {"a", "1", entry.KindString, false}, {"b", "2", entry.KindString, false}}
	if got := pairsOf(fields); !ok || !slices.Equal(got, want) {
		t.Errorf("fields = %+v, ok = %v; want the old field followed by the new ones", got, ok)
	}
}

// A caller that reuses the slice must not pay for growing it on every rejected line.
func TestAppendLogfmtFieldsKeepsGrownCapacityOnReject(t *testing.T) {
	line := []byte("a=1 b=2 " + strings.Repeat("word ", 100))

	fields, _, ok := appendLogfmtFields(nil, line)

	if ok || len(fields) != 0 {
		t.Fatalf("got %d fields, ok = %v; want the line rejected", len(fields), ok)
	}
	if cap(fields) < 102 {
		t.Errorf("the rejected line left room for %d fields, want the 102 it was split into", cap(fields))
	}
}

// The prefix, keys and values are windows into the line, not copies: overwriting the line shows through them.
func TestAppendLogfmtFieldsPointsIntoTheLine(t *testing.T) {
	line := []byte(`svc | key=value msg="quoted text" bare n=123 tail`)
	fields, prefix, ok := appendLogfmtFields(nil, line)
	if !ok || len(prefix) == 0 || len(fields) != 5 {
		t.Fatalf("got %d fields with prefix %q, ok = %v; want 5 fields and a prefix", len(fields), prefix, ok)
	}

	for i := range line {
		line[i] = '#'
	}

	windows := make([][]byte, 0, 1+2*len(fields))
	windows = append(windows, prefix)
	for _, f := range fields {
		windows = append(windows, f.Key, f.Value.Raw)
	}
	for _, w := range windows {
		if want := strings.Repeat("#", len(w)); string(w) != want {
			t.Errorf("%q does not share memory with the line", w)
		}
		// An append to a window must not reach the bytes of the line that follow it.
		if cap(w) != len(w) {
			t.Errorf("a window of %d bytes has %d spare ones, want none", len(w), cap(w)-len(w))
		}
	}
}

func TestAppendLogfmtFieldsDoesNotAllocate(t *testing.T) {
	line := []byte(`2026/10/11 12:00:00 time=2026-10-11T12:00:00Z level=info msg="request \"done\"" status=200 path=/x ready`)
	dst := make([]entry.Field, 0, 16)

	allocs := testing.AllocsPerRun(100, func() {
		if _, _, ok := appendLogfmtFields(dst, line); !ok {
			t.Fatal("rejected a logfmt line")
		}
	})
	if allocs != 0 {
		t.Errorf("appendLogfmtFields allocates %v times per call, want 0", allocs)
	}

	// A rejected line must be as cheap.
	plain := []byte("took 5ms (retries=3, status=ok) and then some more plain text")
	if allocs := testing.AllocsPerRun(100, func() { _, _, _ = appendLogfmtFields(dst, plain) }); allocs != 0 {
		t.Errorf("rejecting a line allocates %v times per call, want 0", allocs)
	}
}
