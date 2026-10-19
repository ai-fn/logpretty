package app

import (
	"bytes"
	"cmp"
	"io"
	"strings"
	"testing"
)

// One iteration is a whole Run over about 100k lines, so allocs/op is the fixed setup cost, not per-line work.
func BenchmarkRun(b *testing.B) {
	const (
		lines = 100_000
		short = `{"level":"info","msg":"request done","status":200}`
		user  = `{"level":"warn","msg":"slow request","user_id":42,"latency":"1.2s","path":"/api/v1/users"}`
		// Two containers at once, one of them with a stack trace between the entries of the other.
		compose = "web-1  | " + `{"level":"error","msg":"request failed","status":500}` + "\n" +
			"web-1  | Traceback (most recent call last):\n" +
			"api-1  | " + `{"level":"info","msg":"request done","status":200}` + "\n" +
			"web-1  |   File \"/app/views.py\", line 12, in index\n" +
			"api-1  | GET /health 200\n" +
			"web-1  | ValueError: bad template"
		stacktrace = `{"level":"error","msg":"query failed","error":"timeout","stacktrace":"main.run\n\t/app/main.go:30\nmain.main\n\t/app/main.go:12"}`
		// Three requests at once, their ids under three keys.
		otel = `{"level":"info","msg":"request","trace_id":"4bf92f3577b34da6a3ce929d0e0e4736","path":"/a"}` + "\n" +
			`{"level":"info","msg":"request","traceId":"00f067aa0ba902b7","path":"/b"}` + "\n" +
			`level=info msg=request traceID=a3ce929d0e0e4736 path=/c`
	)
	tests := []struct {
		name string
		args []string
		line string // a unit of one line or more, repeated to make about 100k
		out  int    // the lines the unit is shown as; 0 means as many as it has
	}{
		{name: "json-short", line: short},
		{name: "logfmt-short", line: `level=info msg="request done" status=200`},
		{name: "plain-1KiB", line: strings.Repeat("x", 1<<10)},
		// Rows with flags: every line passes. Compare level-short with json-short and user-target with user-bare,
		// the same lines without flags; the target row also prints fewer fields. No name holds another one.
		{name: "level-short", args: []string{"--level", "info"}, line: short},
		{name: "user-bare", line: user},
		{name: "user-target", args: []string{"--level", "warn", "--where", "user_id=42", "--fields", "msg,latency"}, line: user},
		// A stack trace in a field is unfolded into 6 lines; --no-expand keeps it in one.
		{name: "json-stacktrace", line: stacktrace, out: 6},
		{name: "json-stacktrace-flat", args: []string{"--no-expand"}, line: stacktrace},
		// With --level error the plain lines follow the entries of their own container: 4 of the 6 are shown.
		{name: "compose-trace", line: compose, out: 6},
		{name: "compose-trace-level", args: []string{"--level", "error"}, line: compose, out: 4},
		// --trace keeps one request of the three.
		{name: "otel-bare", line: otel},
		{name: "otel-trace", args: []string{"--trace", "00f067aa0ba902b7"}, line: otel, out: 1},
	}

	for _, tt := range tests {
		b.Run(tt.name, func(b *testing.B) {
			unitLines := strings.Count(tt.line, "\n") + 1
			repeats := lines / unitLines
			want := repeats * cmp.Or(tt.out, unitLines)
			input := []byte(strings.Repeat(tt.line+"\n", repeats))
			in := bytes.NewReader(input)
			// A row whose lines were dropped would measure next to nothing and look fast.
			var out bytes.Buffer
			if code := Run(b.Context(), tt.args, in, &out, io.Discard); code != 0 || bytes.Count(out.Bytes(), []byte("\n")) != want {
				b.Fatalf("exit code = %d and %d lines of output, want 0 and %d", code, bytes.Count(out.Bytes(), []byte("\n")), want)
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
