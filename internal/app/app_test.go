package app

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"io"
	"io/fs"
	"math"
	"math/rand/v2"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"testing/iotest"
	"testing/synctest"
	"time"

	"github.com/ai-fn/logpretty/internal/cli"
	"github.com/ai-fn/logpretty/internal/entry"
	"github.com/ai-fn/logpretty/internal/filter"
	"github.com/ai-fn/logpretty/internal/render"
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
		{name: "max-line with a unit", args: []string{"--max-line", "1K"}, in: x(1030) + "\nok\n", wantOut: x(1024) + "…[+6B]\nok\n"},
		{name: "max-line with an equals sign", args: []string{"-max-line=3"}, in: "abcdef\n", wantOut: "abc…[+3B]\n"},
		{name: "a truncated line is not parsed", args: []string{"-max-line", "8"}, in: `{"msg":"a"}` + "\n", wantOut: `{"msg":"…[+3B]` + "\n"},
		{name: "a line of exactly max-line is parsed", args: []string{"-max-line", "11"}, in: `{"msg":"a"}` + "\n", wantOut: "a\n"},

		{
			name: "level keeps the entries of that level and above, with the plain lines after them",
			args: []string{"--level", "warn"},
			in: "banner\n" + `{"level":"info","msg":"started"}` + "\n  after info\n" + `{"level":"error","msg":"failed"}` + "\npanic: boom\n\tmain.go:30\n" +
				"level=warn msg=slow\n" + `{"msg":"no level"}` + "\nafter no level\n",
			wantOut: "ERROR failed\npanic: boom\n\tmain.go:30\nWARN  slow\n",
		},
		{name: "level as a log spells it", args: []string{"--level=WARNING"}, in: "level=info msg=a\nlevel=warn msg=b\n", wantOut: "WARN  b\n"},
		{name: "level that nothing passes leaves the output empty", args: []string{"--level", "fatal"}, in: "level=error msg=a\ntext\n"},
		{
			name:    "a cut line goes the way of the entry before it",
			args:    []string{"--level", "warn", "--max-line", "20"},
			in:      x(30) + "\nlevel=warn msg=a\n" + x(30) + "\nlevel=info msg=b\n" + x(30) + "\n",
			wantOut: "WARN  a\n" + x(20) + "…[+10B]\n",
		},

		{
			name: "where keeps the entries that meet it, with the plain lines after them",
			args: []string{"--where", "user_id=42"},
			in: "banner\n" + `{"msg":"a","user_id":42}` + "\n  after a\n" + `{"msg":"b","user_id":7}` + "\n  after b\n" +
				"msg=c user_id=42\n" + `{"msg":"no user"}` + "\nafter no user\n",
			wantOut: "a  user_id=42\n  after a\nc  user_id=42\n",
		},
		{
			name:    "where given twice asks for both",
			args:    []string{"--where", "user_id=42", "--where", "http.status>=500"},
			in:      `{"msg":"a","user_id":42,"http":{"status":502}}` + "\n" + `{"msg":"b","user_id":42,"http":{"status":200}}` + "\n" + `{"msg":"c","user_id":7,"http":{"status":502}}` + "\n",
			wantOut: `a  user_id=42 http={"status":502}` + "\n",
		},
		{
			name:    "where and level together",
			args:    []string{"--level", "warn", "--where", "msg~^slow"},
			in:      "level=warn msg=\"slow request\"\nlevel=info msg=\"slow start\"\nlevel=error msg=failed\n",
			wantOut: "WARN  slow request\n",
		},
		{name: "where that nothing meets leaves the output empty", args: []string{"--where", "user_id"}, in: "level=error msg=a\ntext\n"},
		{
			name:    "where reads a time without a zone as local",
			args:    []string{"--where", "time>=2026-10-11T12:00:00"},
			in:      `{"time":"2026-10-11T10:00:00Z","msg":"early"}` + "\n" + `{"time":"2026-10-11T12:30:00Z","msg":"late"}` + "\n",
			wantOut: "13:00:00.000 early\n15:30:00.000 late\n",
		},
		{
			name:    "utc after where still sets the zone of its time",
			args:    []string{"--where", "time>=2026-10-11T12:00:00", "--utc"},
			in:      `{"time":"2026-10-11T10:00:00Z","msg":"early"}` + "\n" + `{"time":"2026-10-11T12:30:00Z","msg":"late"}` + "\n",
			wantOut: "12:30:00.000 late\n",
		},

		{
			name:    "plain show keeps the plain lines whatever the filter",
			args:    []string{"--level", "error", "--plain", "show"},
			in:      "banner\nlevel=info msg=a\n  after a\nlevel=error msg=b\n  after b\n",
			wantOut: "banner\n  after a\nERROR b\n  after b\n",
		},
		{
			name:    "plain hide drops the plain lines even without a filter",
			args:    []string{"--plain=hide"},
			in:      "banner\n\nlevel=info msg=a\n  after a\n{}\nlevel=error msg=b\n",
			wantOut: "INFO  a\n{}\nERROR b\n",
		},
		{
			name:    "plain follow is what happens without the flag",
			args:    []string{"--level", "error", "--plain", "follow"},
			in:      "banner\nlevel=info msg=a\n  after a\nlevel=error msg=b\n  after b\n",
			wantOut: "ERROR b\n  after b\n",
		},
		{
			name:    "a cut line is hidden by plain hide",
			args:    []string{"--plain", "hide", "--max-line", "20"},
			in:      x(30) + "\nlevel=warn msg=a\n" + x(30) + "\n",
			wantOut: "WARN  a\n",
		},
		{
			name:    "a cut line is shown by plain show",
			args:    []string{"--plain", "show", "--level", "error", "--max-line", "20"},
			in:      x(30) + "\nlevel=warn msg=a\n" + x(30) + "\n",
			wantOut: x(20) + "…[+10B]\n" + x(20) + "…[+10B]\n",
		},

		{
			name:    "fields choose what is shown and in which order",
			args:    []string{"--utc", "--fields", "latency,msg"},
			in:      `{"time":"2026-10-11T12:00:00Z","level":"warn","msg":"slow","user_id":42,"latency":"1.2s"}` + "\n",
			wantOut: "12:00:00.000 WARN  slow  latency=1.2s\n",
		},
		{
			name:    "an entry with none of the fields is not printed, the lines that are not entries are",
			args:    []string{"--fields", "latency"},
			in:      "banner\n" + `{"level":"info","msg":"ok"}` + "\n  after ok\n" + `{"level":"warn","latency":"1.2s"}` + "\n{}\n" + x(1030) + "\n",
			wantOut: "banner\n  after ok\nWARN  latency=1.2s\n{}\n" + x(1030) + "\n",
		},
		{
			name:    "fields do not change what a filter decides for the plain lines",
			args:    []string{"--level", "error", "--fields", "latency"},
			in:      `{"level":"error","msg":"boom"}` + "\npanic: boom\n" + `{"level":"info","latency":"1s"}` + "\n  after info\n",
			wantOut: "panic: boom\n",
		},
		{name: "fields given twice add up", args: []string{"--fields", "b", "--fields", "a,b"}, in: `{"a":1,"b":2,"c":3}` + "\n", wantOut: "b=2 a=1\n"},
		{
			name:    "the filters and the fields together",
			args:    []string{"--level", "warn", "--where", "user_id=42", "--fields", "msg,latency"},
			in:      `{"time":"2026-10-11 12:00:00","level":"warn","msg":"slow","user_id":42,"latency":"1.2s"}` + "\n" + `{"level":"info","msg":"ok","user_id":42}` + "\n  at main.go:30\n",
			wantOut: "12:00:00.000 WARN  slow  latency=1.2s\n",
		},
		{
			name:    "no-time leaves the time out",
			args:    []string{"--no-time"},
			in:      `{"time":"2026-10-11T12:00:00Z","level":"info","msg":"hi","n":1}` + "\n" + `{"time":"2026-10-11T12:00:01Z"}` + "\nplain\n",
			wantOut: "INFO  hi  n=1\nplain\n",
		},
		{
			name:    "no-time with fields",
			args:    []string{"--no-time", "--fields", "n,time,msg"},
			in:      `{"time":"2026-10-11T12:00:00Z","level":"info","msg":"hi","n":1}` + "\n",
			wantOut: "INFO  hi  n=1\n",
		},
		{
			name:    "color with fields",
			args:    []string{"--color=always", "--utc", "--fields", "ms"},
			in:      `{"time":"2026-10-11T12:00:00Z","level":"warn","msg":"slow","ms":120}` + "\n",
			wantOut: "\x1b[2m12:00:00.000\x1b[0m \x1b[33mWARN\x1b[0m  \x1b[2mms=\x1b[0m120\n",
		},
		{name: "a cut line is shown whatever the fields are", args: []string{"--fields", "a", "--max-line", "8"}, in: `{"a":1,"b":"long"}` + "\n", wantOut: `{"a":1,"…[+10B]` + "\n"},

		{name: "huge max-line allocates nothing up front", args: []string{"--max-line=" + strconv.Itoa(math.MaxInt)}, in: "abc\n", wantOut: "abc\n"},
		{name: "double dash ends the flags", args: []string{"--"}, in: "abc\n", wantOut: "abc\n"},

		{
			name:    "color always",
			args:    []string{"-color=always", "-utc"},
			in:      `{"time":"2026-10-11T12:00:00Z","level":"warn","msg":"slow","ms":120}` + "\nplain\n",
			wantOut: "\x1b[2m12:00:00.000\x1b[0m \x1b[33mWARN\x1b[0m  slow  \x1b[2mms=\x1b[0m120\nplain\n",
		},
		{name: "color always marks a cut line", args: []string{"-color", "always", "-max-line", "3"}, in: "abcdef\n", wantOut: "abc\x1b[33m…[+3B]\x1b[0m\n"},
		{name: "color always still writes control characters out", args: []string{"-color=always"}, in: "a\x1b[31mb\n", wantOut: `a\x1b[31mb` + "\n"},
		{name: "color never", args: []string{"-color=never"}, in: "level=warn msg=x\n", wantOut: "WARN  x\n"},
		{name: "the last color wins", args: []string{"-color=always", "-color=never"}, in: "level=warn msg=x\n", wantOut: "WARN  x\n"},
		{name: "color auto does not color what is not a terminal", args: []string{"-color=auto"}, in: "level=warn msg=x\n", wantOut: "WARN  x\n"},
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

// The Linux console knows no 256 colors: trace ids get the basic ones there. Which color an id gets is tested in render.
func TestRunTraceColors(t *testing.T) {
	const in = `{"msg":"x","trace_id":"4bf9"}` + "\n"
	tests := []struct {
		name, term string
		palette    []string
	}{
		{name: "256 colors", term: "xterm-256color", palette: render.DefaultTheme().Traces},
		{name: "no TERM", palette: render.DefaultTheme().Traces},
		{name: "the Linux console", term: "linux", palette: render.BasicTracePalette()},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("TERM", tt.term)
			var stdout, stderr bytes.Buffer
			if code := Run(t.Context(), []string{"--color=always"}, strings.NewReader(in), &stdout, &stderr); code != 0 {
				t.Fatalf("exit code = %d, want 0 (stderr: %q)", code, stderr.String())
			}
			out := stdout.String()
			if !slices.ContainsFunc(tt.palette, func(c string) bool { return out == "x  \x1b[2mtrace_id=\x1b[0m"+c+"4bf9\x1b[0m\n" }) {
				t.Errorf("stdout = %q, want the id in one of the colors %q", out, tt.palette)
			}
		})
	}
}

// The shape of a failure: one line, or two for an expression, then the hint. The arguments are tested in internal/cli.
func TestRunArgumentErrors(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{name: "unknown flag", args: []string{"--nope"}, want: "flag provided but not defined: -nope"},
		{name: "positional argument", args: []string{"file.log"}, want: `unexpected argument "file.log"`},
		{name: "positional argument after double dash", args: []string{"--", "file.log"}, want: `unexpected argument "file.log"`},
		{name: "max-line zero", args: []string{"--max-line", "0"}, want: `invalid value "0" for flag -max-line: want a positive size in bytes, such as 4096, 64K or 1M`},
		{name: "color of an unknown kind", args: []string{"--color=sometimes"}, want: `invalid value "sometimes" for flag -color: invalid color mode "sometimes": want auto, always or never`},
		{name: "level of an unknown name", args: []string{"--level", "loud"}, want: `invalid value "loud" for flag -level: invalid level "loud": want trace, debug, info, warn, error or fatal`},
		{name: "control characters in an argument", args: []string{"--\x1b[31m"}, want: `flag provided but not defined: -\x1b[31m`},
		{name: "plain of an unknown kind", args: []string{"--plain", "all"}, want: `invalid value "all" for flag -plain: invalid plain mode "all": want follow, show or hide`},
		{name: "fields with a name left out", args: []string{"--fields", "a,,b"}, want: `invalid value "a,,b" for flag -fields: a field has no name; the names are separated by commas, as in msg,latency`},
		{name: "trace with no id", args: []string{"--trace="}, want: `invalid value "" for flag -trace: the trace id is empty`},
		{name: "where expression", args: []string{"--where", "user_id=>42"}, want: "--where 'user_id=>42'\n" + strings.Repeat(" ", 28) + "^ unexpected '>'"},
		{
			name: "control characters in a where expression",
			args: []string{"--where", "a\x1b[31m=>1"},
			want: `--where 'a\x1b[31m=>1'` + "\n" + strings.Repeat(" ", 30) + "^ unexpected '>'",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer

			code := Run(t.Context(), tt.args, iotest.ErrReader(errors.New("boom")), &stdout, &stderr)

			want := "logpretty: " + tt.want + "\nRun 'logpretty --help' for usage.\n"
			if code != 2 || stdout.Len() != 0 || stderr.String() != want {
				t.Errorf("code = %d, stdout = %q, stderr = %q; want 2, no output and %q", code, stdout.String(), stderr.String(), want)
			}
		})
	}
}

// The help and the version are all that is asked for: the input is not read.
func TestRunInfo(t *testing.T) {
	tests := []struct {
		arg  string
		want string
	}{
		{arg: "-h", want: cli.Usage},
		{arg: "--help", want: cli.Usage},
		{arg: "--version", want: cli.Version() + "\n"},
	}
	for _, tt := range tests {
		t.Run(tt.arg, func(t *testing.T) {
			var stdout, stderr bytes.Buffer

			code := Run(t.Context(), []string{tt.arg}, iotest.ErrReader(errors.New("boom")), &stdout, &stderr)

			if code != 0 || stdout.String() != tt.want || stderr.Len() != 0 {
				t.Errorf("code = %d, stdout = %q, stderr = %q; want 0, %q and nothing on stderr", code, stdout.String(), stderr.String(), tt.want)
			}
		})
	}
}

func TestRunInfoWriteErrors(t *testing.T) {
	for _, arg := range []string{"--help", "--version"} {
		var stderr bytes.Buffer
		if code := Run(t.Context(), []string{arg}, strings.NewReader(""), failWriter{}, &stderr); code != 1 || !strings.Contains(stderr.String(), "write:") {
			t.Errorf("%s into a failing stdout: code = %d, stderr = %q; want 1 and a write error", arg, code, stderr.String())
		}

		for _, cause := range closedPipeErrs {
			stderr.Reset()
			out := writerFunc(func([]byte) (int, error) { return 0, cause })
			if code := Run(t.Context(), []string{arg}, strings.NewReader(""), out, &stderr); code != 0 || stderr.Len() != 0 {
				t.Errorf("%s into a closed stdout (%v): code = %d, stderr = %q; want 0 and nothing", arg, cause, code, stderr.String())
			}
		}
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
		// With a level and a condition asked for, the lines below are kept or dropped alike without a cost.
		if loc != nil {
			cond, err := filter.Compile("status>=200", loc)
			if err != nil {
				t.Fatal(err)
			}
			p.filter.MinLevel, p.filter.Where = entry.LevelInfo, []filter.Cond{cond}
			p.renderer.NoTime, p.renderer.Fields = true, render.Select([]string{"msg", "status", "nope"})
		}

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
