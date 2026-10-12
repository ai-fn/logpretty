package parse

import (
	"slices"
	"testing"
	"time"

	"github.com/ai-fn/logpretty/internal/entry"
)

func TestFind(t *testing.T) {
	const nested = `{"http":{"status":502,"req":{"method":"GET","headers":{"accept":"*/*"}},"tags":["a","b"],"note":null},"user_id":42,"msg":"failed"}`

	tests := []struct {
		name     string
		line     string
		path     string
		want     string // Raw of the value
		wantKind entry.Kind
		missing  bool
	}{
		{name: "key of the entry", line: nested, path: "user_id", want: "42", wantKind: entry.KindNumber},
		{name: "string value", line: nested, path: "msg", want: "failed", wantKind: entry.KindString},
		{name: "object value", line: `{"a":{"b":1}}`, path: "a", want: `{"b":1}`, wantKind: entry.KindObject},
		{name: "one level down", line: nested, path: "http.status", want: "502", wantKind: entry.KindNumber},
		{name: "two levels down", line: nested, path: "http.req.method", want: "GET", wantKind: entry.KindString},
		{name: "three levels down", line: nested, path: "http.req.headers.accept", want: "*/*", wantKind: entry.KindString},
		{name: "object on the way", line: nested, path: "http.req.headers", want: `{"accept":"*/*"}`, wantKind: entry.KindObject},
		{name: "array value", line: nested, path: "http.tags", want: `["a","b"]`, wantKind: entry.KindArray},
		{name: "null value is a value", line: nested, path: "http.note", want: "null", wantKind: entry.KindNull},
		{name: "whitespace inside the object", line: "{\"a\" : {\t\"b\" :\r\n 1 } }", path: "a.b", want: "1", wantKind: entry.KindNumber},

		{name: "no such key", line: nested, path: "nope", missing: true},
		{name: "no such key one level down", line: nested, path: "http.nope", missing: true},
		{name: "the path goes on past a number", line: nested, path: "user_id.x", missing: true},
		{name: "the path goes on past a string", line: nested, path: "msg.x", missing: true},
		{name: "the path goes on past null", line: nested, path: "http.note.x", missing: true},
		{name: "arrays are not entered", line: nested, path: "http.tags.0", missing: true},
		{name: "a key is not matched by its beginning", line: nested, path: "user", missing: true},
		{name: "a key is not matched by its end", line: nested, path: "id", missing: true},
		{name: "the name of an object without a dot after it", line: nested, path: "httpstatus", missing: true},
		{name: "only a dot leads down", line: nested, path: "http/status", missing: true},
		{name: "another object with a name as long", line: `{"abcd":{"status":502}}`, path: "http.status", missing: true},
		{name: "letter case matters", line: nested, path: "User_ID", missing: true},
		{name: "empty path", line: nested, path: "", missing: true},
		{name: "dot at the end", line: nested, path: "http.", missing: true},
		{name: "dot at the start", line: nested, path: ".http", missing: true},
		{name: "two dots", line: nested, path: "http..status", missing: true},

		{name: "a key with a dot is found as it is written", line: `{"log.level":"info"}`, path: "log.level", want: "info", wantKind: entry.KindString},
		{name: "a key with a dot wins over the way down", line: `{"a":{"b":1},"a.b":2}`, path: "a.b", want: "2", wantKind: entry.KindNumber},
		{name: "a key with a dot on the way down", line: `{"a.b":{"c":1}}`, path: "a.b.c", want: "1", wantKind: entry.KindNumber},
		{name: "a key with a dot below", line: `{"a":{"b.c":1}}`, path: "a.b.c", want: "1", wantKind: entry.KindNumber},
		{name: "every split of the path is tried", line: `{"a":{"b":{"x":0}},"a.b":{"c":1}}`, path: "a.b.c", want: "1", wantKind: entry.KindNumber},
		{name: "empty key", line: `{"":1}`, path: "", want: "1", wantKind: entry.KindNumber},
		{name: "empty key below", line: `{"a":{"":1}}`, path: "a.", want: "1", wantKind: entry.KindNumber},

		{name: "the first of two keys alike", line: `{"x":1,"x":2}`, path: "x", want: "1", wantKind: entry.KindNumber},
		{name: "the first of two keys alike below", line: `{"a":{"x":1,"x":2}}`, path: "a.x", want: "1", wantKind: entry.KindNumber},
		{name: "the first object that has the rest of the path", line: `{"a":{"x":1},"a":{"b":2}}`, path: "a.b", want: "2", wantKind: entry.KindNumber},
		{name: "a value that is not an object does not stop the search", line: `{"a":1,"a":{"b":2}}`, path: "a.b", want: "2", wantKind: entry.KindNumber},

		{name: "key with an escape", line: `{"a\"b":1}`, path: `a"b`, want: "1", wantKind: entry.KindNumber},
		{name: "key with an escape below", line: `{"o":{"caf\u00e9":1}}`, path: "o.caf\u00e9", want: "1", wantKind: entry.KindNumber},
		{name: "value with an escape comes as written", line: `{"o":{"s":"a\nb"}}`, path: "o.s", want: `a\nb`, wantKind: entry.KindString},

		{name: "logfmt", line: `level=info msg="request done" status=200`, path: "status", want: "200", wantKind: entry.KindString},
		{name: "logfmt key with a dot", line: `level=info http.status=502 a=1`, path: "http.status", want: "502", wantKind: entry.KindString},
		{name: "logfmt has no way down", line: `level=info http={"status":502} a=1`, path: "http.status", missing: true},
		{name: "logfmt word without a value is not a key", line: `level=info msg=x done status=200`, path: "done", missing: true},
		{name: "logfmt key with an empty value", line: `level=info msg=x done= status=200`, path: "done", want: "", wantKind: entry.KindString},
		{name: "JSON after the prefix of a collector", line: `web-1 | {"http":{"status":502}}`, path: "http.status", want: "502", wantKind: entry.KindNumber},
		{name: "plain line", line: "panic: boom", path: "panic", missing: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := Parser{Loc: time.UTC}
			var f Finder
			// Clipped, so that a read past the end of the line panics instead of finding spare capacity.
			got, ok := f.Find(p.Parse(slices.Clip([]byte(tt.line))), []byte(tt.path))

			if ok == tt.missing {
				t.Fatalf("Find(%q) found = %v, want %v (value %q)", tt.path, ok, !tt.missing, got.Raw)
			}
			if string(got.Raw) != tt.want || got.Kind != tt.wantKind {
				t.Errorf("Find(%q) = %q of kind %v, want %q of kind %v", tt.path, got.Raw, got.Kind, tt.want, tt.wantKind)
			}
		})
	}
}

// What an earlier search left in the scratch space must not show in a later one.
func TestFindWithAUsedFinder(t *testing.T) {
	p := Parser{Loc: time.UTC}
	var f Finder
	lines := []string{
		`{"a":{"b":{"c":{"d":1}}},"z":{"y":2}}`,
		`{"a":{"b":7}}`,
		`{"z":{"y":{"x":3}}}`,
		`a=1 b=2`,
	}
	for range 2 {
		for _, line := range lines {
			e := p.Parse(slices.Clip([]byte(line)))
			for _, path := range []string{"a.b.c.d", "a.b", "z.y", "z.y.x", "a", "b", "nope.x"} {
				got, ok := f.Find(e, []byte(path))
				var fresh Finder
				want, wantOK := fresh.Find(e, []byte(path))
				if ok != wantOK || string(got.Raw) != string(want.Raw) || got.Kind != want.Kind {
					t.Errorf("%s: Find(%q) = %q, %v with a used Finder; %q, %v with a new one", line, path, got.Raw, ok, want.Raw, wantOK)
				}
				// Pairs left behind would pile up over a long stream.
				if len(f.scratch) != 0 {
					t.Fatalf("%s: Find(%q) left %d pairs in the scratch space", line, path, len(f.scratch))
				}
			}
		}
	}
}

func TestFindDoesNotAllocate(t *testing.T) {
	p := Parser{Loc: time.UTC}
	e := p.Parse([]byte(`{"level":"info","http":{"req":{"headers":{"accept":"*/*"}},"status":502},"user_id":42}`))
	var f Finder
	paths := [][]byte{[]byte("user_id"), []byte("http.status"), []byte("http.req.headers.accept"), []byte("http.req.nope"), []byte("nope")}
	for _, path := range paths {
		f.Find(e, path) // grows the scratch space once
	}

	if allocs := testing.AllocsPerRun(100, func() {
		for _, path := range paths {
			f.Find(e, path)
		}
	}); allocs != 0 {
		t.Errorf("Find allocates %v times per round, want 0", allocs)
	}
}
