package parse

import (
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/ai-fn/logpretty/internal/entry"
)

// describe renders everything Parse found, so that two entries can be compared and a difference read.
func describe(t *testing.T, e *entry.Entry) string {
	t.Helper()
	return fmt.Sprintf("format %d, prefix %q (collector: %v), fields %+v, roles %+v",
		e.Format, e.Prefix, e.CollectorPrefix, pairsOf(e.Fields), rolesOf(t, e))
}

func TestParse(t *testing.T) {
	str := func(key, raw string) pair { return pair{key, raw, entry.KindString, false} }
	num := func(key, raw string) pair { return pair{key, raw, entry.KindNumber, false} }

	tests := []struct {
		name       string
		line       string
		format     entry.Format
		wantPrefix string
		// A logfmt prefix is a collector's own only if it looks like one; the prefix of JSON always does.
		collector bool
		want      []pair
	}{
		{name: "JSON", line: `{"a":1,"b":"x"}`, format: entry.FormatJSON, want: []pair{num("a", "1"), str("b", "x")}},
		{name: "JSON in whitespace", line: " \t{\"a\":1} \r", format: entry.FormatJSON, want: []pair{num("a", "1")}},
		{name: "empty JSON object", line: `{}`, format: entry.FormatJSON},
		{
			name:   "JSON with pairs inside its strings",
			line:   `{"url":"http://x?a=1","q":"b=2 c=3"}`,
			format: entry.FormatJSON,
			want:   []pair{str("url", "http://x?a=1"), str("q", "b=2 c=3")},
		},
		{name: "logfmt", line: `a=1 b="x y"`, format: entry.FormatLogfmt, want: []pair{str("a", "1"), str("b", "x y")}},
		{
			name:       "logfmt after a prefix",
			line:       `2026/10/11 12:00:00 a=1 b=2`,
			format:     entry.FormatLogfmt,
			wantPrefix: `2026/10/11 12:00:00`,
			want:       []pair{str("a", "1"), str("b", "2")},
		},
		{name: "logfmt ending in a brace", line: `a=1 b=2}`, format: entry.FormatLogfmt, want: []pair{str("a", "1"), str("b", "2}")}},
		{
			name:       "logfmt after a word in braces",
			line:       `{x} a=1 b={y}`,
			format:     entry.FormatLogfmt,
			wantPrefix: `{x}`,
			want:       []pair{str("a", "1"), str("b", "{y}")},
		},
		{
			name:       "logfmt after an equals sign at the start",
			line:       `=x a=1 b=2`,
			format:     entry.FormatLogfmt,
			wantPrefix: `=x`,
			want:       []pair{str("a", "1"), str("b", "2")},
		},
		{
			name:       "logfmt after a container name",
			line:       `svc | level=info msg=x`,
			format:     entry.FormatLogfmt,
			wantPrefix: `svc |`,
			collector:  true,
			want:       []pair{str("level", "info"), str("msg", "x")},
		},
		{
			name:       "logfmt after a timestamp",
			line:       `2026-10-11T12:00:01.000000000Z level=info msg=hello a=1`,
			format:     entry.FormatLogfmt,
			wantPrefix: `2026-10-11T12:00:01.000000000Z`,
			collector:  true,
			want:       []pair{str("level", "info"), str("msg", "hello"), str("a", "1")},
		},
		{
			name:       "logfmt after a timestamp with a space in it",
			line:       `2026-10-11 12:00:00,123 a=1 b=2`,
			format:     entry.FormatLogfmt,
			wantPrefix: `2026-10-11 12:00:00,123`,
			collector:  true,
			want:       []pair{str("a", "1"), str("b", "2")},
		},
		{
			name:       "logfmt after a container name and a timestamp",
			line:       `svc-1  | 2026-10-11T12:00:00Z a=1 b=2`,
			format:     entry.FormatLogfmt,
			wantPrefix: `svc-1  | 2026-10-11T12:00:00Z`,
			collector:  true,
			want:       []pair{str("a", "1"), str("b", "2")},
		},
		{
			name:       "logfmt after a bar alone",
			line:       `| a=1 b=2`,
			format:     entry.FormatLogfmt,
			wantPrefix: `|`,
			collector:  true,
			want:       []pair{str("a", "1"), str("b", "2")},
		},
		{
			name:       "logfmt after a prefix of 64 bytes",
			line:       strings.Repeat("s", 63) + `| a=1 b=2`,
			format:     entry.FormatLogfmt,
			wantPrefix: strings.Repeat("s", 63) + `|`,
			collector:  true,
			want:       []pair{str("a", "1"), str("b", "2")},
		},
		{
			name:       "logfmt after a prefix of 65 bytes",
			line:       strings.Repeat("s", 64) + `| a=1 b=2`,
			format:     entry.FormatLogfmt,
			wantPrefix: strings.Repeat("s", 64) + `|`,
			want:       []pair{str("a", "1"), str("b", "2")},
		},
		{
			name:       "logfmt after a word between the bar and the pairs",
			line:       `svc | out a=1 b=2`,
			format:     entry.FormatLogfmt,
			wantPrefix: `svc | out`,
			want:       []pair{str("a", "1"), str("b", "2")},
		},
		{
			name:       "logfmt after a log of the logger itself",
			line:       `2026/10/11 12:00:01 INFO hello a=1 b=2`,
			format:     entry.FormatLogfmt,
			wantPrefix: `2026/10/11 12:00:01 INFO hello`,
			want:       []pair{str("a", "1"), str("b", "2")},
		},
		{
			name:       "logfmt after a tab behind the bar",
			line:       "svc |\t2026-10-11T12:00:00Z a=1 b=2",
			format:     entry.FormatLogfmt,
			wantPrefix: "svc |\t2026-10-11T12:00:00Z",
			collector:  true,
			want:       []pair{str("a", "1"), str("b", "2")},
		},
		{
			name:       "logfmt with a collector prefix after a byte order mark",
			line:       "\ufeffsvc | a=1 b=2",
			format:     entry.FormatLogfmt,
			wantPrefix: `svc |`,
			collector:  true,
			want:       []pair{str("a", "1"), str("b", "2")},
		},
		{
			name:       "logfmt after a timestamp and more text, as structlog writes it",
			line:       `2026-10-11T12:00:04.123456Z [info     ] user logged in [app] user_id=5 ip=1.2.3.4`,
			format:     entry.FormatLogfmt,
			wantPrefix: `2026-10-11T12:00:04.123456Z [info     ] user logged in [app]`,
			want:       []pair{str("user_id", "5"), str("ip", "1.2.3.4")},
		},
		{
			name:       "logfmt after a klog header",
			line:       `E1011 12:00:02.000003   12345 controller.go:10] "Failed to sync" err="timeout" key="ns/x"`,
			format:     entry.FormatLogfmt,
			wantPrefix: `E1011 12:00:02.000003   12345 controller.go:10] "Failed to sync"`,
			want:       []pair{str("err", "timeout"), str("key", "ns/x")},
		},
		{name: "plain", line: `hello world`},
		{name: "empty line", line: ``},
		{name: "blank line", line: " \t \r"},
		{name: "one pair in a sentence", line: `Starting server on port=8080`},

		{
			name:       "JSON after a timestamp",
			line:       `2026-10-11T12:00:00.123456789Z {"a":1}`,
			format:     entry.FormatJSON,
			wantPrefix: `2026-10-11T12:00:00.123456789Z`,
			want:       []pair{num("a", "1")},
		},
		{
			name:       "JSON after a timestamp with a space before its offset",
			line:       `2026-10-11 12:00:00 +0000 {"a":1}`,
			format:     entry.FormatJSON,
			wantPrefix: `2026-10-11 12:00:00 +0000`,
			want:       []pair{num("a", "1")},
		},
		{
			name:       "JSON after a container name and a timestamp with a space before its offset",
			line:       `web | 2026-10-11 12:00:00 +0300 {"a":1}`,
			format:     entry.FormatJSON,
			wantPrefix: `web | 2026-10-11 12:00:00 +0300`,
			want:       []pair{num("a", "1")},
		},
		{
			name:       "JSON after a timestamp with a space in it",
			line:       `2026-10-11 12:00:00,123 {"a":1}`,
			format:     entry.FormatJSON,
			wantPrefix: `2026-10-11 12:00:00,123`,
			want:       []pair{num("a", "1")},
		},
		{
			name:       "JSON after a container name",
			line:       `svc-1  | {"a":1}`,
			format:     entry.FormatJSON,
			wantPrefix: `svc-1  |`,
			want:       []pair{num("a", "1")},
		},
		{
			name:       "JSON after a timestamp and a container name",
			line:       `2026-10-11T12:00:00Z svc-1 | {"a":1}`,
			format:     entry.FormatJSON,
			wantPrefix: `2026-10-11T12:00:00Z svc-1 |`,
			want:       []pair{num("a", "1")},
		},
		{
			name:       "JSON after a container name and a timestamp",
			line:       `svc-1  | 2026-10-11T12:00:00.123456789Z {"a":1}`,
			format:     entry.FormatJSON,
			wantPrefix: `svc-1  | 2026-10-11T12:00:00.123456789Z`,
			want:       []pair{num("a", "1")},
		},
		{
			name:       "JSON after a container name and a timestamp with a space in it",
			line:       `svc-1 |2026-10-11 12:00:00 {"a":1}`,
			format:     entry.FormatJSON,
			wantPrefix: `svc-1 |2026-10-11 12:00:00`,
			want:       []pair{num("a", "1")},
		},
		{
			name:       "JSON after two bars",
			line:       `a | b | {"a":1}`,
			format:     entry.FormatJSON,
			wantPrefix: `a | b |`,
			want:       []pair{num("a", "1")},
		},
		{name: "JSON after a bar alone", line: `| {"a":1}`, format: entry.FormatJSON, wantPrefix: `|`, want: []pair{num("a", "1")}},
		{name: "a word between the bar and the timestamp", line: `svc | out 2026-10-11T12:00:00Z {"a":1}`},
		{name: "a word between the timestamp and JSON", line: `svc | 2026-10-11T12:00:00Z INFO {"a":1}`},
		{name: "a timestamp before the bar and a word after it", line: `2026-10-11T12:00:00Z | out {"a":1}`},
		{name: "a colored container name", line: "\x1b[36msvc-1  |\x1b[0m " + `{"a":1}`},
		{name: "a pod name in brackets", line: `[pod/web-1/app] {"a":1}`},
		{
			name:       "JSON right after the bar",
			line:       `  svc |{"a":1}  `,
			format:     entry.FormatJSON,
			wantPrefix: `svc |`,
			want:       []pair{num("a", "1")},
		},
		{
			name:       "prefix of 64 bytes",
			line:       strings.Repeat("s", 63) + `| {"a":1}`,
			format:     entry.FormatJSON,
			wantPrefix: strings.Repeat("s", 63) + `|`,
			want:       []pair{num("a", "1")},
		},
		{
			name:       "prefix of 64 bytes and the whitespace around it",
			line:       "  " + strings.Repeat("s", 63) + "|  \t" + `{"a":1}`,
			format:     entry.FormatJSON,
			wantPrefix: strings.Repeat("s", 63) + `|`,
			want:       []pair{num("a", "1")},
		},
		{name: "prefix of 65 bytes", line: strings.Repeat("s", 64) + `| {"a":1}`},
		{name: "prefix that is a word", line: `error: {"a":1}`},
		{name: "prefix that is a sentence", line: `failed to parse {"a":1}`},
		{name: "prefix that is a date without a time", line: `2026-10-11 {"a":1}`},
		{name: "prefix with the bar in the middle", line: `svc | out {"a":1}`},
		{name: "braces after a timestamp that are not JSON", line: `2026-10-11T12:00:00Z {foo}`},
		{name: "braces after a container name that are not JSON", line: `svc | {foo}`},
		{name: "text after JSON", line: `{"a":1} trailing`},
		{name: "two JSON objects", line: `{"a":1} {"b":2}`},
		{name: "braces that are not JSON", line: `{foo}`},
		{name: "JSON that is cut short", line: `{"a":1`},
		{name: "one brace too many", line: `{"a":1}}`},
		{name: "a closing brace only", line: `done}`},
		{name: "a JSON array", line: `[{"a":1}]`},

		{name: "JSON after a byte order mark", line: "\ufeff" + `{"a":1}`, format: entry.FormatJSON, want: []pair{num("a", "1")}},
		{
			name:   "logfmt after a byte order mark",
			line:   "\ufeff" + `a=1 b=2`,
			format: entry.FormatLogfmt,
			want:   []pair{str("a", "1"), str("b", "2")},
		},
		{
			name:       "prefixed JSON after a byte order mark",
			line:       "\ufeff" + `svc | {"a":1}`,
			format:     entry.FormatJSON,
			wantPrefix: `svc |`,
			want:       []pair{num("a", "1")},
		},
		{name: "plain after a byte order mark", line: "\ufeffhello"},
		{name: "two byte order marks", line: "\xef\xbb\xbf\xef\xbb\xbf" + `{"a":1}`},
		{name: "a part of a byte order mark", line: "\xef\xbb" + `{"a":1}`},
		{name: "a byte order mark alone", line: "\ufeff"},
		{name: "a byte order mark in the middle", line: `a=1 ` + "\ufeff" + `{"a":1}`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Clipped, so that a read past the end of the line panics instead of finding spare capacity.
			line := slices.Clip([]byte(tt.line))
			var p Parser

			e := p.Parse(line)

			if e.Format != tt.format {
				t.Errorf("Parse(%q): format = %d, want %d", tt.line, e.Format, tt.format)
			}
			if string(e.Prefix) != tt.wantPrefix || (tt.wantPrefix == "") != (e.Prefix == nil) {
				t.Errorf("Parse(%q): prefix = %q (nil: %v), want %q", tt.line, e.Prefix, e.Prefix == nil, tt.wantPrefix)
			}
			wantCollector := tt.collector || tt.format == entry.FormatJSON && tt.wantPrefix != ""
			if e.CollectorPrefix != wantCollector {
				t.Errorf("Parse(%q): CollectorPrefix = %v, want %v", tt.line, e.CollectorPrefix, wantCollector)
			}
			// An append to the prefix must not reach the bytes of the line that follow it.
			if cap(e.Prefix) != len(e.Prefix) {
				t.Errorf("Parse(%q): the prefix has %d spare bytes, want none", tt.line, cap(e.Prefix)-len(e.Prefix))
			}
			if got := pairsOf(e.Fields); !slices.Equal(got, tt.want) {
				t.Errorf("Parse(%q)\n got %+v\nwant %+v", tt.line, got, tt.want)
			}
			if string(e.Line) != tt.line || len(line) > 0 && &e.Line[0] != &line[0] {
				t.Errorf("Parse(%q): Line = %q, want the line itself", tt.line, e.Line)
			}
			if tt.format == entry.FormatPlain {
				if got := rolesOf(t, e); got != (roles{}) {
					t.Errorf("Parse(%q): a plain line has the roles %+v", tt.line, got)
				}
			}
		})
	}
}

func TestAppendJSONLineFieldsRejects(t *testing.T) {
	lines := []string{
		``, ` `, `hello`, `done}`, `{foo}`, `svc | {foo}`, `2026-10-11T12:00:00Z {foo}`, `error: {"a":1}`, `{"a":1} x`,
		strings.Repeat("s", 64) + `| {"a":1}`,
	}

	for _, line := range lines {
		kept := []entry.Field{{Key: []byte("kept")}}

		fields, prefix, ok := appendJSONLineFields(kept, slices.Clip([]byte(line)))

		if ok || prefix != nil || len(fields) != 1 || string(fields[0].Key) != "kept" {
			t.Errorf("appendJSONLineFields(%q) = %+v, prefix %q, %v; want dst as it was, no prefix and false",
				line, pairsOf(fields), prefix, ok)
		}
	}
}

func TestParseNil(t *testing.T) {
	var p Parser

	e := p.Parse(nil)

	if e.Format != entry.FormatPlain || e.Line != nil || e.Prefix != nil || len(e.Fields) != 0 {
		t.Errorf("Parse(nil) = %s, want an empty plain entry", describe(t, e))
	}
}

func TestParseAssignsRoles(t *testing.T) {
	tests := []struct {
		name string
		line string
		want roles
	}{
		{
			name: "JSON",
			line: `{"time":"2026-10-11T12:00:00Z","level":"warn","msg":"x","n":1}`,
			want: roles{"2026-10-11T12:00:00Z", entry.LevelWarn, "time", "level", "msg"},
		},
		{
			name: "logfmt",
			line: `svc | ts=1791720000 lvl=dbg msg=x`,
			want: roles{"2026-10-11T12:00:00Z", entry.LevelDebug, "ts", "lvl", "msg"},
		},
		{
			name: "prefixed JSON takes the time from a field, not from the prefix",
			line: `2026-10-11T12:00:00Z {"level":"error","message":"x"}`,
			want: roles{level: entry.LevelError, levelKey: "level", msgKey: "message"},
		},
		{
			name: "time without a zone is in the zone of the parser",
			line: `{"time":"2026-10-11 12:00:00","msg":"x"}`,
			want: roles{time: "2026-10-11T09:00:00Z", timeKey: "time", msgKey: "msg"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := Parser{Loc: testLoc}
			if got := rolesOf(t, p.Parse(slices.Clip([]byte(tt.line)))); got != tt.want {
				t.Errorf("roles of %q\n got %+v\nwant %+v", tt.line, got, tt.want)
			}
		})
	}
}

func TestParseWithoutLocationIsLocal(t *testing.T) {
	var p Parser

	e := p.Parse([]byte(`time="2026-10-11 12:00:00" msg=x`))

	if want := time.Date(2026, 10, 11, 12, 0, 0, 0, time.Local); !e.Time.Equal(want) {
		t.Errorf("Time = %v, want %v", e.Time, want)
	}
}

// One parser over lines of every format must give what a new parser gives for each: nothing of a line
// may leak into the next one.
func TestParseMixedStream(t *testing.T) {
	lines := []string{
		`{"time":"2026-10-11T12:00:00Z","level":"error","msg":"boom","a":1,"b":2,"c":3}`,
		`plain text`,
		`svc | level=info msg=started`,
		``,
		`2026-10-11T12:00:00Z {"n":1}`,
		`Starting server on port=8080`,
		`{"msg":"x"}`,
		`ts=1791720000 lvl=warn event=y extra`,
		`{}`,
		`svc | a=1 b=2`,
		`a=1 b=2`,
		`2026/10/11 12:00:00 a=1 b=2`,
		`a=1 b=2`,
		`took 5ms (retries=3, status=ok)`,
		`{"level":"debug"}`,
	}

	shared := Parser{Loc: testLoc}
	// Twice, so that every line also follows the last one.
	for range 2 {
		for _, line := range lines {
			fresh := Parser{Loc: testLoc}
			want := describe(t, fresh.Parse(slices.Clip([]byte(line))))
			if got := describe(t, shared.Parse(slices.Clip([]byte(line)))); got != want {
				t.Errorf("Parse(%q) after other lines\n got %s\nwant %s", line, got, want)
			}
		}
	}
}

// The entry and everything in it belong to the parser and to the line: both are reused for the next line.
func TestParseReusesEntry(t *testing.T) {
	var p Parser
	buf := []byte(`{"msg":"aaa"}`)

	first := p.Parse(buf)
	keptLine, keptValue := string(first.Line), string(first.Fields[0].Value.Raw)
	value := first.Fields[0].Value.Raw
	// The reader overwrites its buffer with the next line.
	copy(buf, `{"msg":"bbb"}`)
	second := p.Parse(buf)

	if second != first {
		t.Errorf("Parse returned another entry for the next line, want the same one reused")
	}
	if string(first.Line) != `{"msg":"bbb"}` || string(value) != "bbb" {
		t.Errorf("after the next line the old entry holds %q and its value %q, want them overwritten", first.Line, value)
	}
	if keptLine != `{"msg":"aaa"}` || keptValue != "aaa" {
		t.Errorf("copies taken before the next line are %q and %q, want them intact", keptLine, keptValue)
	}
}

// A plain line that looked like logfmt until its pairs were counted must not grow the fields again
// every time such a line comes.
func TestParseKeepsGrownFieldsAfterPlainLine(t *testing.T) {
	line := []byte("a=1 " + strings.Repeat("word ", 100))
	var p Parser
	if e := p.Parse(line); e.Format != entry.FormatPlain || len(e.Fields) != 0 {
		t.Fatalf("format = %d with %d fields, want a plain line", e.Format, len(e.Fields))
	}

	if allocs := testing.AllocsPerRun(100, func() { p.Parse(line) }); allocs != 0 {
		t.Errorf("Parse allocates %v times for a plain line it has seen before, want 0", allocs)
	}
}

func TestParseDoesNotAllocate(t *testing.T) {
	lines := []string{
		`{"time":"2026-10-11T12:00:00.123+03:00","level":"info","msg":"request done","status":200,"tags":["a","b"]}`,
		`2026-10-11T12:00:00.123456789Z {"level":"info","msg":"request done"}`,
		`svc-1  | {"level":"info","msg":"request done"}`,
		`svc-1  | level=info msg=started`,
		`2026-10-11T12:00:01.000000000Z level=info msg=hello a=1`,
		`2026/10/11 12:00:01 INFO hello a=1 b=2`,
		`time=2026-10-11T12:00:00Z level=info msg="request \"done\"" status=200`,
		`2026-10-11 12:00:00 INFO request done in 5ms`,
		`took 5ms (retries=3, status=ok) and then some more plain text`,
		`failed to parse {foo}`,
		"\xef\xbb\xbf" + `svc | 2026-10-11T12:00:00Z {"level":"info","msg":"request done"}`,
		``,
	}
	p := Parser{Loc: testLoc}
	for _, line := range lines {
		p.Parse([]byte(line)) // the first lines grow the fields of the entry
	}

	for _, line := range lines {
		if raceEnabled && strings.Contains(line, "{") {
			continue
		}
		b := []byte(line)
		if allocs := testing.AllocsPerRun(100, func() { p.Parse(b) }); allocs != 0 {
			t.Errorf("Parse allocates %v times for %q, want 0", allocs, line)
		}
	}
}
