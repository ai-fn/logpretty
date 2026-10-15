package filter

import (
	"testing"
	"time"

	"github.com/ai-fn/logpretty/internal/entry"
	"github.com/ai-fn/logpretty/internal/parse"
)

// The entry is parsed once: this is the price of the decision alone, on top of the parser.
func BenchmarkKeep(b *testing.B) {
	const line = `{"time":"2026-10-11T12:00:00.123+03:00","level":"warn","msg":"slow request","user_id":42,"latency":"1.2s",` +
		`"http":{"method":"GET","path":"/api/v1/users","status":502},"note":"say \"hi\"","trace_id":"4bf92f3577b34da6a3ce929d0e0e4736"}`

	tests := []struct {
		name  string
		level entry.Level
		where []string
	}{
		{name: "no-filter"},
		{name: "level", level: entry.LevelWarn},
		{name: "equal-number", where: []string{"user_id=42"}},
		{name: "equal-string", where: []string{"latency=1.2s"}},
		{name: "equal-escaped-string", where: []string{`note="say \"hi\""`}},
		{name: "order", where: []string{"user_id>=42"}},
		{name: "regexp", where: []string{"msg~^slow"}},
		{name: "nested-path", where: []string{"http.status>=500"}},
		{name: "missing", where: []string{"nope!=1"}},
		{name: "time", where: []string{"time>=2026-10-11T00:00:00Z"}},
		{name: "target-command", level: entry.LevelWarn, where: []string{"user_id=42"}},
	}
	for _, tt := range tests {
		b.Run(tt.name, func(b *testing.B) {
			f := Filter{MinLevel: tt.level}
			for _, expression := range tt.where {
				c, err := Compile(expression, time.UTC)
				if err != nil {
					b.Fatal(err)
				}
				f.Where = append(f.Where, c)
			}
			p := parse.Parser{Loc: time.UTC}
			e := p.Parse([]byte(line))
			if !f.Keep(e) { // grows the scratch space once
				b.Fatal("the entry does not pass the filter")
			}
			b.ReportAllocs()

			for b.Loop() {
				f.Keep(e)
			}
		})
	}
}

// Two containers with a stack trace in the middle, decided under --level error. The unnamed stream takes the
// general rule, so the two rows are not one workload.
func BenchmarkKeepSources(b *testing.B) {
	streams := []struct {
		name  string
		lines []string
		named bool   // the lines carry a source
		want  []bool // what the filter decides on each of them
	}{
		{name: "with-sources", named: true, want: []bool{true, true, false, true, false, true}, lines: []string{
			`web-1  | {"level":"error","msg":"request failed"}`,
			"web-1  | Traceback (most recent call last):",
			`api-1  | {"level":"info","msg":"request done"}`,
			`web-1  |   File "/app/views.py", line 12, in index`,
			"api-1  | GET /health 200",
			"web-1  | ValueError: bad template",
		}},
		{name: "without-sources", want: []bool{true, true, false, false, false, false}, lines: []string{
			`{"level":"error","msg":"request failed"}`,
			"Traceback (most recent call last):",
			`{"level":"info","msg":"request done"}`,
			`  File "/app/views.py", line 12, in index`,
			"GET /health 200",
			"ValueError: bad template",
		}},
	}
	for _, tt := range streams {
		b.Run(tt.name, func(b *testing.B) {
			// One parser for each line: a parser gives out the same entry again.
			entries := make([]*entry.Entry, len(tt.lines))
			for i, line := range tt.lines {
				p := &parse.Parser{Loc: time.UTC}
				entries[i] = p.Parse([]byte(line))
			}
			f := Filter{MinLevel: entry.LevelError}
			for _, e := range entries { // remembers the sources once
				f.Keep(e)
			}
			// A parser that stopped reading the container name would leave this row on the general path.
			for i, e := range entries {
				if got := f.Keep(e); (len(e.Source) > 0) != tt.named || got != tt.want[i] {
					b.Fatalf("line %q: source %q, kept %v; want a source: %v, kept %v", tt.lines[i], e.Source, got, tt.named, tt.want[i])
				}
			}
			b.ReportAllocs()

			for b.Loop() {
				for _, e := range entries {
					f.Keep(e)
				}
			}
		})
	}
}
