// Package app wires the log pipeline: read lines from stdin, process them, write to stdout.
package app

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"strconv"
	"sync"

	"github.com/ai-fn/logpretty/internal/linereader"
)

// ExitInterrupted is the exit code Run returns when ctx is canceled.
const ExitInterrupted = 130

const writeBufferSize = 64 << 10

// Run executes the CLI and returns the process exit code: 0 success, 1 read or write error,
// 2 invalid arguments, ExitInterrupted when ctx is canceled before stdin ends.
func Run(ctx context.Context, args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("logpretty", flag.ContinueOnError)
	fs.SetOutput(stderr)
	maxLine := fs.Int("max-line", linereader.DefaultMaxLine, "longest line to keep, in `bytes`; the rest of a longer line is dropped")
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}
	if fs.NArg() > 0 {
		_, _ = fmt.Fprintf(stderr, "logpretty: unexpected argument %q\n", fs.Arg(0))
		return 2
	}
	if *maxLine <= 0 {
		_, _ = fmt.Fprintln(stderr, "logpretty: -max-line must be positive")
		return 2
	}

	p := &pipeline{
		r: linereader.New(stdin, *maxLine),
		w: bufio.NewWriterSize(stdout, writeBufferSize),
	}
	// A read from stdin cannot be interrupted, so the pipeline runs aside and is abandoned on cancel.
	done := make(chan error, 1)
	go func() { done <- p.run() }()

	select {
	case err := <-done:
		if err != nil {
			_, _ = fmt.Fprintln(stderr, "logpretty:", err)
			return 1
		}
		return 0
	case <-ctx.Done():
		p.stop()
		return ExitInterrupted
	}
}

// pipeline copies lines from r to w. mu is held while output may be unflushed and released only
// around a read that may block, so whoever takes it finds everything rendered so far flushed.
type pipeline struct {
	r *linereader.Reader
	w *bufio.Writer

	mu      sync.Mutex
	stopped bool
}

// stop waits for the lines being written and makes the pipeline write nothing more.
func (p *pipeline) stop() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.stopped = true
}

func (p *pipeline) run() error {
	for {
		// Next is the only read that may block, and it runs before burst takes mu.
		if done, err := p.burst(p.r.Next()); done {
			return err
		}
	}
}

// burst handles the result of Next and every complete line already buffered, then flushes.
// It reports done when the stream has ended, failed or been stopped.
func (p *pipeline) burst(line []byte, dropped int, readErr error) (done bool, err error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.stopped {
		return true, nil
	}

	for readErr == nil {
		if err := render(p.w, line, dropped); err != nil {
			return true, fmt.Errorf("write: %w", err)
		}
		if p.r.MayBlock() {
			break
		}
		line, dropped, readErr = p.r.Next() // a complete line or an error is pending: does not read
	}

	if err := p.w.Flush(); err != nil {
		return true, fmt.Errorf("write: %w", err)
	}
	switch {
	case readErr == nil:
		return false, nil
	case errors.Is(readErr, io.EOF):
		return true, nil
	default:
		return true, fmt.Errorf("read: %w", readErr)
	}
}

// render writes the line and, for a truncated one, a marker with the dropped byte count.
// bufio.Writer keeps the first error and returns it from every later call, so only the last result is checked.
func render(w *bufio.Writer, line []byte, dropped int) error {
	_, _ = w.Write(line)
	if dropped > 0 {
		_, _ = w.WriteString("…[+")
		_, _ = w.Write(strconv.AppendInt(w.AvailableBuffer(), int64(dropped), 10))
		_ = w.WriteByte(']')
	}
	return w.WriteByte('\n')
}
