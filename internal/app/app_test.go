package app

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"io"
	"io/fs"
	"math/rand/v2"
	"runtime"
	"strings"
	"sync"
	"testing"
	"testing/iotest"
	"testing/synctest"
	"time"
)

type failWriter struct{}

func (failWriter) Write([]byte) (int, error) { return 0, errors.New("disk full") }

// syncWriter is a buffer safe to read while Run is writing; it signals its first write.
type syncWriter struct {
	mu    sync.Mutex
	buf   bytes.Buffer
	wrote chan struct{}
}

func (w *syncWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	n, err := w.buf.Write(p)
	select {
	case w.wrote <- struct{}{}:
	default:
	}
	return n, err
}

func (w *syncWriter) String() string {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.buf.String()
}

// A fixed zone, because on a UTC machine a mixed-up zone would go unnoticed.
// Not for parallel tests: it swaps time.Local.
func setLocalZone(t *testing.T) {
	t.Helper()
	old := time.Local
	time.Local = time.FixedZone("test", 3*60*60)
	t.Cleanup(func() { time.Local = old })
}

func TestRun(t *testing.T) {
	setLocalZone(t)
	x := func(n int) string { return strings.Repeat("x", n) }

	tests := []struct {
		name       string
		args       []string
		in         string
		wantOut    string
		wantCode   int
		wantStderr string
	}{
		{name: "empty input"},
		{name: "each format is rendered", in: "{\"id\":1}\nlevel=warn msg=x\nplain text\n", wantOut: "id=1\nWARN  x\nplain text\n"},
		{
			name:    "utc shows the time in UTC",
			args:    []string{"-utc"},
			in:      `{"time":"2026-10-11T12:00:00.5+03:00","level":"info","msg":"hi","n":1}` + "\n",
			wantOut: "09:00:00.500 INFO  hi  n=1\n",
		},
		{name: "time is shown in the local zone", in: `{"time":"2026-10-11T12:00:00Z","msg":"hi"}` + "\n", wantOut: "15:00:00.000 hi\n"},
		{name: "time without a zone is local", in: `{"time":"2026-10-11 12:00:00","msg":"hi"}` + "\n", wantOut: "12:00:00.000 hi\n"},
		{
			name:    "utc reads a time without a zone as UTC",
			args:    []string{"-utc"},
			in:      `{"time":"2026-10-11 12:00:00","msg":"hi"}` + "\n",
			wantOut: "12:00:00.000 hi\n",
		},
		{name: "control characters are written out", in: "a\x1b[31mb\n{\"msg\":\"x\\u001b]0;t\\u0007\"}\n", wantOut: `a\x1b[31mb` + "\n" + `x\x1b]0;t\x07` + "\n"},
		{name: "byte order mark is dropped", in: "\xef\xbb\xbf{\"msg\":\"x\"}\n", wantOut: "x\n"},
		{name: "blank lines are kept", in: "\n\nx\n\n", wantOut: "\n\nx\n\n"},
		{name: "last line gets a newline", in: "abc", wantOut: "abc\n"},
		{name: "crlf becomes lf", in: "one\r\ntwo\r\n", wantOut: "one\ntwo\n"},
		{name: "cr inside a line is written out", in: "a\rb\n", wantOut: `a\rb` + "\n"},
		{name: "cr at EOF is written out", in: "abc\r", wantOut: `abc\r` + "\n"},

		{name: "max-line marks the dropped bytes", args: []string{"-max-line", "8"}, in: x(10) + "\nok\n", wantOut: x(8) + "…[+2B]\nok\n"},
		{name: "line of exactly max-line is not marked", args: []string{"-max-line", "8"}, in: x(8) + "\n", wantOut: x(8) + "\n"},
		{name: "line of exactly max-line with CRLF is not marked", args: []string{"-max-line", "8"}, in: x(8) + "\r\n", wantOut: x(8) + "\n"},
		{name: "one byte over max-line", args: []string{"-max-line", "8"}, in: x(9) + "\r\n", wantOut: x(8) + "…[+1B]\n"},
		{name: "max-line applies to the unterminated last line", args: []string{"-max-line", "4"}, in: "0123456789", wantOut: "0123…[+6B]\n"},
		{name: "max-line with an equals sign", args: []string{"-max-line=3"}, in: "abcdef\n", wantOut: "abc…[+3B]\n"},
		{name: "a truncated line is not parsed", args: []string{"-max-line", "8"}, in: `{"msg":"a"}` + "\n", wantOut: `{"msg":"…[+3B]` + "\n"},
		{name: "a line of exactly max-line is parsed", args: []string{"-max-line", "11"}, in: `{"msg":"a"}` + "\n", wantOut: "a\n"},

		{name: "huge max-line allocates nothing up front", args: []string{"-max-line=9223372036854775807"}, in: "abc\n", wantOut: "abc\n"},
		{name: "double dash ends the flags", args: []string{"--"}, in: "abc\n", wantOut: "abc\n"},
		{name: "positional argument after double dash", args: []string{"--", "file.log"}, wantCode: 2, wantStderr: "unexpected argument"},
		{name: "unknown flag", args: []string{"-nope"}, wantCode: 2, wantStderr: "flag provided but not defined"},
		{name: "positional argument", args: []string{"file.log"}, wantCode: 2, wantStderr: "unexpected argument"},
		{name: "max-line zero", args: []string{"-max-line", "0"}, wantCode: 2, wantStderr: "-max-line must be positive"},
		{name: "max-line negative", args: []string{"-max-line", "-5"}, wantCode: 2, wantStderr: "-max-line must be positive"},
		{name: "max-line not a number", args: []string{"-max-line", "abc"}, wantCode: 2, wantStderr: "invalid value"},
		{name: "help", args: []string{"-h"}, wantStderr: "Usage of logpretty"},
		{name: "utc takes no value", args: []string{"-utc", "yes"}, wantCode: 2, wantStderr: "unexpected argument"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			code := Run(t.Context(), tt.args, strings.NewReader(tt.in), &stdout, &stderr)

			if code != tt.wantCode {
				t.Errorf("exit code = %d, want %d (stderr: %q)", code, tt.wantCode, stderr.String())
			}
			if stdout.String() != tt.wantOut {
				t.Errorf("stdout = %q, want %q", stdout.String(), tt.wantOut)
			}
			if tt.wantStderr == "" && stderr.Len() != 0 {
				t.Errorf("stderr = %q, want empty", stderr.String())
			}
			if !strings.Contains(stderr.String(), tt.wantStderr) {
				t.Errorf("stderr = %q, want it to contain %q", stderr.String(), tt.wantStderr)
			}
		})
	}
}

func TestRunDefaultMaxLine(t *testing.T) {
	const limit = 1 << 20
	var stdout bytes.Buffer

	code := Run(t.Context(), nil, strings.NewReader(strings.Repeat("x", limit+5)+"\nnext\n"), &stdout, io.Discard)

	want := strings.Repeat("x", limit) + "…[+5B]\nnext\n"
	if code != 0 || stdout.String() != want {
		t.Errorf("code = %d, output of %d bytes (tail %q); want 0 and %d bytes ending in the marker",
			code, stdout.Len(), stdout.String()[max(0, stdout.Len()-12):], len(want))
	}
}

// Plain lines come out as they went in, whatever the sizes of the reads and of the batches.
func TestRunMatchesCatOnPlainText(t *testing.T) {
	rng := rand.New(rand.NewPCG(1, 2))
	// No equals sign and no colon: without them a line is neither logfmt nor a JSON object with a pair.
	const alphabet = "abc XYZ 012 \t{}[]\",_-./"
	var in strings.Builder
	for range 20_000 {
		for range rng.IntN(200) {
			in.WriteByte(alphabet[rng.IntN(len(alphabet))])
		}
		in.WriteByte('\n')
	}

	// One byte at a time flushes after every line; the plain reader exercises batching across the write buffer.
	readers := map[string]func(io.Reader) io.Reader{
		"one byte per read": iotest.OneByteReader,
		"half reads":        iotest.HalfReader,
		"whole reads":       func(r io.Reader) io.Reader { return r },
	}
	for name, wrap := range readers {
		t.Run(name, func(t *testing.T) {
			var stdout bytes.Buffer

			code := Run(t.Context(), nil, wrap(strings.NewReader(in.String())), &stdout, io.Discard)

			if code != 0 || stdout.String() != in.String() {
				t.Errorf("code = %d, output differs from input (%d vs %d bytes)", code, stdout.Len(), in.Len())
			}
		})
	}
}

func TestWriteDoesNotAllocate(t *testing.T) {
	lines := []string{
		"some log line",
		`time=2026-10-11T12:00:00Z level=info msg="request \"done\"" status=200`,
		`{"time":"2026-10-11T12:00:00.123+03:00","level":"info","msg":"request done","status":200}`,
		"a line with \x1b and \xff",
	}
	// nil is the zone of the machine, which is what runs without -utc.
	for _, loc := range []*time.Location{nil, time.UTC} {
		p := &pipeline{w: bufio.NewWriterSize(io.Discard, writeBufferSize)}
		p.parser.Loc, p.renderer.Loc = loc, loc

		for _, line := range lines {
			if raceEnabled && strings.HasPrefix(line, "{") {
				continue
			}
			b := []byte(line)
			for _, dropped := range []int{0, 12345} {
				if err := p.write(b, dropped); err != nil { // the first call grows the buffers
					t.Fatal(err)
				}
				allocs := testing.AllocsPerRun(100, func() {
					if err := p.write(b, dropped); err != nil {
						t.Fatal(err)
					}
				})
				if allocs != 0 {
					t.Errorf("zone %v: write of %q with %d dropped bytes allocates %v times per call, want 0", loc, line, dropped, allocs)
				}
			}
		}
	}
}

func TestRunIOErrors(t *testing.T) {
	boom := errors.New("boom")

	tests := []struct {
		name       string
		in         io.Reader
		out        io.Writer
		wantOut    string
		wantStderr string
	}{
		{name: "read error", in: iotest.ErrReader(boom), wantStderr: "read:"},
		{
			name:       "read error after data keeps earlier output",
			in:         io.MultiReader(strings.NewReader("hello\n"), iotest.ErrReader(boom)),
			wantOut:    "hello\n",
			wantStderr: "read:",
		},
		{
			name:       "read error mid-line keeps the partial line",
			in:         io.MultiReader(strings.NewReader("hello"), iotest.ErrReader(boom)),
			wantOut:    "hello\n",
			wantStderr: "read:",
		},
		{
			name:       "write error on a complete line wins over a later read error",
			in:         io.MultiReader(strings.NewReader("hello\n"), iotest.ErrReader(boom)),
			out:        failWriter{},
			wantStderr: "write:",
		},
		{
			name:       "write error wins over a read error that ends a partial line",
			in:         io.MultiReader(strings.NewReader("hello"), iotest.ErrReader(boom)),
			out:        failWriter{},
			wantStderr: "write:",
		},
		{name: "write error", in: strings.NewReader("hello\n"), out: failWriter{}, wantStderr: "write:"},
		{name: "write error on a line larger than the write buffer", in: strings.NewReader(strings.Repeat("x", 100_000) + "\n"), out: failWriter{}, wantStderr: "write:"},
		{name: "write error on the final flush", in: strings.NewReader("hello"), out: failWriter{}, wantStderr: "write:"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			out := tt.out
			if out == nil {
				out = &stdout
			}

			code := Run(t.Context(), nil, tt.in, out, &stderr)

			if code != 1 {
				t.Errorf("exit code = %d, want 1", code)
			}
			if !strings.Contains(stderr.String(), tt.wantStderr) {
				t.Errorf("stderr = %q, want it to contain %q", stderr.String(), tt.wantStderr)
			}
			if stdout.String() != tt.wantOut {
				t.Errorf("stdout = %q, want %q", stdout.String(), tt.wantOut)
			}
		})
	}
}

// Output must be flushed before the next read blocks, even when a partial line is buffered.
func TestRunFlushesBeforeBlockingRead(t *testing.T) {
	pr, pw := io.Pipe()
	defer func() { _ = pr.Close() }()
	defer func() { _ = pw.Close() }()
	out := &syncWriter{wrote: make(chan struct{}, 1)}
	done := make(chan int, 1)
	go func() { done <- Run(t.Context(), nil, pr, out, io.Discard) }()
	go func() { _, _ = pw.Write([]byte("a\nb")) }()

	select {
	case <-out.wrote:
	case <-time.After(5 * time.Second):
		t.Fatal("output was not flushed while the reader was blocked on a partial line")
	}
	if got := out.String(); got != "a\n" {
		t.Errorf("first flush = %q, want only the complete line", got)
	}

	if err := pw.Close(); err != nil {
		t.Fatal(err)
	}
	if code := <-done; code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	if got := out.String(); got != "a\nb\n" {
		t.Errorf("final output = %q, want %q", got, "a\nb\n")
	}
}

// gateWriter holds every write until release is closed, so a test can act while a write is in flight.
type gateWriter struct {
	out     syncWriter
	release chan struct{}
}

func (w *gateWriter) Write(p []byte) (int, error) {
	<-w.release
	return w.out.Write(p)
}

func TestRunStopsWhenContextIsCanceled(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		pr, pw := io.Pipe()
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		var out, stderr syncWriter
		done := make(chan int, 1)
		go func() { done <- Run(ctx, nil, pr, &out, &stderr) }()

		if _, err := pw.Write([]byte("a\nb")); err != nil {
			t.Fatal(err)
		}
		synctest.Wait() // Run is now blocked reading the rest of line "b"
		cancel()

		if code := <-done; code != ExitInterrupted {
			t.Errorf("exit code = %d, want %d", code, ExitInterrupted)
		}
		if got := out.String(); got != "a\n" {
			t.Errorf("output at exit = %q, want %q", got, "a\n")
		}

		// Input that arrives after the stop must not reach stdout.
		if _, err := pw.Write([]byte("\nc\n")); err != nil {
			t.Fatal(err)
		}
		if err := pw.Close(); err != nil {
			t.Fatal(err)
		}
		synctest.Wait()
		if got := out.String(); got != "a\n" {
			t.Errorf("output after exit = %q, want it unchanged", got)
		}
		if got := stderr.String(); got != "" {
			t.Errorf("stderr = %q, want empty", got)
		}
	})
}

// A cancellation that lands while lines are being written waits for them: no output is lost or torn.
func TestRunFinishesWriteInFlightBeforeStopping(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		pr, pw := io.Pipe()
		defer func() { _ = pw.Close() }()
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		out := &gateWriter{release: make(chan struct{})}
		done := make(chan int, 1)
		go func() { done <- Run(ctx, nil, pr, out, io.Discard) }()

		if _, err := pw.Write([]byte("a\nb\n")); err != nil {
			t.Fatal(err)
		}
		synctest.Wait() // both lines are rendered and their write to stdout is held by the gate
		if got := out.out.String(); got != "" {
			t.Fatalf("output before release = %q, want empty", got)
		}
		cancel()
		runtime.Gosched() // lets Run reach stop() while the write is still held; best effort, not a guarantee
		close(out.release)

		if code := <-done; code != ExitInterrupted {
			t.Errorf("exit code = %d, want %d", code, ExitInterrupted)
		}
		if got := out.out.String(); got != "a\nb\n" {
			t.Errorf("output at exit = %q, want %q", got, "a\nb\n")
		}
	})
}

// When the reader of stdout goes away the stream has nowhere to go: that is not an error to report.
func TestRunExitsQuietlyWhenStdoutIsClosed(t *testing.T) {
	if len(closedPipeErrs) == 0 {
		t.Fatal("no closed-pipe errors are defined for this platform")
	}
	inputs := map[string]string{
		"write fails on flush":                      "a\nb\n",
		"write fails while a long line is rendered": strings.Repeat("x", 2*writeBufferSize) + "\n",
	}
	for _, cause := range closedPipeErrs {
		for name, in := range inputs {
			t.Run(cause.Error()+"/"+name, func(t *testing.T) {
				out := writerFunc(func([]byte) (int, error) {
					return 0, &fs.PathError{Op: "write", Path: "/dev/stdout", Err: cause}
				})
				var stderr bytes.Buffer

				code := Run(t.Context(), nil, strings.NewReader(in), out, &stderr)

				if code != 0 || stderr.Len() != 0 {
					t.Errorf("code = %d, stderr = %q; want 0 and nothing on stderr", code, stderr.String())
				}
			})
		}
	}
}

type writerFunc func([]byte) (int, error)

func (f writerFunc) Write(p []byte) (int, error) { return f(p) }
