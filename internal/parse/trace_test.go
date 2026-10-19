package parse

import (
	"slices"
	"testing"
	"time"

	"github.com/ai-fn/logpretty/internal/entry"
)

func TestTrace(t *testing.T) {
	tests := []struct {
		name    string
		line    string
		want    string // the id
		key     string // the key of the field it was found in, "" when it is nested
		number  bool
		missing bool
	}{
		{name: "OpenTelemetry", line: `{"msg":"x","trace_id":"4bf92f3577b34da6a3ce929d0e0e4736","span_id":"00f067aa0ba902b7"}`, want: "4bf92f3577b34da6a3ce929d0e0e4736", key: "trace_id"},
		{name: "traceId", line: `{"traceId":"abc"}`, want: "abc", key: "traceId"},
		{name: "TraceId", line: `{"TraceId":"abc"}`, want: "abc", key: "TraceId"},
		{name: "traceID", line: `{"traceID":"abc"}`, want: "abc", key: "traceID"},
		{name: "TRACE_ID", line: `{"TRACE_ID":"abc"}`, want: "abc", key: "TRACE_ID"},
		{name: "ECS with a dot", line: `{"trace.id":"abc"}`, want: "abc", key: "trace.id"},
		{name: "Datadog with a dot", line: `{"dd.trace_id":"1234567890123456789"}`, want: "1234567890123456789", key: "dd.trace_id"},
		{name: "Serilog", line: `{"@t":"2026-10-11T12:00:00Z","@mt":"x","@tr":"abc","@sp":"def"}`, want: "abc", key: "@tr"},
		{name: "GCP with the project", line: `{"logging.googleapis.com/trace":"projects/p/traces/abc"}`, want: "abc", key: "logging.googleapis.com/trace"},
		{name: "GCP bare", line: `{"logging.googleapis.com/trace":"abc"}`, want: "abc", key: "logging.googleapis.com/trace"},
		{name: "GCP with an escaped slash", line: `{"logging.googleapis.com/trace":"projects\/p\/traces\/abc"}`, want: "abc", key: "logging.googleapis.com/trace"},
		{name: "number", line: `{"dd.trace_id":1234567890123456789}`, want: "1234567890123456789", key: "dd.trace_id", number: true},
		{name: "nested number", line: `{"dd":{"trace_id":-1}}`, want: "-1", number: true},
		{name: "escaped string is decoded", line: `{"trace_id":"a\u0062c"}`, want: "abc", key: "trace_id"},
		{name: "key with an escape", line: `{"trace\u005fid":"abc"}`, want: "abc", key: "trace_id"},

		{name: "trace_id wins over traceId", line: `{"traceId":"b","trace_id":"a"}`, want: "a", key: "trace_id"},
		{name: "traceId wins over trace.id", line: `{"trace.id":"b","traceId":"a"}`, want: "a", key: "traceId"},
		{name: "@tr wins over GCP", line: `{"logging.googleapis.com/trace":"b","@tr":"a"}`, want: "a", key: "@tr"},
		{name: "the first of two keys alike", line: `{"trace_id":"a","trace_id":"b"}`, want: "a", key: "trace_id"},
		{name: "a key with a dot wins over the nested one", line: `{"trace":{"id":"b"},"trace.id":"a"}`, want: "a", key: "trace.id"},

		{name: "null gives way to the next key", line: `{"trace_id":null,"traceId":"a"}`, want: "a", key: "traceId"},
		{name: "empty string gives way", line: `{"trace_id":"","traceId":"a"}`, want: "a", key: "traceId"},
		{name: "object gives way", line: `{"trace_id":{"x":1},"traceId":"a"}`, want: "a", key: "traceId"},
		{name: "array gives way", line: `{"trace_id":["x"],"traceId":"a"}`, want: "a", key: "traceId"},
		{name: "bool gives way", line: `{"trace_id":true,"traceId":"a"}`, want: "a", key: "traceId"},
		{name: "GCP without an id gives way", line: `{"logging.googleapis.com/trace":"projects/p/traces/","trace.id":"a"}`, want: "a", key: "trace.id"},
		{name: "an escape is never empty", line: `{"trace_id":"\u0000","traceId":"a"}`, want: "\x00", key: "trace_id"},

		{name: "ECS nested", line: `{"trace":{"id":"abc"},"transaction":{"id":"x"}}`, want: "abc"},
		{name: "Datadog nested", line: `{"dd":{"trace_id":"123","span_id":"456"},"message":"x"}`, want: "123"},
		{name: "nested in any letter case", line: `{"Trace":{"ID":"abc"}}`, want: "abc"},
		{name: "nested null gives way", line: `{"trace":{"id":null},"dd":{"trace_id":"a"}}`, want: "a"},
		{name: "nested key with an escape", line: `{"tr\u0061ce":{"\u0069d":"abc"}}`, want: "abc"},
		{name: "nested with an escape", line: `{"trace":{"id":"a\/b"}}`, want: "a/b"},
		{name: "ECS before Datadog", line: `{"dd":{"trace_id":"b"},"trace":{"id":"a"}}`, want: "a"},
		{name: ".NET scopes", line: `{"LogLevel":"Information","Scopes":["outer",{"Message":"SpanId:1, TraceId:abc","SpanId":"1","TraceId":"abc","ParentId":"0"}]}`, want: "abc"},
		{name: "scopes in any letter case", line: `{"scopes":[{"traceid":"abc"}]}`, want: "abc"},
		{name: "scopes come last", line: `{"Scopes":[{"TraceId":"b"}],"dd":{"trace_id":"a"}}`, want: "a"},
		{name: "the first scope with an id", line: `{"Scopes":[{"TraceId":null},{"TraceId":"a"},{"TraceId":"b"}]}`, want: "a"},
		{name: "empty scopes", line: `{"Scopes":[]}`, missing: true},

		{name: "logfmt", line: `level=info msg=x trace_id=abc`, want: "abc", key: "trace_id"},
		{name: "logfmt traceID", line: `level=info msg=x traceID=abc`, want: "abc", key: "traceID"},
		{name: "logfmt quoted", line: `level=info msg=x trace_id="a\"b"`, want: `a"b`, key: "trace_id"},
		{name: "logfmt word without a value", line: `level=info msg=x trace_id a=1`, missing: true},
		{name: "logfmt object text is not entered", line: `level=info trace={"id":"abc"} a=1`, missing: true},
		{name: "after the prefix of a collector", line: `web-1 | {"trace_id":"abc"}`, want: "abc", key: "trace_id"},

		{name: "no id", line: `{"msg":"x","span_id":"abc","trace":"long text"}`, missing: true},
		{name: "a longer key", line: `{"trace_ids":"abc","xtrace_id":"abc"}`, missing: true},
		{name: "nested key under another name", line: `{"span":{"id":"abc"},"trace":{"trace_id":"abc"}}`, missing: true},
		{name: "deeper than one level", line: `{"ctx":{"trace":{"id":"abc"}}}`, missing: true},
		{name: "plain line", line: `trace_id=abc is in the text`, missing: true},
	}

	var used Finder
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := Parser{Loc: time.UTC}
			// Clipped, so that a read past the end of the line panics instead of finding spare capacity.
			e := p.Parse(slices.Clip([]byte(tt.line)))
			for _, f := range []*Finder{new(Finder), &used} {
				id, field, ok := f.Trace(e)
				if tt.missing {
					if ok || id.Raw != nil || id.Kind != entry.KindNone || field != -1 {
						t.Fatalf("Trace(%s) = %q, %d, %v; want nothing", tt.line, id.Raw, field, ok)
					}
					continue
				}
				if !ok || string(id.Raw) != tt.want || id.Escaped {
					t.Fatalf("Trace(%s) = %q (escaped %v), %v; want %q", tt.line, id.Raw, id.Escaped, ok, tt.want)
				}
				wantKind := entry.KindString
				if tt.number {
					wantKind = entry.KindNumber
				}
				if id.Kind != wantKind {
					t.Errorf("Trace(%s) is a %v; want a %v", tt.line, id.Kind, wantKind)
				}
				key := ""
				if field >= 0 {
					key = string(e.Fields[field].Key)
				}
				if key != tt.key {
					t.Errorf("Trace(%s) found it in %q (field %d); want %q", tt.line, key, field, tt.key)
				}
			}
		})
	}
}

func TestTraceDoesNotAllocate(t *testing.T) {
	lines := []string{
		`{"msg":"x","trace_id":"4bf92f3577b34da6a3ce929d0e0e4736"}`,
		`{"logging.googleapis.com/trace":"projects\/p\/traces\/abc"}`,
		`{"trace_id":"a\u0062c"}`,
		`{"dd":{"trace_id":"123"},"trace":{"id":null}}`,
		`{"Scopes":["outer",{"Message":"m","TraceId":"abc"}]}`,
		`{"msg":"no id","a":1,"b":{"c":2}}`,
	}
	// No escaped keys: the way down decodes them into memory of their own, as Find does.
	p := Parser{Loc: time.UTC}
	var f Finder
	for _, line := range lines {
		e := p.Parse([]byte(line))
		f.Trace(e) // the scratch space grows once
		if n := testing.AllocsPerRun(100, func() { f.Trace(e) }); n != 0 {
			t.Errorf("Trace(%s) allocates %v times", line, n)
		}
	}
}
