package app

import (
	"bytes"
	"io"
	"strings"
	"testing"
)

// One iteration is a whole Run over 100k lines, so allocs/op is the fixed setup cost, not per-line work.
func BenchmarkRun(b *testing.B) {
	const (
		lines = 100_000
		short = `{"level":"info","msg":"request done","status":200}`
		user  = `{"level":"warn","msg":"slow request","user_id":42,"latency":"1.2s","path":"/api/v1/users"}`
	)
	tests := []struct {
		name string
		args []string
		line string
	}{
		{name: "json-short", line: short},
		{name: "logfmt-short", line: `level=info msg="request done" status=200`},
		{name: "plain-1KiB", line: strings.Repeat("x", 1<<10)},
		// Rows with flags: every line passes. Compare level-short with json-short and user-target with user-bare,
		// the same lines without flags; the target row also prints fewer fields. No name holds another one.
		{name: "level-short", args: []string{"--level", "info"}, line: short},
		{name: "user-bare", line: user},
		{name: "user-target", args: []string{"--level", "warn", "--where", "user_id=42", "--fields", "msg,latency"}, line: user},
	}

	for _, tt := range tests {
		b.Run(tt.name, func(b *testing.B) {
			input := []byte(strings.Repeat(tt.line+"\n", lines))
			in := bytes.NewReader(input)
			// A row whose lines were dropped would measure next to nothing and look fast.
			var out bytes.Buffer
			if code := Run(b.Context(), tt.args, in, &out, io.Discard); code != 0 || bytes.Count(out.Bytes(), []byte("\n")) != lines {
				b.Fatalf("exit code = %d and %d lines of output, want 0 and %d", code, bytes.Count(out.Bytes(), []byte("\n")), lines)
			}
			b.SetBytes(int64(len(input)))
			b.ReportAllocs()

			for b.Loop() {
				in.Reset(input)
				if code := Run(b.Context(), tt.args, in, io.Discard, io.Discard); code != 0 {
					b.Fatalf("exit code = %d, want 0", code)
				}
			}
		})
	}
}
