// Package app wires the log pipeline: read lines from stdin, process them, write to stdout.
package app

import (
	"bufio"
	"errors"
	"flag"
	"fmt"
	"io"
	"strconv"

	"github.com/ai-fn/logpretty/internal/linereader"
)

const writeBufferSize = 64 << 10

// Run executes the CLI and returns the process exit code: 0 success,
// 1 read or write error, 2 invalid arguments.
func Run(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
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

	if err := process(stdin, stdout, *maxLine); err != nil {
		_, _ = fmt.Fprintln(stderr, "logpretty:", err)
		return 1
	}
	return 0
}

func process(in io.Reader, out io.Writer, maxLine int) error {
	r := linereader.New(in, maxLine)
	w := bufio.NewWriterSize(out, writeBufferSize)
	for {
		line, dropped, err := r.Next()
		if err != nil {
			if ferr := w.Flush(); ferr != nil {
				return fmt.Errorf("write: %w", ferr)
			}
			if errors.Is(err, io.EOF) {
				return nil
			}
			return fmt.Errorf("read: %w", err)
		}

		if err := render(w, line, dropped); err != nil {
			return fmt.Errorf("write: %w", err)
		}
		if r.MayBlock() {
			if err := w.Flush(); err != nil {
				return fmt.Errorf("write: %w", err)
			}
		}
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
