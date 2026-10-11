// Package linereader reads newline-terminated lines of any length from a stream.
package linereader

import (
	"bufio"
	"bytes"
	"io"
)

const (
	// DefaultMaxLine is the longest line kept; the rest of a longer line is dropped.
	DefaultMaxLine = 1 << 20

	bufferSize = 64 << 10
)

// Reader splits a stream into lines without ever failing on a long one.
type Reader struct {
	br      *bufio.Reader
	maxLine int
	buf     []byte
	err     error // read error held back until the data before it has been returned
}

// New returns a Reader keeping at most maxLine bytes per line; maxLine <= 0 means DefaultMaxLine.
func New(r io.Reader, maxLine int) *Reader {
	return newReader(r, bufferSize, maxLine)
}

func newReader(r io.Reader, bufSize, maxLine int) *Reader {
	if maxLine <= 0 {
		maxLine = DefaultMaxLine
	}
	return &Reader{br: bufio.NewReaderSize(r, bufSize), maxLine: maxLine}
}

// Next returns the line without its terminator and the bytes dropped by truncation; the slice
// is valid until the next call. A final line without "\n" comes first and the error (sticky) after it.
func (r *Reader) Next() (line []byte, dropped int, err error) {
	if r.err != nil {
		return nil, 0, r.err
	}

	// buf keeps up to maxLine bytes; the final byte is tracked in last, so a trailing "\r" costs no extra room.
	r.buf = r.buf[:0]
	var (
		total  int // content bytes seen, without the "\n"
		last   byte
		hasNL  bool
		failed error
	)
	for {
		chunk, e := r.br.ReadSlice('\n')
		content := chunk
		if e == nil {
			content = chunk[:len(chunk)-1]
			hasNL = true
		}
		if len(content) > 0 {
			total += len(content)
			last = content[len(content)-1]
			r.buf = append(r.buf, content[:min(len(content), r.maxLine-len(r.buf))]...)
		}
		if e == bufio.ErrBufferFull {
			continue // the line is longer than the reader's buffer
		}
		if e != nil {
			failed = e
		}
		break
	}

	if failed != nil {
		r.err = failed
		if total == 0 {
			return nil, 0, failed
		}
	}

	if hasNL && last == '\r' {
		total--
		if len(r.buf) > total {
			r.buf = r.buf[:total]
		}
	}
	if total > r.maxLine {
		dropped = total - r.maxLine
		r.buf = r.buf[:r.maxLine]
	}
	return r.buf, dropped, nil
}

// MayBlock reports whether the next call to Next may block on the underlying reader:
// true unless a complete line is already buffered or an error is pending.
func (r *Reader) MayBlock() bool {
	if r.err != nil {
		return false
	}
	p, _ := r.br.Peek(r.br.Buffered()) // peeking buffered bytes never blocks
	return bytes.IndexByte(p, '\n') < 0
}
