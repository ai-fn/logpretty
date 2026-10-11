package linereader

import (
	"bytes"
	"errors"
	"io"
	"slices"
	"testing"
)

var errBoom = errors.New("boom")

// chunkReader hands out at most chunk bytes per Read, then final, either with the last bytes or on the next Read.
type chunkReader struct {
	data     []byte
	chunk    int
	final    error
	together bool
	reads    int
}

func (c *chunkReader) Read(p []byte) (int, error) {
	c.reads++
	if len(c.data) == 0 {
		return 0, c.final
	}
	n := min(len(p), len(c.data), c.chunk)
	copy(p, c.data[:n])
	c.data = c.data[n:]
	if len(c.data) == 0 && c.together {
		return n, c.final
	}
	return n, nil
}

// reference is the obvious implementation Reader must agree with.
func reference(data []byte, maxLine int) []item {
	var out []item
	for len(data) > 0 {
		line, rest, hasNL := bytes.Cut(data, []byte{'\n'})
		if hasNL {
			line = bytes.TrimSuffix(line, []byte{'\r'})
		}
		dropped := 0
		if len(line) > maxLine {
			dropped = len(line) - maxLine
			line = line[:maxLine]
		}
		out = append(out, item{string(line), dropped})
		data = rest
	}
	return out
}

func FuzzReader(f *testing.F) {
	// Seeds are offset by the clamping below: maxLine = 1 + m%64, bufSize = 16 + b%64.
	f.Add([]byte("a\nb\r\nc"), uint8(8), uint8(16), uint8(0), uint8(0))
	f.Add([]byte("xxxxxxxxx\r\nnext\n"), uint8(8), uint8(16), uint8(3), uint8(1))
	f.Add([]byte("\r\r\n\n\r"), uint8(1), uint8(16), uint8(1), uint8(2))
	f.Add(bytes.Repeat([]byte("z"), 200), uint8(8), uint8(16), uint8(7), uint8(3))
	f.Add([]byte("abc\r"), uint8(3), uint8(16), uint8(2), uint8(2))
	f.Add(append(bytes.Repeat([]byte("a"), 31), "\r\nb\n"...), uint8(40), uint8(16), uint8(31), uint8(0)) // CR ends a buffer, LF starts the next

	f.Fuzz(func(t *testing.T, data []byte, maxLine, bufSize, chunk, mode uint8) {
		ml := 1 + int(maxLine)%64
		bs := 16 + int(bufSize)%64
		src := &chunkReader{data: slices.Clone(data), chunk: 1 + int(chunk)%32, final: io.EOF, together: mode&2 != 0}
		if mode&1 != 0 {
			src.final = errBoom
		}
		r := newReader(src, bs, ml)

		var got []item
		for range len(data) + 2 {
			readsBefore := src.reads
			mayBlock := r.MayBlock()

			line, dropped, err := r.Next()

			if !mayBlock && src.reads != readsBefore {
				t.Fatalf("MayBlock was false but Next read from the source (data %q)", data)
			}
			if err != nil {
				if !errors.Is(err, src.final) || len(line) != 0 || dropped != 0 {
					t.Fatalf("Next = %q, %d, %v; want only %v (data %q)", line, dropped, err, src.final, data)
				}
				if want := reference(data, ml); !slices.Equal(got, want) {
					t.Fatalf("maxLine=%d bufSize=%d chunk=%d mode=%d data %q:\n got  %+v\n want %+v", ml, bs, src.chunk, mode, data, got, want)
				}
				if line, dropped, again := r.Next(); !errors.Is(again, src.final) || len(line) != 0 || dropped != 0 {
					t.Fatalf("call after the error = %q, %d, %v; want %v again", line, dropped, again, src.final)
				}
				return
			}
			if len(line) > ml || dropped < 0 {
				t.Fatalf("line of %d bytes (maxLine %d), dropped %d (data %q)", len(line), ml, dropped, data)
			}
			got = append(got, item{string(line), dropped})
		}
		t.Fatalf("reader did not reach the end within %d calls (data %q)", len(data)+2, data)
	})
}
