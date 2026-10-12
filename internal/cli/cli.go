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
      --color WHEN   color the output: auto (only on a terminal, the default),
                     always or never
      --utc          show times in UTC instead of the local time zone
      --max-line N   longest line to keep, in bytes (default 1048576); the rest
                     of a longer line is dropped
  -h, --help         print this help

Examples:
  kubectl logs -f pod | logpretty
  docker compose logs -f | logpretty --utc
  logpretty --color=always < app.log | less -R
`

// Parse reads the arguments that follow the name of the program. The Config is zero unless the error is nil.
func Parse(args []string) (Config, error) {
	cfg := Config{MaxLine: linereader.DefaultMaxLine}
	fs := newFlagSet(&cfg)
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return Config{}, ErrHelp
		}
		return Config{}, err
	}
	if fs.NArg() > 0 {
		return Config{}, fmt.Errorf("unexpected argument %q", fs.Arg(0))
	}
	if cfg.MaxLine <= 0 {
		return Config{}, errors.New("--max-line must be positive")
	}
	return cfg, nil
}

// newFlagSet defines the flags, which write into cfg. The set prints nothing: Parse returns what went wrong.
func newFlagSet(cfg *Config) *flag.FlagSet {
	fs := flag.NewFlagSet("logpretty", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.IntVar(&cfg.MaxLine, "max-line", cfg.MaxLine, "")
	fs.BoolVar(&cfg.UTC, "utc", cfg.UTC, "")
	fs.Func("color", "", func(s string) (err error) {
		cfg.Color, err = term.ParseColorMode(s)
		return err
	})
	return fs
}

// WriteError reports arguments that Parse turned down. The flag package echoes an argument as it was typed,
// so the text is escaped like a log line.
func WriteError(w io.Writer, err error) {
	msg := render.AppendInline([]byte("logpretty: "), []byte(err.Error()))
	_, _ = w.Write(append(msg, "\nRun 'logpretty --help' for usage.\n"...))
}
