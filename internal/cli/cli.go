// Package cli reads the command line into a Config.
package cli

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/ai-fn/logpretty/internal/entry"
	"github.com/ai-fn/logpretty/internal/filter"
	"github.com/ai-fn/logpretty/internal/linereader"
	"github.com/ai-fn/logpretty/internal/render"
	"github.com/ai-fn/logpretty/internal/term"
)

// Config is what the command line asks for.
type Config struct {
	MaxLine int // the longest line to keep, in bytes
	UTC     bool
	Color   term.ColorMode
	Level   entry.Level   // the lowest level to show; LevelUnknown shows every entry
	Where   []filter.Cond // conditions an entry must meet to be shown, all of them
}

// ErrHelp is what Parse returns when the arguments ask for the help; it is a request, not a failure.
var ErrHelp = errors.New("help requested")

// Usage is the text of the help. It is written by hand: the flag package prints one dash and no examples.
const Usage = `Usage: logpretty [flags] < input

Pretty-print JSON, logfmt and plain-text logs read from stdin. The format of
every line is detected on its own, so mixed streams need no flags.

Flags:
      --level LEVEL     show the entries of this level or above: trace, debug,
                        info, warn, error or fatal. An entry without a level
                        is hidden, and so are the plain lines after a hidden
                        entry, such as its stack trace
      --where EXPR      show the entries that meet the expression, see below.
                        Given twice, it asks for both; the plain lines after
                        a hidden entry are hidden as well
      --color WHEN      color the output: auto (only on a terminal, the
                        default), always or never
      --utc             show times in UTC instead of the local time zone
      --max-line SIZE   longest line to keep, in bytes or with K, M or G
                        (default 1M); the rest of a longer line is dropped
      --version         print the version
  -h, --help            print this help

Expressions:
  KEY                   the entry has the field
  KEY = VALUE           equal. Two numbers are compared as numbers, so 42
                        finds 42, "42" and 42.0; anything else as text
  KEY != VALUE          not equal, or the entry has no such field
  KEY > VALUE           above, as numbers; also >=, < and <=. The level and
                        the time have an order of their own, see below
  KEY ~ REGEXP          the regular expression (RE2) matches a part of the
                        value
  KEY !~ REGEXP         it does not, or the entry has no such field

  The spaces around an operator may be left out. A value with a space goes in
  double quotes, inside the shell's single quotes:
    --where 'msg="connection refused"'
  A key with dots looks into nested objects: http.status. The keys msg, level
  and time stand for the message, the level and the time of an entry whatever
  its own keys are. A level is compared by its severity and a time as a
  moment: level>=warn, time>=2026-10-11T12:00:00Z. A time without a zone is
  local, or UTC with --utc.

Examples:
  kubectl logs -f pod | logpretty
  kubectl logs -f pod | logpretty --level warn
  kubectl logs -f pod | logpretty --where 'status>=500' --where 'path~^/api/'
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
	// Only now is the zone known: --utc may stand after the expression it applies to.
	var loc *time.Location
	if f.UTC {
		loc = time.UTC
	}
	for _, expr := range f.exprs {
		cond, err := filter.Compile(expr, loc)
		if err != nil {
			return Config{}, &whereError{expr: expr, err: err}
		}
		f.Where = append(f.Where, cond)
	}
	if f.version {
		return Config{}, ErrVersion
	}
	if fs.NArg() > 0 {
		return Config{}, fmt.Errorf("unexpected argument %q", fs.Arg(0))
	}
	return f.Config, nil
}

// flags is what the flag set writes into: the Config and what Parse has yet to turn into a part of it.
type flags struct {
	Config
	version bool
	exprs   []string // the expressions of --where as they were typed
}

// whereError is an expression of --where that cannot be read. WriteError shows where its trouble is.
type whereError struct {
	expr string
	err  error
}

func (e *whereError) Error() string {
	return fmt.Sprintf("invalid value %q for flag -where: %v", e.expr, e.err)
}

func (e *whereError) Unwrap() error { return e.err }

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
	fs.Func("level", "", func(s string) (err error) {
		f.Level, err = filter.ParseLevel(s)
		return err
	})
	fs.Func("where", "", func(s string) error {
		f.exprs = append(f.exprs, s)
		return nil
	})
	fs.BoolVar(&f.version, "version", false, "")
	return fs
}

// WriteError reports arguments that Parse turned down. The flag package echoes an argument as it was typed,
// so the text is escaped like a log line. An expression gets a second line, with a caret under its trouble.
func WriteError(w io.Writer, err error) {
	msg := []byte("logpretty: ")
	where, isWhere := errors.AsType[*whereError](err)
	syntax, isSyntax := errors.AsType[*filter.SyntaxError](err)
	if isWhere && isSyntax {
		msg = append(msg, "--where '"...)
		// Counted in the text as it is printed: there a control character takes several columns, not one.
		before := where.expr[:min(max(syntax.Pos, 0), len(where.expr))]
		column := len(msg) + utf8.RuneCount(render.AppendInline(nil, []byte(before)))
		msg = append(render.AppendInline(msg, []byte(where.expr)), "'\n"...)
		msg = append(msg, strings.Repeat(" ", column)...)
		msg = render.AppendInline(append(msg, "^ "...), []byte(syntax.Msg))
	} else {
		msg = render.AppendInline(msg, []byte(err.Error()))
	}
	_, _ = w.Write(append(msg, "\nRun 'logpretty --help' for usage.\n"...))
}
