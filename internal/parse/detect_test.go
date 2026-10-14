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
	return fmt.Sprintf("format %d, collector %q (source %q), stamp %q, prefix %q, fields %+v, roles %+v",
		e.Format, e.Collector, e.Source, e.Stamp, e.Prefix, pairsOf(e.Fields), rolesOf(t, e))
}

func TestParse(t *testing.T) {
	str := func(key, raw string) pair { return pair{key, raw, entry.KindString, false} }
	num := func(key, raw string) pair { return pair{key, raw, entry.KindNumber, false} }

	tests := []struct {
		name          string
		line          string
		format        entry.Format
		wantCollector string
		wantSource    string
		wantStamp     string
		wantPrefix    string // the logger's own text before logfmt pairs
		want          []pair
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
			name:          "logfmt after a container name",
			line:          `svc | level=info msg=x`,
			format:        entry.FormatLogfmt,
			wantCollector: `svc |`,
			wantSource:    `svc`,
			want:          []pair{str("level", "info"), str("msg", "x")},
		},
		{
			name:      "logfmt after a timestamp",
			line:      `2026-10-11T12:00:01.000000000Z level=info msg=hello a=1`,
			format:    entry.FormatLogfmt,
			wantStamp: `2026-10-11T12:00:01.000000000Z`,
			want:      []pair{str("level", "info"), str("msg", "hello"), str("a", "1")},
		},
		{
			name:      "logfmt after a timestamp with a space in it",
			line:      `2026-10-11 12:00:00,123 a=1 b=2`,
			format:    entry.FormatLogfmt,
			wantStamp: `2026-10-11 12:00:00,123`,
			want:      []pair{str("a", "1"), str("b", "2")},
		},
		{
			name:          "logfmt after a container name and a timestamp",
			line:          `svc-1  | 2026-10-11T12:00:00Z a=1 b=2`,
			format:        entry.FormatLogfmt,
			wantCollector: `svc-1  |`,
			wantSource:    `svc-1`,
			wantStamp:     `2026-10-11T12:00:00Z`,
			want:          []pair{str("a", "1"), str("b", "2")},
		},
		{
			name:          "logfmt after a bar alone",
			line:          `| a=1 b=2`,
			format:        entry.FormatLogfmt,
			wantCollector: `|`,
			want:          []pair{str("a", "1"), str("b", "2")},
		},
		{
			name:          "logfmt after a prefix of 64 bytes",
			line:          strings.Repeat("s", 63) + `| a=1 b=2`,
			format:        entry.FormatLogfmt,
			wantCollector: strings.Repeat("s", 63) + `|`,
			wantSource:    strings.Repeat("s", 63),
			want:          []pair{str("a", "1"), str("b", "2")},
		},
		{
			name:       "logfmt after a prefix of 65 bytes",
			line:       strings.Repeat("s", 64) + `| a=1 b=2`,
			format:     entry.FormatLogfmt,
			wantPrefix: strings.Repeat("s", 64) + `|`,
			want:       []pair{str("a", "1"), str("b", "2")},
		},
		{
			name:          "logfmt after a word between the bar and the pairs",
			line:          `svc | out a=1 b=2`,
			format:        entry.FormatLogfmt,
			wantCollector: `svc |`,
			wantSource:    `svc`,
			wantPrefix:    `out`,
			want:          []pair{str("a", "1"), str("b", "2")},
		},
		{
			name:       "logfmt after a log of the logger itself",
			line:       `2026/10/11 12:00:01 INFO hello a=1 b=2`,
			format:     entry.FormatLogfmt,
			wantPrefix: `2026/10/11 12:00:01 INFO hello`,
			want:       []pair{str("a", "1"), str("b", "2")},
		},
		{
			name:          "logfmt after a tab behind the bar",
			line:          "svc |\t2026-10-11T12:00:00Z a=1 b=2",
			format:        entry.FormatLogfmt,
			wantCollector: `svc |`,
			wantSource:    `svc`,
			wantStamp:     `2026-10-11T12:00:00Z`,
			want:          []pair{str("a", "1"), str("b", "2")},
		},
		{
			name:          "logfmt with a collector prefix after a byte order mark",
			line:          "\ufeffsvc | a=1 b=2",
			format:        entry.FormatLogfmt,
			wantCollector: `svc |`,
			wantSource:    `svc`,
			want:          []pair{str("a", "1"), str("b", "2")},
		},
		{
			name:       "logfmt after a timestamp and more text, as structlog writes it",
			line:       `2026-10-11T12:00:04.123456Z [info     ] user logged in [app] user_id=5 ip=1.2.3.4`,
			format:     entry.FormatLogfmt,
			wantStamp:  `2026-10-11T12:00:04.123456Z`,
			wantPrefix: `[info     ] user logged in [app]`,
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
			name:      "JSON after a timestamp",
			line:      `2026-10-11T12:00:00.123456789Z {"a":1}`,
			format:    entry.FormatJSON,
			wantStamp: `2026-10-11T12:00:00.123456789Z`,
			want:      []pair{num("a", "1")},
		},
		{
			name:      "JSON after a timestamp with a space before its offset",
			line:      `2026-10-11 12:00:00 +0000 {"a":1}`,
			format:    entry.FormatJSON,
			wantStamp: `2026-10-11 12:00:00 +0000`,
			want:      []pair{num("a", "1")},
		},
		{
			name:          "JSON after a container name and a timestamp with a space before its offset",
			line:          `web | 2026-10-11 12:00:00 +0300 {"a":1}`,
			format:        entry.FormatJSON,
			wantCollector: `web |`,
			wantSource:    `web`,
			wantStamp:     `2026-10-11 12:00:00 +0300`,
			want:          []pair{num("a", "1")},
		},
		{
			name:      "JSON after a timestamp with a space in it",
			line:      `2026-10-11 12:00:00,123 {"a":1}`,
			format:    entry.FormatJSON,
			wantStamp: `2026-10-11 12:00:00,123`,
			want:      []pair{num("a", "1")},
		},
		{
			name:          "JSON after a container name",
			line:          `svc-1  | {"a":1}`,
			format:        entry.FormatJSON,
			wantCollector: `svc-1  |`,
			wantSource:    `svc-1`,
			want:          []pair{num("a", "1")},
		},
		{
			name:          "JSON after a timestamp and a container name",
			line:          `2026-10-11T12:00:00Z svc-1 | {"a":1}`,
			format:        entry.FormatJSON,
			wantCollector: `2026-10-11T12:00:00Z svc-1 |`,
			wantSource:    `svc-1`,
			want:          []pair{num("a", "1")},
		},
		{
			name:          "JSON after a container name and a timestamp",
			line:          `svc-1  | 2026-10-11T12:00:00.123456789Z {"a":1}`,
			format:        entry.FormatJSON,
			wantCollector: `svc-1  |`,
			wantSource:    `svc-1`,
			wantStamp:     `2026-10-11T12:00:00.123456789Z`,
			want:          []pair{num("a", "1")},
		},
		{
			name:          "JSON after a container name and a timestamp with a space in it",
			line:          `svc-1 |2026-10-11 12:00:00 {"a":1}`,
			format:        entry.FormatJSON,
			wantCollector: `svc-1 |`,
			wantSource:    `svc-1`,
			wantStamp:     `2026-10-11 12:00:00`,
			want:          []pair{num("a", "1")},
		},
		{
			name:          "JSON after two bars",
			line:          `a | b | {"a":1}`,
			format:        entry.FormatJSON,
			wantCollector: `a | b |`,
			wantSource:    `a`,
			want:          []pair{num("a", "1")},
		},
		{name: "JSON after a bar alone", line: `| {"a":1}`, format: entry.FormatJSON, wantCollector: `|`, want: []pair{num("a", "1")}},
		{name: "a word between the bar and the timestamp", line: `svc | out 2026-10-11T12:00:00Z {"a":1}`, wantCollector: `svc |`, wantSource: `svc`},
		{
			name:          "a word between the timestamp and JSON",
			line:          `svc | 2026-10-11T12:00:00Z INFO {"a":1}`,
			wantCollector: `svc |`,
			wantSource:    `svc`,
			wantStamp:     `2026-10-11T12:00:00Z`,
		},
		{name: "a timestamp before the bar and a word after it", line: `2026-10-11T12:00:00Z | out {"a":1}`, wantStamp: `2026-10-11T12:00:00Z`},
		{name: "a colored container name", line: "\x1b[36msvc-1  |\x1b[0m " + `{"a":1}`},
		{
			name:          "JSON after a pod name in brackets",
			line:          `[pod/web-1/app] {"a":1}`,
			format:        entry.FormatJSON,
			wantCollector: `[pod/web-1/app]`,
			wantSource:    `pod/web-1/app`,
			want:          []pair{num("a", "1")},
		},
		{
			name:          "JSON after a pod name in brackets and a timestamp",
			line:          `[pod/web-1/app] 2026-10-11T12:00:04.000000000Z {"a":1}`,
			format:        entry.FormatJSON,
			wantCollector: `[pod/web-1/app]`,
			wantSource:    `pod/web-1/app`,
			wantStamp:     `2026-10-11T12:00:04.000000000Z`,
			want:          []pair{num("a", "1")},
		},
		{
			name:          "logfmt after a pod name in brackets",
			line:          `[pod/web-1/app] level=info msg=x`,
			format:        entry.FormatLogfmt,
			wantCollector: `[pod/web-1/app]`,
			wantSource:    `pod/web-1/app`,
			want:          []pair{str("level", "info"), str("msg", "x")},
		},
		{name: "a stack frame after a pod name", line: "[pod/web-1/app] \tat x a=1 b=2", wantCollector: `[pod/web-1/app]`, wantSource: `pod/web-1/app`},
		{name: "indented pairs after a container name", line: "web-1 |   a=1 b=2", wantCollector: `web-1 |`, wantSource: `web-1`},
		{name: "a plain line after a timestamp", line: "2026-10-11T12:00:00Z goroutine 1 [running]:", wantStamp: `2026-10-11T12:00:00Z`},
		{
			name:          "JSON right after the bar",
			line:          `  svc |{"a":1}  `,
			format:        entry.FormatJSON,
			wantCollector: `svc |`,
			wantSource:    `svc`,
			want:          []pair{num("a", "1")},
		},
		{
			name:          "prefix of 64 bytes",
			line:          strings.Repeat("s", 63) + `| {"a":1}`,
			format:        entry.FormatJSON,
			wantCollector: strings.Repeat("s", 63) + `|`,
			wantSource:    strings.Repeat("s", 63),
			want:          []pair{num("a", "1")},
		},
		{
			name:          "prefix of 64 bytes and the whitespace around it",
			line:          "  " + strings.Repeat("s", 63) + "|  \t" + `{"a":1}`,
			format:        entry.FormatJSON,
			wantCollector: strings.Repeat("s", 63) + `|`,
			wantSource:    strings.Repeat("s", 63),
			want:          []pair{num("a", "1")},
		},
		{name: "prefix of 65 bytes", line: strings.Repeat("s", 64) + `| {"a":1}`},
		{name: "prefix that is a word", line: `error: {"a":1}`},
		{name: "prefix that is a sentence", line: `failed to parse {"a":1}`},
		{name: "prefix that is a date without a time", line: `2026-10-11 {"a":1}`},
		{name: "prefix with the bar in the middle", line: `svc | out {"a":1}`, wantCollector: `svc |`, wantSource: `svc`},
		{name: "braces after a timestamp that are not JSON", line: `2026-10-11T12:00:00Z {foo}`, wantStamp: `2026-10-11T12:00:00Z`},
		{name: "braces after a container name that are not JSON", line: `svc | {foo}`, wantCollector: `svc |`, wantSource: `svc`},
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
			name:          "prefixed JSON after a byte order mark",
			line:          "\ufeff" + `svc | {"a":1}`,
			format:        entry.FormatJSON,
			wantCollector: `svc |`,
			wantSource:    `svc`,
			want:          []pair{num("a", "1")},
		},
		// Indented lines are frames of a stack trace, even when they hold pairs.
		{name: "a line of Python source with pairs", line: "    return render(request, template_name=name, context=ctx)"},
		{name: "a Java frame with pairs", line: "\tat x a=1 b=2"},
		{name: "a Node frame with pairs", line: "    at f (a.js:1:2) a=1 b=2"},
		{name: "indented pairs", line: "  level=info msg=x"},
		{name: "pairs after a tab", line: "\ta=1 b=2"},
		{name: "pairs after a byte order mark and a space", line: "\ufeff a=1 b=2"},
		{name: "pairs after a byte order mark and a tab", line: "\ufeff\ta=1 b=2"},
		{name: "logfmt after a carriage return", line: "\ra=1 b=2", format: entry.FormatLogfmt, want: []pair{str("a", "1"), str("b", "2")}},
		{
			name:       "logfmt after a vertical tab",
			line:       "\vx a=1 b=2",
			format:     entry.FormatLogfmt,
			wantPrefix: "\vx",
			want:       []pair{str("a", "1"), str("b", "2")},
		},
		{name: "indented JSON", line: " \t{\"a\":1}", format: entry.FormatJSON, want: []pair{num("a", "1")}},
		{name: "indented JSON after a byte order mark", line: "\ufeff {\"a\":1}", format: entry.FormatJSON, want: []pair{num("a", "1")}},
		{
			name:          "indented JSON after a container name",
			line:          `  svc | {"a":1}`,
			format:        entry.FormatJSON,
			wantCollector: `svc |`,
			wantSource:    `svc`,
			want:          []pair{num("a", "1")},
		},
		// A known limit: a line of a trace that is not indented, with pairs at its end, is still logfmt.
		{
			name:       "the cause of a Java exception with pairs",
			line:       `Caused by: java.io.IOException: boom x=1 y=2`,
			format:     entry.FormatLogfmt,
			wantPrefix: `Caused by: java.io.IOException: boom`,
			want:       []pair{str("x", "1"), str("y", "2")},
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
			for _, part := range []struct {
				name string
				got  []byte
				want string
			}{
				{"collector", e.Collector, tt.wantCollector},
				{"source", e.Source, tt.wantSource},
				{"stamp", e.Stamp, tt.wantStamp},
				{"prefix", e.Prefix, tt.wantPrefix},
			} {
				if string(part.got) != part.want || (part.want == "") != (part.got == nil) {
					t.Errorf("Parse(%q): %s = %q (nil: %v), want %q", tt.line, part.name, part.got, part.got == nil, part.want)
				}
				// An append to a part must not reach the bytes of the line that follow it.
				if cap(part.got) != len(part.got) {
					t.Errorf("Parse(%q): the %s has %d spare bytes, want none", tt.line, part.name, cap(part.got)-len(part.got))
				}
			}
			if got := pairsOf(e.Fields); !slices.Equal(got, tt.want) {
				t.Errorf("Parse(%q)\n got %+v\nwant %+v", tt.line, got, tt.want)
			}
			if string(e.Line) != tt.line || len(line) > 0 && &e.Line[0] != &line[0] {
				t.Errorf("Parse(%q): Line = %q, want the line itself", tt.line, e.Line)
			}
			if tt.format == entry.FormatPlain {
				if got := rolesOf(t, e); got != (roles{time: got.time}) || (got.time == "") != (e.Stamp == nil) {
					t.Errorf("Parse(%q): a plain line has the roles %+v, want only the time of its stamp %q", tt.line, got, e.Stamp)
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

		fields, prefix, split, ok := appendJSONLineFields(kept, slices.Clip([]byte(line)))

		if ok || prefix != nil || split.ok || split.bar != 0 || split.stamp != nil || len(fields) != 1 || string(fields[0].Key) != "kept" {
			t.Errorf("appendJSONLineFields(%q) = %+v, prefix %q (%+v), %v; want dst as it was, no prefix and false",
				line, pairsOf(fields), prefix, split, ok)
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
			name: "prefixed JSON without a time field takes the time of the prefix",
			line: `2026-10-11T12:00:00Z {"level":"error","message":"x"}`,
			want: roles{time: "2026-10-11T12:00:00Z", level: entry.LevelError, levelKey: "level", msgKey: "message"},
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

func TestSplitCollector(t *testing.T) {
	const stamp = "2026-10-11T12:00:00.123456789Z"
	name253 := strings.Repeat("s", 253)
	stamp64 := "2026-10-11T12:00:00." + strings.Repeat("1", 43) + "Z"
	tests := []struct {
		name, line                         string
		collector, source, wantStamp, body string
	}{
		{name: "docker compose", line: "web-1    | hello", collector: "web-1    |", source: "web-1", body: "hello"},
		{name: "docker compose, the longest name", line: "worker-2 | hello", collector: "worker-2 |", source: "worker-2", body: "hello"},
		{name: "docker compose, an empty line", line: "web-1    | ", collector: "web-1    |", source: "web-1"},
		{name: "docker compose, the bar at the end", line: "web-1 |", collector: "web-1 |", source: "web-1"},
		{name: "docker compose, an indented line", line: "web-1 |   at x", collector: "web-1 |", source: "web-1", body: "  at x"},
		{name: "docker compose, a name with dots and underscores", line: "proj_web.1 | x", collector: "proj_web.1 |", source: "proj_web.1", body: "x"},
		{
			name: "docker compose -t", line: "web-1  | 2026-10-11T15:00:00.123456789+03:00 hello",
			collector: "web-1  |", source: "web-1", wantStamp: "2026-10-11T15:00:00.123456789+03:00", body: "hello",
		},
		{name: "kubectl --prefix", line: "[pod/web-1/app] hello", collector: "[pod/web-1/app]", source: "pod/web-1/app", body: "hello"},
		{name: "kubectl --prefix, an empty line", line: "[pod/web-1/app]", collector: "[pod/web-1/app]", source: "pod/web-1/app"},
		{
			name: "kubectl --prefix --timestamps", line: "[pod/web-7d9f.x/app] " + stamp + " hello",
			collector: "[pod/web-7d9f.x/app]", source: "pod/web-7d9f.x/app", wantStamp: stamp, body: "hello",
		},
		{name: "kubectl --timestamps", line: stamp + " hello", wantStamp: stamp, body: "hello"},
		{name: "a timestamp without a fraction", line: "2026-10-11T12:00:00Z hello", wantStamp: "2026-10-11T12:00:00Z", body: "hello"},
		{name: "a timestamp with an offset", line: "2026-10-11T12:00:00-07:00 x", wantStamp: "2026-10-11T12:00:00-07:00", body: "x"},
		{name: "a timestamp at the end", line: stamp, wantStamp: stamp},
		{name: "a timestamp before an indented line", line: stamp + " \tat x", wantStamp: stamp, body: "\tat x"},

		{name: "a word in brackets", line: "[main] hello", body: "[main] hello"},
		{name: "brackets with one slash", line: "[pod/web-1] hello", body: "[pod/web-1] hello"},
		{name: "brackets with an empty name", line: "[pod//app] hello", body: "[pod//app] hello"},
		{name: "brackets with an empty container", line: "[pod/web-1/] hello", body: "[pod/web-1/] hello"},
		{name: "brackets with three slashes", line: "[pod/a/b/c] hello", body: "[pod/a/b/c] hello"},
		{name: "brackets of another kind", line: "[deploy/web/app] hello", body: "[deploy/web/app] hello"},
		{name: "brackets without a space after them", line: "[pod/web-1/app]hello", body: "[pod/web-1/app]hello"},
		{name: "a bar without a space before it", line: "INFO| hello", body: "INFO| hello"},
		{name: "a bar without a space after it", line: "svc |hello", body: "svc |hello"},
		{name: "a bar and a tab", line: "svc |\thello", body: "svc |\thello"},
		{name: "a name with a space", line: "a b | hello", body: "a b | hello"},
		{name: "a bar alone", line: "| hello", body: "| hello"},
		{name: "a name after a space", line: " svc | hello", body: " svc | hello"},
		{name: "loguru", line: "2026-10-11 12:00:03.123 | INFO     | app - x", body: "2026-10-11 12:00:03.123 | INFO     | app - x"},
		{name: "logback", line: "2026-10-11 12:00:00,123 INFO  [main] x", body: "2026-10-11 12:00:00,123 INFO  [main] x"},
		{name: "a timestamp without a zone", line: "2026-10-11T12:00:00 x", body: "2026-10-11T12:00:00 x"},
		{name: "a timestamp with an offset without a colon", line: "2026-10-11T12:00:00+0300 x", body: "2026-10-11T12:00:00+0300 x"},
		{name: "a timestamp that is not a time", line: "2026-13-11T12:00:00Z x", body: "2026-13-11T12:00:00Z x"},
		{name: "a timestamp glued to the text", line: "2026-10-11T12:00:00Zx", body: "2026-10-11T12:00:00Zx"},
		{name: "a timestamp before the name", line: "2026-10-11T12:00:00Z svc | x", wantStamp: "2026-10-11T12:00:00Z", body: "svc | x"},
		{name: "a name longer than the limit", line: strings.Repeat("s", 254) + " | x", body: strings.Repeat("s", 254) + " | x"},
		{name: "a name at the limit", line: strings.Repeat("s", 253) + " | x", collector: strings.Repeat("s", 253) + " |", source: strings.Repeat("s", 253), body: "x"},
		{name: "padding at the limit", line: "s" + strings.Repeat(" ", 253) + "| x", collector: "s" + strings.Repeat(" ", 253) + "|", source: "s", body: "x"},
		{name: "padding past the limit", line: "s" + strings.Repeat(" ", 254) + "| x", body: "s" + strings.Repeat(" ", 254) + "| x"},
		{
			name: "a pod and a container at the limit", line: "[pod/" + name253 + "/" + name253 + "] x",
			collector: "[pod/" + name253 + "/" + name253 + "]", source: "pod/" + name253 + "/" + name253, body: "x",
		},
		{name: "a pod past the limit", line: "[pod/" + name253 + "s/app] x", body: "[pod/" + name253 + "s/app] x"},
		{name: "a container past the limit", line: "[pod/web/" + name253 + "s] x", body: "[pod/web/" + name253 + "s] x"},
		{name: "a timestamp at the limit", line: stamp64 + " x", wantStamp: stamp64, body: "x"},
		{name: "a timestamp past the limit", line: stamp64[:20] + "1" + stamp64[20:] + " x", body: stamp64[:20] + "1" + stamp64[20:] + " x"},
		{name: "a plain line", line: "hello world", body: "hello world"},
		{name: "an empty line", line: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Clipped, so that a read past the end of the line panics instead of finding spare capacity.
			line := slices.Clip([]byte(tt.line))

			collector, source, gotStamp, body, when := splitCollector(line)
			// The time of a strict stamp is read once, here; its zone is its own.
			if want, err := time.Parse(time.RFC3339Nano, tt.wantStamp); (err == nil) != !when.IsZero() || (!when.Equal(want) && err == nil) {
				t.Errorf("splitCollector(%q): time %v, want the time of %q", tt.line, when, tt.wantStamp)
			}
			// With spare capacity, so that a part that runs to the end of the line shows it unless it is clipped.
			_, _, _, spareBody, _ := splitCollector(append(make([]byte, 0, len(line)+8), line...))
			if cap(spareBody) != len(spareBody) {
				t.Errorf("splitCollector(%q): body has %d spare bytes, want none", tt.line, cap(spareBody)-len(spareBody))
			}

			for _, part := range []struct {
				name     string
				got      []byte
				want     string
				canBeNil bool
			}{
				{"collector", collector, tt.collector, true},
				{"source", source, tt.source, true},
				{"stamp", gotStamp, tt.wantStamp, true},
				{"body", body, tt.body, false},
			} {
				if string(part.got) != part.want || part.canBeNil && (part.want == "") != (part.got == nil) {
					t.Errorf("splitCollector(%q): %s = %q (nil: %v), want %q", tt.line, part.name, part.got, part.got == nil, part.want)
				}
				// An append to a part must not reach the bytes of the line that follow it.
				if part.canBeNil && cap(part.got) != len(part.got) {
					t.Errorf("splitCollector(%q): %s has %d spare bytes, want none", tt.line, part.name, cap(part.got)-len(part.got))
				}
			}
		})
	}
}

// The one separator byte after what a collector wrote is dropped; the rest of the whitespace is the line's.
func TestParseBody(t *testing.T) {
	tests := []struct{ name, line, body string }{
		{name: "a tab after the bar", line: "a\t|\t{\"a\":1}", body: `{"a":1}`},
		{name: "spaces before the object", line: `a | b |    {"a":1}`, body: `   {"a":1}`},
		{name: "no space before the object", line: `2026-10-11 12:00:00{"a":1}`, body: `{"a":1}`},
		{name: "logfmt after a stamp", line: "2026-10-11 12:00:00,123  a=1 b=2", body: " a=1 b=2"},
		{name: "a strict collector", line: `svc |    {"a":1}`, body: `   {"a":1}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var p Parser
			if e := p.Parse([]byte(tt.line)); string(e.Body) != tt.body {
				t.Errorf("Parse(%q).Body = %q, want %q", tt.line, e.Body, tt.body)
			}
		})
	}
}

func TestSpacesAreIsSpace(t *testing.T) {
	for c := range 256 {
		if in := strings.IndexByte(spaces, byte(c)) >= 0; in != isSpace(byte(c)) {
			t.Errorf("byte %#x: in spaces %v, isSpace %v", c, in, !in)
		}
	}
}

func TestNameBytes(t *testing.T) {
	for c := range 256 {
		if want := isAlnum(byte(c)) || c == '.' || c == '_' || c == '-'; nameBytes[c] != want {
			t.Errorf("nameBytes[%#x] = %v, want %v", c, nameBytes[c], want)
		}
	}
}

// An entry without a time of its own, and a plain line, take the time a collector wrote.
func TestParseTimeFromStamp(t *testing.T) {
	moscow := time.FixedZone("test", 3*60*60)
	at := func(s string) time.Time {
		tm, err := time.Parse(time.RFC3339Nano, s)
		if err != nil {
			t.Fatal(err)
		}
		return tm
	}
	tests := []struct {
		name, line string
		want       time.Time
		timeField  int
	}{
		{name: "a plain line after a pod", line: "[pod/a/b] 2026-10-11T12:00:00.5Z \tat x", want: at("2026-10-11T12:00:00.5Z"), timeField: -1},
		{name: "a stamp alone", line: "2026-10-11T15:00:00+03:00", want: at("2026-10-11T12:00:00Z"), timeField: -1},
		{name: "JSON without a time", line: `2026-10-11T12:00:00Z {"msg":"x"}`, want: at("2026-10-11T12:00:00Z"), timeField: -1},
		{name: "logfmt without a time", line: `web-1 | 2026-10-11T12:00:00Z a=1 b=2`, want: at("2026-10-11T12:00:00Z"), timeField: -1},
		{name: "the time of the entry wins", line: `2026-10-11T12:00:00Z {"time":"2026-10-11T13:00:00Z"}`, want: at("2026-10-11T13:00:00Z"), timeField: 0},
		{name: "a time of the entry that cannot be read", line: `2026-10-11T12:00:00Z {"time":"yesterday"}`, want: at("2026-10-11T12:00:00Z"), timeField: -1},
		{name: "a stamp without a zone is in Loc", line: `web | 2026-10-11 12:00:00,123 {"a":1}`, want: at("2026-10-11T09:00:00.123Z"), timeField: -1},
		{name: "a stamp before the bar is the collector's text", line: `2026-10-11T12:00:00Z svc | {"a":1}`, timeField: -1},
		{name: "no stamp", line: `web-1 | hello`, timeField: -1},
		{name: "a plain line", line: `2026-10-11 12:00:00 hello`, timeField: -1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := Parser{Loc: moscow}
			p.Parse([]byte(`{"time":"2000-01-01T00:00:00Z"}`)) // its time must not stay
			e := p.Parse([]byte(tt.line))
			if !e.Time.Equal(tt.want) || e.TimeField() != tt.timeField {
				t.Errorf("Parse(%q): time %v, field %d; want %v, %d", tt.line, e.Time, e.TimeField(), tt.want, tt.timeField)
			}
		})
	}
}
