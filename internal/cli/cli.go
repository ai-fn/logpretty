// Package cli reads the command line into a Config.
package cli

import (
	"errors"
	"flag"
	"fmt"
	"io"

	"github.com/ai-fn/logpretty/internal/linereader"
	"github.com/ai-fn/logpretty/internal/render"
	"github.com/ai-fn/logpretty/internal/term"
)

// Config is what the command line asks for.
type Config struct {
	MaxLine int // the longest line to keep, in bytes
	UTC     bool
	Color   term.ColorMode
}

// ErrHelp is what Parse returns when the arguments ask for the help; it is a request, not a failure.
var ErrHelp = errors.New("help requested")

// Usage is the text of the help. It is written by hand: the flag package prints one dash and no examples.
const Usage = `Usage: logpretty [flags] < input

Pretty-print JSON, logfmt and plain-text logs read from stdin. The format of
every line is detected on its own, so mixed streams need no flags.

Flags:
      --color WHEN      color the output: auto (only on a terminal, the
                        default), always or never
      --utc             show times in UTC instead of the local time zone
      --max-line SIZE   longest line to keep, in bytes or with K, M or G
                        (default 1M); the rest of a longer line is dropped
      --version         print the version
  -h, --help            print this help

Examples:
  kubectl logs -f pod | logpretty
  docker compose logs -f | logpretty --utc
  logpretty --color=always < app.log | less -R
`

// Parse reads the arguments that follow the name of the program. The Config is zero unless the error is nil.
func Parse(args []string) (Config, error) {
	f := flags{Config: Config{MaxLine: linereader.DefaultMaxLine}}
	fs := newFlagSet(&f)
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return Config{}, ErrHelp
		}
		return Config{}, err
	}
	if f.version {
		return Config{}, ErrVersion
	}
	if fs.NArg() > 0 {
		return Config{}, fmt.Errorf("unexpected argument %q", fs.Arg(0))
	}
	return f.Config, nil
}

// flags is what the flag set writes into: the Config and the requests that are not a part of it.
type flags struct {
	Config
	version bool
}

// newFlagSet defines the flags, which write into f. The set prints nothing: Parse returns what went wrong.
func newFlagSet(f *flags) *flag.FlagSet {
	fs := flag.NewFlagSet("logpretty", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.Func("max-line", "", func(s string) (err error) {
		f.MaxLine, err = parseSize(s)
		return err
	})
	fs.BoolVar(&f.UTC, "utc", f.UTC, "")
	fs.Func("color", "", func(s string) (err error) {
		f.Color, err = term.ParseColorMode(s)
		return err
	})
	fs.BoolVar(&f.version, "version", false, "")
	return fs
}

// WriteError reports arguments that Parse turned down. The flag package echoes an argument as it was typed,
// so the text is escaped like a log line.
func WriteError(w io.Writer, err error) {
	msg := render.AppendInline([]byte("logpretty: "), []byte(err.Error()))
	_, _ = w.Write(append(msg, "\nRun 'logpretty --help' for usage.\n"...))
}
