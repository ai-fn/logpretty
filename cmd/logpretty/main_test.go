package main

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"strings"
	"testing"
	"testing/iotest"
	"time"
)

type failWriter struct{}

func (failWriter) Write([]byte) (int, error) { return 0, errors.New("disk full") }

// signalWriter reports the first write on a channel.
type signalWriter struct {
	buf   bytes.Buffer
	wrote chan struct{}
}

func (w *signalWriter) Write(p []byte) (int, error) {
	n, err := w.buf.Write(p)
	select {
	case w.wrote <- struct{}{}:
	default:
	}
	return n, err
}

func row(kind string, n int) string { return fmt.Sprintf("%-30s len=%d\n", kind, n) }

func TestRun(t *testing.T) {
	long := strings.Repeat("x", maxLine+10)

	tests := []struct {
		name       string
		args       []string
		in         string
		wantOut    string
		wantCode   int
		wantStderr string
	}{
		{name: "empty input", wantCode: 0},
		{name: "json", in: `{"id":1}` + "\n", wantOut: row(`json id="1"`, 8)},
		{name: "json without trailing newline", in: `{"id":2}`, wantOut: row(`json id="2"`, 8)},
		{name: "json with leading space", in: ` {"id":1}` + "\n", wantOut: row(`json id="1"`, 9)},
		{name: "json string id", in: `{"id":"x"}` + "\n", wantOut: row(`json id="x"`, 10)},
		{name: "json without id", in: `{"a":1}` + "\n", wantOut: row(`json id="<nil>"`, 7)},
		{name: "json escapes control chars in id", in: `{"id":"a\u001b[31m"}` + "\n", wantOut: row(`json id="a\x1b[31m"`, 20)},
		{name: "broken json is plain", in: "{bad\n", wantOut: row("text", 4)},
		{name: "logfmt", in: "level=info msg=started\n", wantOut: row("logfmt", 22)},
		{name: "plain", in: "hello\n", wantOut: row("text", 5)},
		{name: "one pair is plain", in: "port=8080\n", wantOut: row("text", 9)},
		{name: "crlf", in: "hello\r\n", wantOut: row("text", 5)},
		{name: "blank line", in: "\n", wantOut: row("text", 0)},
		{
			name:    "mixed formats",
			in:      "{\"id\":7}\na=1 b=2\nboom\n",
			wantOut: row(`json id="7"`, 8) + row("logfmt", 7) + row("text", 4),
		},
		{
			name:    "long line is truncated and stream continues",
			in:      long + "\n" + `{"id":3}` + "\n",
			wantOut: row("text [truncated]", maxLine) + row(`json id="3"`, 8),
		},
		{name: "unknown flag", args: []string{"-nope"}, wantCode: 2, wantStderr: "flag provided but not defined"},
		{name: "positional argument", args: []string{"file.log"}, wantCode: 2, wantStderr: "unexpected argument"},
		{name: "help", args: []string{"-h"}, wantCode: 0, wantStderr: "Usage of logpretty"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			code := run(tt.args, strings.NewReader(tt.in), &stdout, &stderr)

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
			wantOut:    row("text", 5),
			wantStderr: "read:",
		},
		{name: "write error", in: strings.NewReader("hello\n"), out: failWriter{}, wantStderr: "write:"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			out := tt.out
			if out == nil {
				out = &stdout
			}
			code := run(nil, tt.in, out, &stderr)

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

// A complete line followed by a partial one must be flushed before the next read blocks.
func TestRunFlushesBeforeBlockingRead(t *testing.T) {
	pr, pw := io.Pipe()
	out := &signalWriter{wrote: make(chan struct{}, 1)}
	done := make(chan int, 1)
	go func() { done <- run(nil, pr, out, io.Discard) }()

	go func() { _, _ = pw.Write([]byte("a\nb")) }()

	select {
	case <-out.wrote:
	case <-time.After(5 * time.Second):
		t.Fatal("output was not flushed while the reader was blocked on a partial line")
	}

	if err := pw.Close(); err != nil {
		t.Fatal(err)
	}
	if code := <-done; code != 0 {
		t.Fatalf("exit code = %d, want 0", code)
	}
	if want := row("text", 1) + row("text", 1); out.buf.String() != want {
		t.Errorf("stdout = %q, want %q", out.buf.String(), want)
	}
}
