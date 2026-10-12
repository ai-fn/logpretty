package render

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/ai-fn/logpretty/internal/parse"
)

// SetBytes counts the input line, not the output. The source is cache-resident: a regression guard,
// not absolute throughput.
func BenchmarkRender(b *testing.B) {
	var wide strings.Builder
	wide.WriteString(`{"time":"2026-10-11T12:00:00.123+03:00","level":"info","msg":"request \"done\"","http":{"method":"GET","path":"/api/v1/users","headers":{"accept":"*/*"}}`)
	for i := range 16 {
		fmt.Fprintf(&wide, `,"field%d":%d`, i, i*1000)
	}
	wide.WriteString(`}`)

	lines := []struct{ name, line string }{
		{name: "json-short", line: `{"level":"info","msg":"request done","status":200}`},
		{name: "json-20-fields", line: wide.String()},
		{name: "json-escaped-value", line: `{"time":"2026-10-11T12:00:00Z","level":"error","msg":"line one\nline \"two\"","path":"C:\\dir \u00e9"}`},
		{name: "json-prefix", line: `2026-10-11T12:00:00.123456789Z {"level":"info","msg":"request done","status":200}`},
		{name: "logfmt", line: `time=2026-10-11T12:00:00Z level=info msg="request done" status=200 path=/api/v1/users duration=1.5s`},
		// A plain line takes no style: its color row is a control.
		{name: "plain", line: `2026-10-11 12:00:00 INFO request done in 5ms from 10.0.0.1 for /api/v1/users`},
	}
	themes := []struct {
		name  string
		theme Theme
	}{
		{name: "no-color"},
		{name: "color", theme: DefaultTheme()},
	}

	for _, tt := range lines {
		for _, th := range themes {
			b.Run(tt.name+"/"+th.name, func(b *testing.B) {
				// The zone is set, so that the lazy load of time.Local cannot allocate inside the loop.
				p := parse.Parser{Loc: time.UTC}
				e := p.Parse([]byte(tt.line))
				r := Renderer{Theme: th.theme, Loc: time.UTC}
				dst := r.AppendEntry(nil, e) // grows the buffers once
				b.SetBytes(int64(len(tt.line)))
				b.ReportAllocs()

				for b.Loop() {
					dst = r.AppendEntry(dst[:0], e)
				}
			})
		}
	}
}

func BenchmarkAppendSafe(b *testing.B) {
	binary := make([]byte, 1<<10)
	for i := range binary {
		binary[i] = byte(i)
	}
	inputs := []struct {
		name string
		text []byte
	}{
		{name: "ascii-1KiB", text: []byte(strings.Repeat("x ", 512))},
		{name: "cyrillic-1KiB", text: []byte(strings.Repeat("привет мир ", 51))},
		// Every byte value: control characters to write out and bytes that are not UTF-8.
		{name: "binary-1KiB", text: binary},
	}

	for _, tt := range inputs {
		b.Run(tt.name, func(b *testing.B) {
			dst := appendSafe(nil, tt.text) // grows the buffer once
			b.SetBytes(int64(len(tt.text)))
			b.ReportAllocs()

			for b.Loop() {
				dst = appendSafe(dst[:0], tt.text)
			}
		})
	}
}
