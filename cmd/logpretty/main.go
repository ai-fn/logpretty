package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"regexp"
)

const maxLine = 1 << 20 // longer lines are truncated, but the stream keeps flowing

func main() {
	os.Exit(run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr))
}

// run returns the process exit code: 0 ok, 1 read/write error, 2 bad arguments.
func run(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("logpretty", flag.ContinueOnError)
	fs.SetOutput(stderr)
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

	if err := process(stdin, stdout); err != nil {
		_, _ = fmt.Fprintln(stderr, "logpretty:", err)
		return 1
	}
	return 0
}

func process(in io.Reader, out io.Writer) error {
	r := bufio.NewReaderSize(in, 64<<10)
	w := bufio.NewWriterSize(out, 64<<10)
	var buf []byte
	for {
		line, truncated, err := readLine(r, buf)
		buf = line // keep our own buffer; the reader's slice never escapes
		if len(line) > 0 || err == nil {
			kind := "text"
			if m, ok := parseJSON(line); ok {
				kind = fmt.Sprintf("json id=%q", fmt.Sprint(m["id"])) // %q: log content must not reach the terminal raw
			} else if looksLogfmt(line) {
				kind = "logfmt"
			}
			if truncated {
				kind += " [truncated]"
			}
			if _, werr := fmt.Fprintf(w, "%-30s len=%d\n", kind, len(line)); werr != nil {
				return fmt.Errorf("write: %w", werr)
			}
		}
		if err != nil || nextReadMayBlock(r) {
			if ferr := w.Flush(); ferr != nil {
				return fmt.Errorf("write: %w", ferr)
			}
		}
		if err != nil {
			if errors.Is(err, io.EOF) {
				return nil
			}
			return fmt.Errorf("read: %w", err)
		}
	}
}

// nextReadMayBlock reports whether the buffer holds no complete line, so output must be flushed first.
// A partial line counts too: Buffered() == 0 alone would hide output behind it.
func nextReadMayBlock(r *bufio.Reader) bool {
	p, _ := r.Peek(r.Buffered()) // peeking buffered bytes never blocks
	return !bytes.Contains(p, []byte{'\n'})
}

// readLine reads a line of any length via ReadSlice. It returns at most maxLine
// bytes, reports truncation, and discards the tail of an oversized line.
func readLine(r *bufio.Reader, dst []byte) (line []byte, truncated bool, err error) {
	dst = dst[:0]
	for {
		chunk, e := r.ReadSlice('\n')
		if room := maxLine - len(dst); room > 0 {
			if len(chunk) > room {
				dst = append(dst, chunk[:room]...)
				truncated = true
			} else {
				dst = append(dst, chunk...)
			}
		} else if len(chunk) > 0 {
			truncated = true
		}
		if errors.Is(e, bufio.ErrBufferFull) {
			continue // line is longer than the reader's buffer
		}
		return bytes.TrimRight(dst, "\r\n"), truncated, e
	}
}

var logfmtPair = regexp.MustCompile(`(?:^|\s)[A-Za-z_][A-Za-z0-9_.\-]*=(?:"(?:[^"\\]|\\.)*"|\S*)`)

// looksLogfmt: at least 2 key=value pairs covering most of the line.
func looksLogfmt(b []byte) bool {
	m := logfmtPair.FindAllIndex(b, -1)
	if len(m) < 2 {
		return false
	}
	covered := 0
	for _, p := range m {
		covered += p[1] - p[0]
	}
	return covered*10 >= len(bytes.TrimSpace(b))*7
}

func parseJSON(b []byte) (map[string]any, bool) {
	t := bytes.TrimSpace(b)
	if len(t) == 0 || t[0] != '{' {
		return nil, false
	}
	d := json.NewDecoder(bytes.NewReader(t))
	d.UseNumber()
	var m map[string]any
	if d.Decode(&m) != nil {
		return nil, false
	}
	return m, true
}
