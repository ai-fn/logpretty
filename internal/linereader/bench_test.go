package linereader

import (
	"bytes"
	"strings"
	"testing"
)

// cycleReader serves the same bytes forever.
type cycleReader struct {
	data []byte
	off  int
}

func (c *cycleReader) Read(p []byte) (int, error) {
	n := copy(p, c.data[c.off:])
	c.off = (c.off + n) % len(c.data)
	return n, nil
}

// The source is cache-resident: treat the numbers as a regression guard, not absolute throughput.
func BenchmarkNext(b *testing.B) {
	tests := []struct {
		name    string
		line    string
		maxLine int
	}{
		{name: "json-short", line: `{"level":"info","msg":"request done","status":200}`, maxLine: DefaultMaxLine},
		{name: "plain-1KiB", line: strings.Repeat("x", 1<<10), maxLine: DefaultMaxLine},
		{name: "crlf-short", line: `{"level":"info","msg":"request done"}` + "\r", maxLine: DefaultMaxLine},
		{name: "long-kept", line: strings.Repeat("x", 100<<10), maxLine: DefaultMaxLine},
		{name: "long-truncated", line: strings.Repeat("x", 200<<10), maxLine: 1 << 10},
	}

	for _, tt := range tests {
		b.Run(tt.name, func(b *testing.B) {
			unit := tt.line + "\n"
			src := &cycleReader{data: bytes.Repeat([]byte(unit), max(1, (256<<10)/len(unit)))}
			r := New(src, tt.maxLine)
			b.SetBytes(int64(len(unit)))
			b.ReportAllocs()

			for b.Loop() {
				if _, _, err := r.Next(); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
