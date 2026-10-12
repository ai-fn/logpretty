// Package app wires the log pipeline: read lines from stdin, process them, write to stdout.
package app

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"sync"
	"time"

	"github.com/ai-fn/logpretty/internal/cli"
	"github.com/ai-fn/logpretty/internal/filter"
	"github.com/ai-fn/logpretty/internal/linereader"
	"github.com/ai-fn/logpretty/internal/parse"
	"github.com/ai-fn/logpretty/internal/render"
	"github.com/ai-fn/logpretty/internal/term"
)

// ExitInterrupted is the exit code Run returns when ctx is canceled.
const ExitInterrupted = 130

const writeBufferSize = 64 << 10

// Run executes the CLI and returns the process exit code: 0 success or stdout closed by its reader,
// 1 read or write error, 2 invalid arguments, ExitInterrupted when ctx is canceled before stdin ends.
func Run(ctx context.Context, args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	cfg, err := cli.Parse(args)
	switch {
	case errors.Is(err, cli.ErrHelp):
		return writeInfo(stdout, stderr, cli.Usage)
	case errors.Is(err, cli.ErrVersion):
		return writeInfo(stdout, stderr, cli.Version()+"\n")
	case err != nil:
		cli.WriteError(stderr, err)
		return 2
	}

	p := &pipeline{
		r:      linereader.New(stdin, cfg.MaxLine),
		w:      bufio.NewWriterSize(stdout, writeBufferSize),
		filter: filter.Filter{MinLevel: cfg.Level},
	}
	if cfg.UTC {
		// The same zone for both, so that a time written without one is shown as it was written.
		p.parser.Loc, p.renderer.Loc = time.UTC, time.UTC
	}
	// stdout itself is asked, not the buffer in front of it: only the file knows whether it is a terminal.
	if term.UseColor(cfg.Color, stdout, os.Getenv) {
		p.renderer.Theme = render.DefaultTheme()
	}
	// A read from stdin cannot be interrupted, so the pipeline runs aside and is abandoned on cancel.
	done := make(chan error, 1)
	go func() { done <- p.run() }()

	select {
	case err := <-done:
		// A closed stdout is how `logpretty | head` normally ends, so it is not reported.
		if err != nil && !errors.Is(err, errStdoutClosed) {
			_, _ = fmt.Fprintln(stderr, "logpretty:", err)
			return 1
		}
		return 0
	case <-ctx.Done():
		p.stop()
		return ExitInterrupted
	}
}

// writeInfo prints the help or the version; its reader may leave early too, as in `logpretty --help | head`.
func writeInfo(stdout, stderr io.Writer, text string) int {
	if _, werr := io.WriteString(stdout, text); werr != nil {
		if err := writeError(werr); !errors.Is(err, errStdoutClosed) {
			_, _ = fmt.Fprintln(stderr, "logpretty:", err)
			return 1
		}
	}
	return 0
}

var errStdoutClosed = errors.New("stdout is closed")

// writeError wraps a failed write, or returns errStdoutClosed when the reader of stdout has gone away.
func writeError(err error) error {
	for _, closed := range closedPipeErrs {
		if errors.Is(err, closed) {
			return errStdoutClosed
		}
	}
	return fmt.Errorf("write: %w", err)
}

// pipeline renders the lines of r into w. mu is held while output may be unflushed and released only
// around a read that may block, so whoever takes it finds everything rendered so far flushed.
type pipeline struct {
	r *linereader.Reader
	w *bufio.Writer

	parser   parse.Parser
	filter   filter.Filter
	renderer render.Renderer
	out      []byte

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
		if err := p.write(line, dropped); err != nil {
			return true, writeError(err)
		}
		if p.r.MayBlock() {
			break
		}
		line, dropped, readErr = p.r.Next() // a complete line or an error is pending: does not read
	}

	if err := p.w.Flush(); err != nil {
		return true, writeError(err)
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

// A line that was cut is not parsed: what is left of it says little about its format.
func (p *pipeline) write(line []byte, dropped int) error {
	if dropped > 0 {
		if !p.filter.KeepPlain() {
			return nil
		}
		p.out = p.renderer.AppendTruncated(p.out[:0], line, dropped)
	} else {
		e := p.parser.Parse(line)
		if !p.filter.Keep(e) {
			return nil
		}
		p.out = p.renderer.AppendEntry(p.out[:0], e)
	}
	_, err := p.w.Write(p.out)
	return err
}
