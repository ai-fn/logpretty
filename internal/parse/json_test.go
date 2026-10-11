package parse

import (
	"bytes"
	"encoding/json"
	"slices"
	"strings"
	"testing"

	"github.com/ai-fn/logpretty/internal/entry"
)

// pair is a field in a form that is easy to write and compare in tests.
type pair struct {
	key     string
	raw     string
	kind    entry.Kind
	escaped bool
}

func pairsOf(fields []entry.Field) []pair {
	out := make([]pair, 0, len(fields))
	for _, f := range fields {
		out = append(out, pair{string(f.Key), string(f.Value.Raw), f.Value.Kind, f.Value.Escaped})
	}
	return out
}

func TestAppendJSONFields(t *testing.T) {
	str := func(key, raw string) pair { return pair{key, raw, entry.KindString, false} }
	num := func(key, raw string) pair { return pair{key, raw, entry.KindNumber, false} }

	tests := []struct {
		name string
		line string
		want []pair
	}{
		{name: "empty object", line: `{}`, want: []pair{}},
		{name: "empty object with spaces", line: " {  } ", want: []pair{}},
		{name: "one string", line: `{"msg":"hello"}`, want: []pair{str("msg", "hello")}},
		{
			name: "keys keep their order",
			line: `{"z":1,"a":2,"m":3}`,
			want: []pair{num("z", "1"), num("a", "2"), num("m", "3")},
		},
		{
			name: "numbers stay as written",
			line: `{"id":12345678901234567891,"huge":1e400,"neg":-0,"frac":1.50,"exp":2E+3}`,
			want: []pair{num("id", "12345678901234567891"), num("huge", "1e400"), num("neg", "-0"), num("frac", "1.50"), num("exp", "2E+3")},
		},
		{
			name: "every kind",
			line: `{"s":"x","n":1,"t":true,"f":false,"z":null,"o":{"a":1},"a":[1,2]}`,
			want: []pair{
				str("s", "x"),
				num("n", "1"),
				{"t", "true", entry.KindBool, false},
				{"f", "false", entry.KindBool, false},
				{"z", "null", entry.KindNull, false},
				{"o", `{"a":1}`, entry.KindObject, false},
				{"a", `[1,2]`, entry.KindArray, false},
			},
		},
		{
			name: "nested values are kept whole",
			line: `{"http":{"req":{"hdr":["a","b"]},"status":200},"tags":[{"k":"v"},[1,[2]]],"n":1}`,
			want: []pair{
				{"http", `{"req":{"hdr":["a","b"]},"status":200}`, entry.KindObject, false},
				{"tags", `[{"k":"v"},[1,[2]]]`, entry.KindArray, false},
				num("n", "1"),
			},
		},
		{
			name: "structural characters inside strings",
			line: `{"a":"x,y","b":"{not}","c":"[no]","d":"a:b","e":"} , {"}`,
			want: []pair{str("a", "x,y"), str("b", "{not}"), str("c", "[no]"), str("d", "a:b"), str("e", "} , {")},
		},
		{
			name: "structural characters inside nested strings",
			line: `{"o":{"k":"}]\"}"},"a":["]","\\"],"n":1}`,
			want: []pair{
				{"o", `{"k":"}]\"}"}`, entry.KindObject, false},
				{"a", `["]","\\"]`, entry.KindArray, false},
				num("n", "1"),
			},
		},
		{
			name: "escapes in a string value are kept raw and marked",
			line: `{"msg":"say \"hi\"\n","path":"C:\\","plain":"x"}`,
			want: []pair{{"msg", `say \"hi\"\n`, entry.KindString, true}, {"path", `C:\\`, entry.KindString, true}, str("plain", "x")},
		},
		{
			name: "a string ending in an escaped backslash",
			line: `{"a":"\\","b":"\\\\","c":"\\\"","d":1}`,
			want: []pair{{"a", `\\`, entry.KindString, true}, {"b", `\\\\`, entry.KindString, true}, {"c", `\\\"`, entry.KindString, true}, num("d", "1")},
		},
		{name: "empty string and empty key", line: `{"":"","k":""}`, want: []pair{str("", ""), str("k", "")}},
		{
			name: "an escaped key is decoded",
			line: `{"a\"b":1,"tab\there":2,"caf\u00e9":3}`,
			want: []pair{num(`a"b`, "1"), num("tab\there", "2"), num("café", "3")},
		},
		{name: "duplicate keys are both kept", line: `{"a":1,"a":2}`, want: []pair{num("a", "1"), num("a", "2")}},
		{name: "non-ASCII keys and values", line: `{"ключ":"значение","emoji":"😀"}`, want: []pair{str("ключ", "значение"), str("emoji", "😀")}},
		{
			name: "whitespace between tokens",
			line: "\t{ \"a\" :\t1 ,\r\n \"b\" : \"x\" , \"c\" : [ 1 , 2 ] , \"d\" : null }\r\n",
			want: []pair{num("a", "1"), str("b", "x"), {"c", "[ 1 , 2 ]", entry.KindArray, false}, {"d", "null", entry.KindNull, false}},
		},
		{
			name: "a scalar right before the closing brace",
			line: `{"a":1,"b":true,"c":null}`,
			want: []pair{num("a", "1"), {"b", "true", entry.KindBool, false}, {"c", "null", entry.KindNull, false}},
		},
		{
			name: "every kind of whitespace ends a scalar",
			line: "{\"a\":1\t,\"b\":true\n,\"c\":null\r,\"d\":2 ,\"e\":3\t}",
			want: []pair{num("a", "1"), {"b", "true", entry.KindBool, false}, {"c", "null", entry.KindNull, false}, num("d", "2"), num("e", "3")},
		},
		{name: "whitespace after the object", line: "{\"a\":1} \t\r\n", want: []pair{num("a", "1")}},
		{name: "invalid UTF-8 in a string is kept", line: "{\"a\":\"x\xffy\"}", want: []pair{str("a", "x\xffy")}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Clipped, so that a read past the end of the line panics instead of finding spare capacity.
			fields, ok := appendJSONFields(nil, slices.Clip([]byte(tt.line)))
			if !ok {
				t.Fatalf("appendJSONFields(%q) rejected a JSON object", tt.line)
			}
			if got := pairsOf(fields); !slices.Equal(got, tt.want) {
				t.Errorf("appendJSONFields(%q)\n got %+v\nwant %+v", tt.line, got, tt.want)
			}
		})
	}
}

func TestAppendJSONFieldsRejects(t *testing.T) {
	tests := []struct {
		name string
		line string
	}{
		{name: "empty line", line: ""},
		{name: "blank line", line: "  \t "},
		{name: "plain text", line: "server started"},
		{name: "text after the object", line: `{"id":1} and some text`},
		{name: "text before the object", line: `prefix {"id":1}`},
		{name: "two objects", line: `{"a":1}{"b":2}`},
		{name: "two objects with a space", line: `{"a":1} {"b":2}`},
		{name: "cut off in a value", line: `{"a":"x`},
		{name: "cut off after a comma", line: `{"a":1,`},
		{name: "only an opening brace", line: `{`},
		{name: "only a closing brace", line: `}`},
		{name: "braces around text", line: `{not json}`},
		{name: "array", line: `[{"a":1}]`},
		{name: "string", line: `"{}"`},
		{name: "number", line: `42`},
		{name: "literal", line: `null`},
		{name: "single quotes", line: `{'a':1}`},
		{name: "unquoted key", line: `{a:1}`},
		{name: "trailing comma", line: `{"a":1,}`},
		{name: "key without a value", line: `{"a"}`},
		{name: "missing comma", line: `{"a":1 "b":2}`},
		{name: "bad number", line: `{"a":01}`},
		{name: "bad literal", line: `{"a":nul}`},
		{name: "bad escape", line: `{"a":"\x"}`},
		{name: "raw control character in a string", line: "{\"a\":\"x\ty\"}"},
		{name: "unbalanced nesting", line: `{"a":[1,2}`},
		// encoding/json does not count these as whitespace, so neither does this package.
		{name: "non-breaking space before the object", line: "\u00a0{\"a\":1}"},
		{name: "byte order mark before the object", line: "\ufeff{\"a\":1}"},
		{name: "vertical tab before the object", line: "\v{\"a\":1}"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			kept := []entry.Field{{Key: []byte("kept")}}

			fields, ok := appendJSONFields(kept, slices.Clip([]byte(tt.line)))

			if ok {
				t.Errorf("appendJSONFields(%q) accepted the line as %+v", tt.line, pairsOf(fields[1:]))
			}
			if len(fields) != 1 || string(fields[0].Key) != "kept" {
				t.Errorf("appendJSONFields(%q) changed dst to %+v", tt.line, pairsOf(fields))
			}
		})
	}
}

func TestAppendJSONFieldsAppends(t *testing.T) {
	dst := []entry.Field{{Key: []byte("kept")}}

	fields, ok := appendJSONFields(dst, []byte(`{"a":1}`))

	if want := []string{"kept", "a"}; !ok || len(fields) != 2 || string(fields[0].Key) != want[0] || string(fields[1].Key) != want[1] {
		t.Errorf("fields = %+v, ok = %v; want the old field followed by the new one", pairsOf(fields), ok)
	}
}

// Keys and values are windows into the line, not copies: overwriting the line shows through them.
func TestAppendJSONFieldsPointsIntoTheLine(t *testing.T) {
	line := []byte(`{"key":"value","n":123,"o":{"a":[1]},"a":[1,2],"t":true}`)
	fields, ok := appendJSONFields(nil, line)
	if !ok {
		t.Fatal("rejected a JSON object")
	}

	for i := range line {
		line[i] = '#'
	}

	for _, f := range fields {
		if want := strings.Repeat("#", len(f.Key)); string(f.Key) != want {
			t.Errorf("key = %q, want it to share memory with the line", f.Key)
		}
		if want := strings.Repeat("#", len(f.Value.Raw)); string(f.Value.Raw) != want {
			t.Errorf("value = %q, want it to share memory with the line", f.Value.Raw)
		}
		// An append to a key or a value must not reach the bytes of the line that follow it.
		if cap(f.Key) != len(f.Key) || cap(f.Value.Raw) != len(f.Value.Raw) {
			t.Errorf("key has %d spare bytes and value %d, want none", cap(f.Key)-len(f.Key), cap(f.Value.Raw)-len(f.Value.Raw))
		}
	}
}

// encoding/json limits the nesting depth. Whatever it decides, the splitter agrees and does not recurse.
func TestAppendJSONFieldsDeepNesting(t *testing.T) {
	for _, depth := range []int{100, 9_000, 20_000} {
		nested := strings.Repeat("[", depth) + strings.Repeat("]", depth)
		line := slices.Clip([]byte(`{"a":` + nested + `,"b":1}`))

		fields, ok := appendJSONFields(nil, line)

		if want := json.Valid(line); ok != want {
			t.Fatalf("depth %d: accepted = %v, encoding/json says %v", depth, ok, want)
		}
		if depth == 100 && !ok {
			t.Fatalf("depth %d: rejected a JSON object", depth)
		}
		if ok && (len(fields) != 2 || string(fields[0].Value.Raw) != nested || string(fields[1].Key) != "b") {
			t.Errorf("depth %d: got %d fields, want the nested array whole and then b", depth, len(fields))
		}
	}
}

func TestAppendJSONFieldsDoesNotAllocate(t *testing.T) {
	dst := make([]entry.Field, 0, 16)

	plain := bytes.Repeat([]byte("plain text "), 10)
	if allocs := testing.AllocsPerRun(100, func() { _, _ = appendJSONFields(dst, plain) }); allocs != 0 {
		t.Errorf("rejecting a line allocates %v times per call, want 0", allocs)
	}

	if raceEnabled {
		t.Skip("json.Valid allocates under the race detector")
	}
	line := []byte(`{"time":"2026-10-11T12:00:00Z","level":"info","msg":"request \"done\"","status":200,"http":{"path":"/x"},"tags":["a"]}`)
	allocs := testing.AllocsPerRun(100, func() {
		if _, ok := appendJSONFields(dst, line); !ok {
			t.Fatal("rejected a JSON object")
		}
	})
	if allocs != 0 {
		t.Errorf("appendJSONFields allocates %v times per call, want 0", allocs)
	}
}
