package app

import (
	"bytes"
	"io"
	"strings"
	"testing"
)

// One iteration is a whole Run over 100k lines, so allocs/op is the fixed setup cost, not per-line work.
func BenchmarkRun(b *testing.B) {
	const lines = 100_000
	tests := []struct {
		name string
		line string
	}{
		{name: "json-short", line: `{"level":"info","msg":"request done","status":200}`},
		{name: "logfmt-short", line: `level=info msg="request done" status=200`},
		{name: "plain-1KiB", line: strings.Repeat("x", 1<<10)},
	}

	for _, tt := range tests {
		b.Run(tt.name, func(b *testing.B) {
			input := []byte(strings.Repeat(tt.line+"\n", lines))
			in := bytes.NewReader(input)
			b.SetBytes(int64(len(input)))
			b.ReportAllocs()

			for b.Loop() {
				in.Reset(input)
				if code := Run(b.Context(), nil, in, io.Discard, io.Discard); code != 0 {
					b.Fatalf("exit code = %d, want 0", code)
				}
			}
		})
	}
}
