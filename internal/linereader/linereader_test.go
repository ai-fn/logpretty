package linereader

import (
	"bytes"
	"errors"
	"io"
	"slices"
	"strings"
	"testing"
	"testing/iotest"
)

type item struct {
	line    string
	dropped int
}

func readAll(t *testing.T, r *Reader) ([]item, error) {
	t.Helper()
	var got []item
	for range 10_000 {
		line, dropped, err := r.Next()
		if err != nil {
			if len(line) != 0 || dropped != 0 {
				t.Errorf("Next returned data %q (dropped %d) together with error %v", line, dropped, err)
			}
			return got, err
		}
		got = append(got, item{string(line), dropped})
	}
	t.Fatal("reader did not terminate")
	return nil, nil
}

func TestNext(t *testing.T) {
	x := func(n int) string { return strings.Repeat("x", n) }

	tests := []struct {
		name    string
		in      string
		maxLine int
		bufSize int // 0 means 16, the smallest bufio allows
		want    []item
	}{
		{name: "empty input", in: "", maxLine: 8},
		{name: "one line", in: "abc\n", maxLine: 8, want: []item{{"abc", 0}}},
		{name: "last line without newline", in: "abc", maxLine: 8, want: []item{{"abc", 0}}},
		{name: "two lines", in: "a\nb\n", maxLine: 8, want: []item{{"a", 0}, {"b", 0}}},
		{name: "empty lines", in: "\n\nabc\n", maxLine: 8, want: []item{{"", 0}, {"", 0}, {"abc", 0}}},
		{name: "crlf", in: "abc\r\n", maxLine: 8, want: []item{{"abc", 0}}},
		{name: "bare crlf", in: "\r\n", maxLine: 8, want: []item{{"", 0}}},
		{name: "cr in the middle", in: "a\rb\n", maxLine: 8, want: []item{{"a\rb", 0}}},
		{name: "cr at EOF is kept", in: "abc\r", maxLine: 8, want: []item{{"abc\r", 0}}},
		{name: "lone cr at EOF is kept", in: "\r", maxLine: 8, want: []item{{"\r", 0}}},
		{name: "only one cr is stripped", in: "a\r\r\n", maxLine: 8, want: []item{{"a\r", 0}}},
		{name: "invalid utf-8 and NUL pass through", in: "a\xffb\x00c\n", maxLine: 8, want: []item{{"a\xffb\x00c", 0}}},

		{name: "maxLine-1 with LF", in: x(7) + "\n", maxLine: 8, want: []item{{x(7), 0}}},
		{name: "maxLine with LF", in: x(8) + "\n", maxLine: 8, want: []item{{x(8), 0}}},
		{name: "maxLine+1 with LF", in: x(9) + "\n", maxLine: 8, want: []item{{x(8), 1}}},
		{name: "maxLine-1 with CRLF", in: x(7) + "\r\n", maxLine: 8, want: []item{{x(7), 0}}},
		{name: "maxLine with CRLF is not truncated", in: x(8) + "\r\n", maxLine: 8, want: []item{{x(8), 0}}},
		{name: "maxLine+1 with CRLF", in: x(9) + "\r\n", maxLine: 8, want: []item{{x(8), 1}}},
		{name: "maxLine at EOF without newline", in: x(8), maxLine: 8, want: []item{{x(8), 0}}},
		{name: "maxLine+1 at EOF without newline", in: x(9), maxLine: 8, want: []item{{x(8), 1}}},
		{name: "cr counts as dropped at EOF", in: x(9) + "\r", maxLine: 8, want: []item{{x(8), 2}}},
		{name: "long line spans many buffers", in: x(100) + "\nnext\n", maxLine: 8, want: []item{{x(8), 92}, {"next", 0}}},
		{name: "truncated line then CRLF line", in: x(30) + "\r\nab\r\n", maxLine: 8, want: []item{{x(8), 22}, {"ab", 0}}},

		{name: "truncated line with CRLF split across chunks", in: x(15) + "\r\nnext\n", maxLine: 8, want: []item{{x(8), 7}, {"next", 0}}},
		{name: "full buffer then CRLF", in: x(16) + "\r\n", maxLine: 100, want: []item{{x(16), 0}}},
		{name: "line fills the buffer minus terminator", in: x(15) + "\nnext\n", maxLine: 100, want: []item{{x(15), 0}, {"next", 0}}},
		{name: "line fills the buffer exactly", in: x(16) + "\nnext\n", maxLine: 100, want: []item{{x(16), 0}, {"next", 0}}},
		{name: "line exceeds the buffer by one", in: x(17) + "\nnext\n", maxLine: 100, want: []item{{x(17), 0}, {"next", 0}}},
		{name: "cr ends one chunk and lf starts the next", in: x(15) + "\r\nnext\n", maxLine: 100, want: []item{{x(15), 0}, {"next", 0}}},
		{name: "cr at EOF after a full buffer", in: x(15) + "\r", maxLine: 100, want: []item{{x(15) + "\r", 0}}},
		{name: "default-sized buffer", in: x(70_000) + "\nnext\n", maxLine: 100_000, bufSize: 64 << 10, want: []item{{x(70_000), 0}, {"next", 0}}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			bufSize := tt.bufSize
			if bufSize == 0 {
				bufSize = 16
			}
			got, err := readAll(t, newReader(strings.NewReader(tt.in), bufSize, tt.maxLine))

			if !errors.Is(err, io.EOF) {
				t.Errorf("final error = %v, want io.EOF", err)
			}
			if !slices.Equal(got, tt.want) {
				t.Errorf("lines = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestNextHugeLine(t *testing.T) {
	const size = 10 << 20
	in := strings.Repeat("x", size) + "\nnext\n"

	got, err := readAll(t, New(strings.NewReader(in), DefaultMaxLine))

	if !errors.Is(err, io.EOF) {
		t.Fatalf("final error = %v, want io.EOF", err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d lines, want 2", len(got))
	}
	if len(got[0].line) != DefaultMaxLine || got[0].dropped != size-DefaultMaxLine {
		t.Errorf("huge line: kept %d, dropped %d; want %d, %d", len(got[0].line), got[0].dropped, DefaultMaxLine, size-DefaultMaxLine)
	}
	if got[1] != (item{"next", 0}) {
		t.Errorf("line after the huge one = %+v, want next", got[1])
	}
}

func TestNextNonPositiveMaxLine(t *testing.T) {
	got, _ := readAll(t, New(strings.NewReader("abc\n"), 0))

	if !slices.Equal(got, []item{{"abc", 0}}) {
		t.Errorf("lines = %+v, want abc kept under the default limit", got)
	}
}

func TestNextErrors(t *testing.T) {
	boom := errors.New("boom")

	t.Run("immediate error is sticky", func(t *testing.T) {
		r := New(iotest.ErrReader(boom), DefaultMaxLine)
		for range 3 {
			if _, _, err := r.Next(); !errors.Is(err, boom) {
				t.Fatalf("err = %v, want boom", err)
			}
		}
	})

	t.Run("data before the error is delivered first", func(t *testing.T) {
		r := newReader(io.MultiReader(strings.NewReader("a\nb"), iotest.ErrReader(boom)), 16, 8)

		got, err := readAll(t, r)

		if !errors.Is(err, boom) {
			t.Errorf("final error = %v, want boom", err)
		}
		if want := []item{{"a", 0}, {"b", 0}}; !slices.Equal(got, want) {
			t.Errorf("lines = %+v, want %+v", got, want)
		}
		if _, _, err := r.Next(); !errors.Is(err, boom) {
			t.Errorf("error after the first report = %v, want boom again", err)
		}
	})

	t.Run("EOF is sticky", func(t *testing.T) {
		r := New(strings.NewReader("a\n"), DefaultMaxLine)
		_, _ = readAll(t, r)
		if _, _, err := r.Next(); !errors.Is(err, io.EOF) {
			t.Errorf("err = %v, want io.EOF", err)
		}
	})
}

func TestNextReusesBuffer(t *testing.T) {
	r := New(strings.NewReader("aaa\nbbb\n"), DefaultMaxLine)

	first, _, _ := r.Next()
	kept := bytes.Clone(first)
	if _, _, err := r.Next(); err != nil {
		t.Fatal(err)
	}

	if string(first) != "bbb" {
		t.Errorf("previous slice = %q, want it to be overwritten with bbb", first)
	}
	if string(kept) != "aaa" {
		t.Errorf("cloned slice = %q, want aaa", kept)
	}
}

func TestMayBlock(t *testing.T) {
	boom := errors.New("boom")

	t.Run("empty buffer", func(t *testing.T) {
		r := newReader(strings.NewReader("a\n"), 16, 8)
		if !r.MayBlock() {
			t.Error("MayBlock = false before anything is read, want true")
		}
	})

	t.Run("complete line buffered", func(t *testing.T) {
		r := newReader(strings.NewReader("a\nb\n"), 16, 8)
		_, _, _ = r.Next()
		if r.MayBlock() {
			t.Error("MayBlock = true with a complete line buffered, want false")
		}
	})

	t.Run("partial line buffered", func(t *testing.T) {
		r := newReader(strings.NewReader("a\nb"), 16, 8)
		_, _, _ = r.Next()
		if !r.MayBlock() {
			t.Error("MayBlock = false with only a partial line buffered, want true")
		}
	})

	t.Run("everything consumed", func(t *testing.T) {
		r := newReader(strings.NewReader("a\n"), 16, 8)
		_, _, _ = r.Next()
		if !r.MayBlock() {
			t.Error("MayBlock = false with an empty buffer, want true")
		}
	})

	t.Run("deferred error", func(t *testing.T) {
		r := newReader(io.MultiReader(strings.NewReader("a"), iotest.ErrReader(boom)), 16, 8)
		if line, _, err := r.Next(); err != nil || string(line) != "a" {
			t.Fatalf("Next = %q, %v; want a, nil", line, err)
		}
		if r.MayBlock() {
			t.Error("MayBlock = true with an error pending, want false")
		}
	})
}

func TestNextDoesNotAllocate(t *testing.T) {
	tests := []struct {
		name    string
		line    string
		bufSize int
		maxLine int
	}{
		{name: "short lines", line: "hello world\n", bufSize: bufferSize, maxLine: DefaultMaxLine},
		{name: "CRLF lines", line: "hello world\r\n", bufSize: bufferSize, maxLine: DefaultMaxLine},
		{name: "truncated lines across refills", line: strings.Repeat("x", 100) + "\r\n", bufSize: 16, maxLine: 32},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := newReader(strings.NewReader(strings.Repeat(tt.line, 1000)), tt.bufSize, tt.maxLine)
			for range 10 {
				_, _, _ = r.Next()
			}

			if allocs := testing.AllocsPerRun(100, func() { _, _, _ = r.Next() }); allocs != 0 {
				t.Errorf("Next allocates %v times per call, want 0", allocs)
			}
		})
	}
}
