package render

import (
	"hash/fnv"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/ai-fn/logpretty/internal/parse"
)

// traceColor is the color an id must get, by hash/fnv rather than the loop of the code.
func traceColor(th Theme, id string) string {
	h := fnv.New32a()
	_, _ = h.Write([]byte(id))
	return th.Traces[h.Sum32()%uint32(len(th.Traces))]
}

func TestTraceStyle(t *testing.T) {
	ids := []string{"", "4bf92f3577b34da6a3ce929d0e0e4736", "00f067aa0ba902b7", "1234567890123456789", "a b", "трасса", "\x00\xff"}
	for _, th := range []Theme{DefaultTheme(), {Traces: BasicTracePalette()}} {
		for _, id := range ids {
			if got, want := th.traceStyle([]byte(id)), traceColor(th, id); got != want {
				t.Errorf("traceStyle(%q) with %d colors = %q, want %q", id, len(th.Traces), got, want)
			}
		}
		// Every color is in use: the index takes the whole hash.
		used := map[string]bool{}
		for i := range 1000 {
			used[th.traceStyle(strconv.AppendInt(nil, int64(i), 10))] = true
		}
		if len(used) != len(th.Traces) {
			t.Errorf("1000 ids get %d of the %d colors", len(used), len(th.Traces))
		}
	}
	if got := (&Theme{}).traceStyle([]byte("4bf9")); got != "" {
		t.Errorf("traceStyle without colors = %q, want none", got)
	}
}

func TestAppendEntryTraceColor(t *testing.T) {
	th := tags
	th.Traces = []string{"<c0>", "<c1>", "<c2>", "<c3>", "<c4>"}
	// <c> in want stands for the color of id.
	tests := []struct {
		name     string
		fields   []string
		noExpand bool
		line, id string
		want     string
	}{
		{name: "JSON", line: `{"level":"info","msg":"x","trace_id":"4bf9"}`, id: "4bf9", want: "<l3>INFO</>  x  <k>trace_id=</><c>4bf9</>\n"},
		{name: "logfmt", line: `level=info msg=x traceID=4bf9`, id: "4bf9", want: "<l3>INFO</>  x  <k>traceID=</><c>4bf9</>\n"},
		{name: "a span id is a field like any other", line: `{"msg":"x","span_id":"s1","traceId":"4bf9"}`, id: "4bf9", want: "x  <k>span_id=</>s1 <k>traceId=</><c>4bf9</>\n"},
		{name: "the color of a Google Cloud trace is that of its id", line: `{"msg":"x","logging.googleapis.com/trace":"projects/p/traces/4bf9"}`, id: "4bf9", want: "x  <k>logging.googleapis.com/trace=</><c>projects/p/traces/4bf9</>\n"},
		{name: "a number of Datadog", line: `{"msg":"x","dd.trace_id":123}`, id: "123", want: "x  <k>dd.trace_id=</><c>123</>\n"},
		{name: "an id is decoded to pick its color", line: `{"msg":"x","trace_id":"4bf9"}`, id: "4bf9", want: "x  <k>trace_id=</><c>4bf9</>\n"},
		{name: "the quotes of an id are in its color", line: `{"msg":"x","trace_id":"a b"}`, id: "a b", want: "x  <k>trace_id=</><c>\"a b\"</>\n"},
		{name: "the first of two keys is not the id when it is empty", line: `{"trace_id":"","trace_id":"4bf9"}`, id: "4bf9", want: "<k>trace_id=</>\"\" <k>trace_id=</><c>4bf9</>\n"},
		{name: "an id in an object is not colored in the line", line: `{"msg":"x","trace":{"id":"4bf9"}}`, want: "x  <k>trace=</>{\"id\":\"4bf9\"}\n"},
		{name: "no id", line: `{"msg":"x","trace":"stack"}`, want: "x  <k>trace=</>stack\n"},
		{name: "an id with a line break is unfolded without color", line: `{"msg":"x","trace_id":"a\nb"}`, want: "x\n  <k>trace_id:</>\n    a\n    b\n"},
		{name: "an id with a line break kept in the line", noExpand: true, line: `{"msg":"x","trace_id":"a\nb"}`, id: "a\nb", want: "x  <k>trace_id=</><c>a\\nb</>\n"},

		{name: "fields: a nested id", fields: []string{"msg", "trace"}, line: `{"msg":"x","trace":{"id":"4bf9"}}`, id: "4bf9", want: "x  <k>trace=</><c>4bf9</>\n"},
		{name: "fields: an id in the scopes of .NET", fields: []string{"trace"}, line: `{"Scopes":[{"TraceId":"4bf9"}]}`, id: "4bf9", want: "<k>trace=</><c>4bf9</>\n"},
		{name: "fields: the trace and its own key", fields: []string{"trace", "trace_id"}, line: `{"trace_id":"4bf9"}`, id: "4bf9", want: "<k>trace=</><c>4bf9</> <k>trace_id=</><c>4bf9</>\n"},
		{name: "fields: the key of the id, after an empty one", fields: []string{"trace_id"}, line: `{"trace_id":"","trace_id":"4bf9"}`, want: "<k>trace_id=</>\"\"\n"},
		{name: "fields: a field trace that is not an id", fields: []string{"trace"}, line: `{"trace":"stack"}`, want: "<k>trace=</>stack\n"},
		{name: "fields: a span id", fields: []string{"span_id"}, line: `{"span_id":"s1","trace_id":"4bf9"}`, want: "<k>span_id=</>s1\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := Renderer{Theme: th, Loc: time.UTC, NoExpand: tt.noExpand, Fields: Select(tt.fields)}
			p := parse.Parser{Loc: time.UTC}
			got := string(r.AppendEntry(nil, p.Parse([]byte(tt.line))))
			want := tt.want
			if tt.id != "" {
				want = strings.ReplaceAll(want, "<c>", traceColor(th, tt.id))
			}
			if got != want {
				t.Errorf("AppendEntry(%q)\n got %q\nwant %q", tt.line, got, want)
			}
		})
	}
}

// One id gets one color in each line, under each of its keys and in each format.
func TestTraceColorIsTheSameEverywhere(t *testing.T) {
	const id = "4bf92f3577b34da6a3ce929d0e0e4736"
	th := DefaultTheme()
	want := "\x1b[2mtrace=\x1b[0m" + traceColor(th, id) + id + "\x1b[0m"
	r := Renderer{Theme: th, Loc: time.UTC, Fields: Select([]string{"trace"})}
	for _, line := range []string{
		`{"trace_id":"` + id + `"}`,
		`{"TraceId":"4bf92f3577b34da6a3ce929d0e0e4736","span_id":"00f067aa0ba902b7"}`,
		`a=1 traceID=` + id,
		`{"@tr":"` + id + `"}`,
		`{"logging.googleapis.com/trace":"projects/p/traces/` + id + `"}`,
		`{"trace":{"id":"` + id + `"}}`,
		`{"Scopes":["s",{"TraceId":"` + id + `"}]}`,
	} {
		if got := rendered(t, &r, line); got != want {
			t.Errorf("trace of %q\n got %q\nwant %q", line, got, want)
		}
	}
}
