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
