package parse

import (
	"bytes"
	"encoding/json"
	"slices"
	"testing"
	"unicode/utf8"

	"github.com/ai-fn/logpretty/internal/entry"
)

// refField is a pair as encoding/json sees it.
type refField struct {
	key        string // decoded
	value      string // exactly as written
	keyEscaped bool
}

// referenceJSONFields is the obvious implementation appendJSONFields must agree with.
func referenceJSONFields(t *testing.T, line []byte) (fields []refField, ok bool) {
	t.Helper()
	if !json.Valid(line) {
		return nil, false
	}
	dec := json.NewDecoder(bytes.NewReader(line))
	if tok, err := dec.Token(); err != nil || tok != json.Delim('{') {
		return nil, false
	}
	for dec.More() {
		start := dec.InputOffset()
		key, err := dec.Token()
		if err != nil {
			t.Fatalf("reference: reading a key of %q: %v", line, err)
		}
		// Between two tokens only the key itself can hold a backslash.
		keyEscaped := bytes.IndexByte(line[start:dec.InputOffset()], '\\') >= 0
		var value json.RawMessage
		if err := dec.Decode(&value); err != nil {
			t.Fatalf("reference: reading a value of %q: %v", line, err)
		}
		fields = append(fields, refField{key: key.(string), value: string(value), keyEscaped: keyEscaped})
	}
	return fields, true
}

func FuzzJSON(f *testing.F) {
	for _, seed := range []string{
		`{}`, ` { } `, `{"msg":"hello"}`, `{"z":1,"a":2}`, `{"id":12345678901234567891,"e":1e400}`,
		`{"s":"x","n":-1.5e-3,"t":true,"f":false,"z":null,"o":{"a":[1,{"b":"}"}]},"a":[]}`,
		`{"a":"say \"hi\"\n","b":"\\","c":"\\\""}`, `{"a\"b":1,"caf\u00e9":2,"":3}`, `{"a":1,"a":2}`,
		"\t{ \"a\" :\t1 ,\r\n \"b\" : [ 1 , 2 ] }\r\n", `{"a":1} x`, `{"a":1}{"b":2}`, `[1]`, `{"a":1,}`, `{'a':1}`,
		"{\"a\":\"x\xffy\",\"\xfe\":1}", "\u00a0{\"a\":1}", `{"a":"\ud83d\ude00","\ud83d":"\ude00"}`, `{`, ``,
	} {
		f.Add([]byte(seed))
	}

	f.Fuzz(func(t *testing.T, line []byte) {
		// A copy, because the target overwrites the line below and must not touch the engine's data.
		// Clipped, so that a read past the end panics instead of finding spare capacity.
		line = slices.Clip(slices.Clone(line))
		want, wantOK := referenceJSONFields(t, line)

		fields, ok := appendJSONFields(nil, line)

		if ok != wantOK {
			t.Fatalf("appendJSONFields(%q) accepted = %v, encoding/json says %v", line, ok, wantOK)
		}
		if !ok {
			if len(fields) != 0 {
				t.Fatalf("appendJSONFields(%q) rejected the line but returned %d fields", line, len(fields))
			}
			return
		}
		if len(fields) != len(want) {
			t.Fatalf("appendJSONFields(%q) found %d fields, encoding/json finds %d", line, len(fields), len(want))
		}
		for i, field := range fields {
			key := string(field.Key)
			if !utf8.Valid(field.Key) {
				key = string([]rune(key)) // encoding/json replaces every invalid byte with U+FFFD
			}
			if key != want[i].key {
				t.Fatalf("appendJSONFields(%q): key %d = %q, encoding/json gives %q", line, i, key, want[i].key)
			}
			value := string(field.Value.Raw)
			if field.Value.Kind == entry.KindString {
				value = `"` + value + `"`
			}
			if value != want[i].value {
				t.Fatalf("appendJSONFields(%q): value %d = %q, encoding/json gives %q", line, i, value, want[i].value)
			}
			if escaped := bytes.IndexByte(field.Value.Raw, '\\') >= 0; field.Value.Kind == entry.KindString && field.Value.Escaped != escaped {
				t.Fatalf("appendJSONFields(%q): value %d has Escaped = %v, want %v", line, i, field.Value.Escaped, escaped)
			}
			if kind := kindOf(want[i].value); field.Value.Kind != kind {
				t.Fatalf("appendJSONFields(%q): value %d has kind %d, want %d", line, i, field.Value.Kind, kind)
			}
		}

		// Fields are windows into the line: flipping every byte of the line must show through them.
		before := pairsOf(fields)
		for i := range line {
			line[i] ^= 0xff
		}
		for i, field := range fields {
			// A key with escapes is the one thing that is decoded into memory of its own.
			if !want[i].keyEscaped && !isFlipped(field.Key, before[i].key) {
				t.Fatalf("key %d (%q) is not a clipped window into the line", i, before[i].key)
			}
			if !isFlipped(field.Value.Raw, before[i].raw) {
				t.Fatalf("value %d (%q) is not a clipped window into the line", i, before[i].raw)
			}
		}
	})
}

// isFlipped reports whether got is before with every byte inverted, and has no spare capacity.
func isFlipped(got []byte, before string) bool {
	if len(got) != len(before) || cap(got) != len(got) {
		return false
	}
	for i := range got {
		if got[i] != before[i]^0xff {
			return false
		}
	}
	return true
}

// kindOf names the kind of a JSON value by its first byte, as the reference for the fuzz target.
func kindOf(value string) entry.Kind {
	switch value[0] {
	case '"':
		return entry.KindString
	case '{':
		return entry.KindObject
	case '[':
		return entry.KindArray
	case 't', 'f':
		return entry.KindBool
	case 'n':
		return entry.KindNull
	default:
		return entry.KindNumber
	}
}
