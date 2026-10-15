package parse

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"
)

type benchLine struct {
	name, line string
	bare       bool // a lone JSON object, which json.Valid takes whole
}

// The source is cache-resident: treat the numbers as a regression guard, not absolute throughput.
func benchLines() []benchLine {
	var wide strings.Builder
	wide.WriteString(`{"time":"2026-10-11T12:00:00.123+03:00","level":"info","msg":"request \"done\"","http":{"method":"GET","path":"/api/v1/users","headers":{"accept":"*/*"}}`)
	for i := range 16 {
		fmt.Fprintf(&wide, `,"field%d":%d`, i, i*1000)
	}
	wide.WriteString(`}`)

	return []benchLine{
		{name: "json-short", line: `{"level":"info","msg":"request done","status":200}`, bare: true},
		{name: "json-20-fields", line: wide.String(), bare: true},
		{name: "json-zoneless-time", line: `{"asctime":"2026-10-11 12:00:00,123","levelname":"INFO","message":"started"}`, bare: true},
		// The one path that allocates: a key with an escape is decoded into memory of its own.
		{name: "json-escaped-key", line: `{"level":"info","msg":"request done","st\"atus":200}`, bare: true},
		{name: "json-prefix", line: `2026-10-11T12:00:00.123456789Z {"level":"info","msg":"request done","status":200}`},
		{name: "logfmt", line: `time=2026-10-11T12:00:00Z level=info msg="request done" status=200 path=/api/v1/users duration=1.5s`},
		{name: "plain", line: `2026-10-11 12:00:00 INFO request done in 5ms from 10.0.0.1 for /api/v1/users`},
		{name: "plain-one-pair", line: `Starting server on port=8080 with the configuration read from the environment`},
		{name: "plain-brace", line: `2026/10/11 12:00:00 config: {Addr::8080 ReadTimeout:30s}`},
		{name: "plain-1KiB", line: strings.Repeat("x ", 512)},
		// One word that is no prefix, as in a hex dump: the name of a container must not be scanned to its end.
		{name: "plain-long-word", line: strings.Repeat("a", 1<<10)},
		{name: "plain-compose", line: `web-1    |   File "/app/views.py", line 12, in index`},
		{name: "plain-stamp", line: `[pod/web-1/app] 2026-10-11T12:00:05.000000000Z goroutine 1 [running]:`},
		{name: "json-kubectl-prefix", line: `[pod/web-1/app] 2026-10-11T12:00:04.000000000Z {"level":"info","msg":"request done","status":200}`},
	}
}

func BenchmarkParse(b *testing.B) {
	for _, tt := range benchLines() {
		b.Run(tt.name, func(b *testing.B) {
			// Loc is set, so that the lazy load of time.Local cannot allocate inside the loop.
			p := Parser{Loc: time.UTC}
			line := []byte(tt.line)
			p.Parse(line) // grows the fields once
			b.SetBytes(int64(len(line)))
			b.ReportAllocs()

			for b.Loop() {
				p.Parse(line)
			}
		})
	}
}

// BenchmarkValid is the price of json.Valid alone on the same lines, to see what the parser adds to it.
func BenchmarkValid(b *testing.B) {
	for _, tt := range benchLines() {
		if !tt.bare {
			continue
		}
		b.Run(tt.name, func(b *testing.B) {
			line := []byte(tt.line)
			b.SetBytes(int64(len(line)))
			b.ReportAllocs()

			for b.Loop() {
				json.Valid(line)
			}
		})
	}
}

func BenchmarkFind(b *testing.B) {
	var line string
	for _, tt := range benchLines() {
		if tt.name == "json-20-fields" {
			line = tt.line
		}
	}
	tests := []struct{ name, path string }{
		{name: "first-key", path: "time"},
		{name: "last-key", path: "field15"},
		{name: "one-level-down", path: "http.method"},
		{name: "two-levels-down", path: "http.headers.accept"},
		{name: "missing", path: "nope"},
		{name: "missing-below", path: "http.nope"},
	}
	for _, tt := range tests {
		b.Run(tt.name, func(b *testing.B) {
			p := Parser{Loc: time.UTC}
			e := p.Parse([]byte(line))
			path := []byte(tt.path)
			var f Finder
			f.Find(e, path) // grows the scratch space once
			b.ReportAllocs()

			for b.Loop() {
				f.Find(e, path)
			}
		})
	}
}
