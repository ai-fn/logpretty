package cli

import (
	"bytes"
	"errors"
	"flag"
	"math"
	"path/filepath"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/ai-fn/logpretty/internal/entry"
	"github.com/ai-fn/logpretty/internal/filter"
	"github.com/ai-fn/logpretty/internal/golden"
	"github.com/ai-fn/logpretty/internal/linereader"
	"github.com/ai-fn/logpretty/internal/parse"
	"github.com/ai-fn/logpretty/internal/term"
)

// A fixed zone, because on a UTC machine a mixed-up zone would go unnoticed.
// Not for parallel tests: it swaps time.Local.
func setLocalZone(t *testing.T) {
	t.Helper()
	old := time.Local
	time.Local = time.FixedZone("test", 3*60*60)
	t.Cleanup(func() { time.Local = old })
}

func TestParse(t *testing.T) {
	defaults := Config{MaxLine: linereader.DefaultMaxLine}

	tests := []struct {
		name    string
		args    []string
		want    Config
		wantErr string
	}{
		{name: "no arguments", want: defaults},
		{name: "empty arguments", args: []string{}, want: defaults},
		{
			name: "every flag with two dashes",
			args: []string{"--max-line", "8", "--utc", "--color", "always"},
			want: Config{MaxLine: 8, UTC: true, Color: term.ColorAlways},
		},
		{
			name: "every flag with one dash",
			args: []string{"-max-line", "8", "-utc", "-color", "never"},
			want: Config{MaxLine: 8, UTC: true, Color: term.ColorNever},
		},
		{
			name: "values after an equals sign",
			args: []string{"--max-line=3", "-color=always", "--utc=true"},
			want: Config{MaxLine: 3, UTC: true, Color: term.ColorAlways},
		},
		{name: "utc turned off", args: []string{"--utc=false"}, want: defaults},
		{name: "color auto", args: []string{"--color=auto"}, want: defaults},
		{
			name: "the last of a repeated flag wins",
			args: []string{"--color=always", "--color=never", "--max-line=1", "--max-line=2"},
			want: Config{MaxLine: 2, Color: term.ColorNever},
		},
		{name: "level", args: []string{"--level", "warn"}, want: Config{MaxLine: linereader.DefaultMaxLine, Level: entry.LevelWarn}},
		{name: "every level", args: []string{"--level=trace", "--level=debug", "--level=info", "--level=error", "--level=fatal"}, want: Config{MaxLine: linereader.DefaultMaxLine, Level: entry.LevelFatal}},
		{name: "level as a log spells it", args: []string{"-level=WARNING"}, want: Config{MaxLine: linereader.DefaultMaxLine, Level: entry.LevelWarn}},
		{name: "level in its short form", args: []string{"--level", "Err"}, want: Config{MaxLine: linereader.DefaultMaxLine, Level: entry.LevelError}},
		{name: "double dash ends the flags", args: []string{"--"}, want: defaults},
		{name: "max-line with a unit", args: []string{"--max-line", "64K"}, want: Config{MaxLine: 64 << 10}},
		{name: "max-line with a unit spelled out", args: []string{"--max-line=2MiB"}, want: Config{MaxLine: 2 << 20}},
		{name: "the largest max-line", args: []string{"--max-line=" + strconv.Itoa(math.MaxInt)}, want: Config{MaxLine: math.MaxInt}},

		{name: "positional argument", args: []string{"file.log"}, wantErr: `unexpected argument "file.log"`},
		{name: "positional argument after double dash", args: []string{"--", "--utc"}, wantErr: `unexpected argument "--utc"`},
		{name: "positional argument after a flag", args: []string{"--utc", "yes"}, wantErr: `unexpected argument "yes"`},
		{name: "unknown flag", args: []string{"--nope"}, wantErr: "flag provided but not defined: -nope"},
		{name: "flag without its value", args: []string{"--color"}, wantErr: "flag needs an argument: -color"},
		{name: "max-line zero", args: []string{"--max-line", "0"}, wantErr: `invalid value "0" for flag -max-line: want a positive size in bytes, such as 4096, 64K or 1M`},
		{name: "max-line negative", args: []string{"--max-line", "-5"}, wantErr: `invalid value "-5" for flag -max-line: want a positive size`},
		{name: "max-line not a number", args: []string{"--max-line", "abc"}, wantErr: `invalid value "abc" for flag -max-line`},
		{name: "max-line too large", args: []string{"--max-line=9223372036854775808"}, wantErr: `invalid value "9223372036854775808" for flag -max-line: the size is too large`},
		{name: "max-line with a decimal unit", args: []string{"--max-line=1KB"}, wantErr: `invalid value "1KB" for flag -max-line: want a positive size`},
		{name: "color of an unknown kind", args: []string{"--color=sometimes"}, wantErr: `invalid color mode "sometimes": want auto, always or never`},
		{name: "color in another case", args: []string{"--color=Always"}, wantErr: `invalid color mode "Always"`},
		{name: "color with an empty value", args: []string{"--color="}, wantErr: `invalid color mode ""`},
		{name: "level of an unknown name", args: []string{"--level", "loud"}, wantErr: `invalid value "loud" for flag -level: invalid level "loud": want trace, debug, info, warn, error or fatal`},
		{name: "level as a number", args: []string{"--level=40"}, wantErr: `invalid level "40"`},
		{name: "level with an empty value", args: []string{"--level="}, wantErr: `invalid level ""`},
		{name: "level spelled unknown", args: []string{"--level=unknown"}, wantErr: `invalid level "unknown"`},
		{name: "level with a space", args: []string{"--level", " warn"}, wantErr: `invalid level " warn"`},
		{name: "level with a newline", args: []string{"--level", "warn\n"}, wantErr: `invalid level "warn\n"`},
		{name: "level with an offset above", args: []string{"--level", "info+2"}, wantErr: `invalid level "info+2"`},
		{name: "level with an offset below", args: []string{"--level", "debug-4"}, wantErr: `invalid level "debug-4"`},
		{name: "level without its value", args: []string{"--level"}, wantErr: "flag needs an argument: -level"},
		{name: "where without its value", args: []string{"--where"}, wantErr: "flag needs an argument: -where"},
		{name: "an unknown flag wins over a where before it", args: []string{"--where", "a=>1", "--nope"}, wantErr: "flag provided but not defined: -nope"},
		{name: "where wins over a positional argument", args: []string{"--where", "a=>1", "file.log"}, wantErr: `invalid value "a=>1" for flag -where: unexpected '>'`},
		{name: "where without quotes around its spaces", args: []string{"--where", "a", "=", "1"}, wantErr: `unexpected argument "="`},
		{name: "utc with a value that is not a boolean", args: []string{"--utc=maybe"}, wantErr: `invalid boolean value "maybe" for -utc`},
		{name: "a lone dash is an argument", args: []string{"-"}, wantErr: `unexpected argument "-"`},
		{name: "an empty argument", args: []string{""}, wantErr: `unexpected argument ""`},
		{name: "a positional argument wins over a later help", args: []string{"file.log", "--help"}, wantErr: `unexpected argument "file.log"`},
		{name: "utc with an empty value", args: []string{"--utc="}, wantErr: `invalid boolean value "" for -utc`},
		{name: "max-line with an empty value", args: []string{"--max-line="}, wantErr: `invalid value "" for flag -max-line`},
		{name: "three dashes", args: []string{"---x"}, wantErr: "bad flag syntax: ---x"},
		{name: "a flag without a name", args: []string{"--=x"}, wantErr: "bad flag syntax: --=x"},
		{name: "an error wins over a later help", args: []string{"--nope", "--help"}, wantErr: "flag provided but not defined: -nope"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Parse(tt.args)

			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("Parse(%q) fails: %v", tt.args, err)
				}
				if !reflect.DeepEqual(got, tt.want) {
					t.Errorf("Parse(%q) = %+v, want %+v", tt.args, got, tt.want)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("Parse(%q) error = %v, want it to contain %q", tt.args, err, tt.wantErr)
			}
			if errors.Is(err, ErrHelp) {
				t.Errorf("Parse(%q) error is ErrHelp, want a failure", tt.args)
			}
			if !reflect.DeepEqual(got, Config{}) {
				t.Errorf("Parse(%q) = %+v with an error, want the zero Config", tt.args, got)
			}
		})
	}
}

func TestParseWhere(t *testing.T) {
	setLocalZone(t)
	const line = `{"time":"2026-10-11T10:00:00Z","level":"warn","msg":"connection refused","user_id":42,"http":{"status":502}}`

	tests := []struct {
		name  string
		args  []string
		conds int
		want  bool // whether the line passes
	}{
		{name: "a condition that holds", args: []string{"--where", "user_id=42"}, conds: 1, want: true},
		{name: "a condition that does not", args: []string{"--where", "user_id=7"}, conds: 1},
		{name: "one dash and an equals sign", args: []string{"-where=user_id=42"}, conds: 1, want: true},
		{name: "a value in quotes", args: []string{"--where", `msg="connection refused"`}, conds: 1, want: true},
		{name: "spaces around the operator", args: []string{"--where", "http.status >= 500"}, conds: 1, want: true},
		{name: "a key alone", args: []string{"--where", "http"}, conds: 1, want: true},
		{name: "repeated: both hold", args: []string{"--where", "user_id=42", "--where", "level>=warn"}, conds: 2, want: true},
		{name: "repeated: the first does not hold", args: []string{"--where", "user_id=7", "--where", "level>=warn"}, conds: 2},
		{name: "repeated: the last does not hold", args: []string{"--where", "user_id=42", "--where", "level>warn"}, conds: 2},
		{name: "repeated around another flag", args: []string{"--where", "user_id=42", "--level", "info", "--where", "msg~refused$"}, conds: 2, want: true},

		// 12:00 in the local zone of the test is 09:00 UTC, before the entry; 12:00 UTC is after it.
		{name: "a time without a zone is local", args: []string{"--where", "time>=2026-10-11T12:00:00"}, conds: 1, want: true},
		{name: "utc before the time", args: []string{"--utc", "--where", "time>=2026-10-11T12:00:00"}, conds: 1},
		{name: "utc after the time", args: []string{"--where", "time>=2026-10-11T12:00:00", "--utc"}, conds: 1},
		{name: "utc turned off again", args: []string{"--utc", "--where", "time>=2026-10-11T12:00:00", "--utc=false"}, conds: 1, want: true},
		{name: "a time with a zone ignores utc", args: []string{"--where", "time>=2026-10-11T12:00:00+03:00", "--utc"}, conds: 1, want: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg, err := Parse(tt.args)
			if err != nil {
				t.Fatalf("Parse(%q) fails: %v", tt.args, err)
			}
			if len(cfg.Where) != tt.conds {
				t.Fatalf("Parse(%q) has %d conditions, want %d", tt.args, len(cfg.Where), tt.conds)
			}

			var p parse.Parser
			f := filter.Filter{Where: cfg.Where}
			if got := f.Keep(p.Parse([]byte(line))); got != tt.want {
				t.Errorf("Parse(%q): the line passes = %v, want %v", tt.args, got, tt.want)
			}
		})
	}
}

// What the user sees for an expression that cannot be read: the expression, then a caret under the trouble.
func TestParseWhereErrors(t *testing.T) {
	tests := []struct {
		name   string
		expr   string
		shown  string // the expression as it is printed
		column int    // of the caret, in characters of the expression as it is printed
		msg    string
	}{
		{name: "ascii", expr: "user_id=>42", shown: "user_id=>42", column: 8, msg: "unexpected '>'"},
		{name: "cyrillic before the trouble is counted in letters", expr: "имя=>5", shown: "имя=>5", column: 4, msg: "unexpected '>'"},
		{name: "cyrillic in a quoted value", expr: `msg="привет" x`, shown: `msg="привет" x`, column: 13, msg: "unexpected 'x'"},
		{name: "the trouble is a letter that is not ASCII", expr: `"a"é`, shown: `"a"é`, column: 3, msg: "unexpected 'é'"},
		{name: "a control character takes the width it is printed with", expr: "a\x1bb=>1", shown: `a\x1bb=>1`, column: 7, msg: "unexpected '>'"},
		{name: "a control character of two bytes", expr: "a\u009b=>1", shown: `a\x9b=>1`, column: 6, msg: "unexpected '>'"},
		{name: "a bidi override", expr: "a\u202e=>1", shown: `a\u202e=>1`, column: 8, msg: "unexpected '>'"},
		{name: "a tab is printed as text", expr: "a\t=>1", shown: `a\t=>1`, column: 4, msg: "unexpected '>'"},
		{name: "a newline does not break the line", expr: "a\n=>1", shown: `a\n=>1`, column: 4, msg: "unexpected '>'"},
		{name: "a byte that is not UTF-8", expr: "\xff=>1", shown: "\ufffd=>1", column: 2, msg: "unexpected '>'"},
		{
			name:   "a control character after the trouble moves nothing",
			expr:   "n>\x1b[31m",
			shown:  `n>\x1b[31m`,
			column: 2,
			msg:    `"\x1b[31m" is not a number, and >, >=, < and <= compare numbers`,
		},
		{name: "trouble at the end", expr: "a=", shown: "a=", column: 2, msg: `missing value; an empty one is written ""`},
		{name: "an open quote", expr: `a="x`, shown: `a="x`, column: 2, msg: "unterminated quote"},
		{name: "an empty expression", expr: "", shown: "", column: 0, msg: "empty expression"},
		{name: "only spaces", expr: "  ", shown: "  ", column: 2, msg: "empty expression"},
		{
			name:   "a value with a space",
			expr:   "a=b c",
			shown:  "a=b c",
			column: 4,
			msg:    "unexpected 'c': an expression is one condition, and a value with spaces needs double quotes",
		},
		{name: "a single quote", expr: "a='x'", shown: "a='x'", column: 2, msg: `unexpected '\'': text with it needs double quotes`},
		{name: "an order of text", expr: "latency>100ms", shown: "latency>100ms", column: 8, msg: `"100ms" is not a number, and >, >=, < and <= compare numbers`},
		{name: "a regular expression", expr: "msg~a(b", shown: "msg~a(b", column: 4, msg: "invalid regular expression: missing closing )"},
		{name: "a level", expr: "level>=loud", shown: "level>=loud", column: 7, msg: `invalid level "loud": want trace, debug, info, warn, error or fatal`},
		{name: "a time", expr: "time > yesterday", shown: "time > yesterday", column: 7, msg: `"yesterday" is not a time: want one such as 2026-10-11T12:00:00Z`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Parse([]string{"--where", tt.expr})

			if err == nil {
				t.Fatalf("Parse(--where %q) = %+v, want an error", tt.expr, got)
			}
			if !reflect.DeepEqual(got, Config{}) {
				t.Errorf("Parse(--where %q) = %+v with an error, want the zero Config", tt.expr, got)
			}
			var out bytes.Buffer
			WriteError(&out, err)
			// The caret is counted from the start of the line, past "logpretty: --where '".
			want := "logpretty: --where '" + tt.shown + "'\n" + strings.Repeat(" ", 20+tt.column) + "^ " + tt.msg +
				"\nRun 'logpretty --help' for usage.\n"
			if out.String() != want {
				t.Errorf("WriteError for --where %q wrote\n%s\nwant\n%s", tt.expr, out.String(), want)
			}
		})
	}
}

func TestParseWhereReportsTheFirstFailure(t *testing.T) {
	args := []string{"--where", "a=1", "--where", "b=>2", "--utc", "--where", "c=>3"}

	_, err := Parse(args)

	if want := `invalid value "b=>2" for flag -where: unexpected '>'`; err == nil || err.Error() != want {
		t.Errorf("Parse(%q) error = %v, want %s", args, err, want)
	}
}

func TestParseHelp(t *testing.T) {
	for _, args := range [][]string{
		{"-h"},
		{"--help"},
		{"-help"},
		{"--h"},
		{"--utc", "--help"},
		{"--help", "--nope"},
		{"-h", "file.log"},
		// The help is acted on as the flags are read, and an expression is checked after them all.
		{"--where", "a=>1", "--help"},
	} {
		if _, err := Parse(args); !errors.Is(err, ErrHelp) {
			t.Errorf("Parse(%q) error = %v, want ErrHelp", args, err)
		}
	}
	// After the double dash it is an argument like any other.
	if _, err := Parse([]string{"--", "--help"}); err == nil || errors.Is(err, ErrHelp) {
		t.Errorf(`Parse("--", "--help") error = %v, want a failure that is not ErrHelp`, err)
	}
}

// TestUsage keeps the whole help under review; rewrite the file with "go test ./internal/cli -run TestUsage -update".
func TestUsage(t *testing.T) {
	golden.Check(t, filepath.Join("testdata", "help.golden"), []byte(Usage))

	if !strings.HasPrefix(Usage, "Usage: logpretty ") || !strings.HasSuffix(Usage, "\n") || strings.HasSuffix(Usage, "\n\n") {
		t.Errorf("the help must start with the usage line and end in one newline:\n%s", Usage)
	}
	// The default is written in the help by hand.
	if m := regexp.MustCompile(`(?s)--max-line SIZE.*?\(default (\w+)\)`).FindStringSubmatch(Usage); m == nil {
		t.Error("the help does not name the default of --max-line")
	} else if n, err := parseSize(m[1]); err != nil || n != linereader.DefaultMaxLine {
		t.Errorf("the help names %s as the default of --max-line, want %d bytes", m[1], linereader.DefaultMaxLine)
	}
	for i, line := range strings.Split(Usage, "\n") {
		if strings.TrimRight(line, " \t") != line || strings.Contains(line, "\t") {
			t.Errorf("line %d of the help has a tab or spaces at its end: %q", i+1, line)
		}
		// A terminal of 80 columns must not wrap the help.
		if len(line) > 80 {
			t.Errorf("line %d of the help is %d bytes long, want at most 80: %q", i+1, len(line), line)
		}
	}
}

// What the help shows as an expression must be one.
func TestUsageExpressionsCompile(t *testing.T) {
	shown := regexp.MustCompile(`--where '([^']*)'`).FindAllStringSubmatch(Usage, -1)
	if len(shown) < 3 {
		t.Fatalf("the help shows %d expressions after --where, want at least 3", len(shown))
	}
	for _, m := range shown {
		if _, err := filter.Compile(m[1], nil); err != nil {
			t.Errorf("the help shows --where '%s', which fails: %v", m[1], err)
		}
	}
	// The ones in the running text have no flag before them to be found by.
	for _, expr := range []string{"http.status", "level>=warn", "time>=2026-10-11T12:00:00Z"} {
		if _, err := filter.Compile(expr, nil); err != nil || !strings.Contains(Usage, expr) {
			t.Errorf("the help must show %s, and it must work: %v", expr, err)
		}
	}
}

func TestUsageNamesEveryFlag(t *testing.T) {
	seen := 0
	newFlagSet(new(flags)).VisitAll(func(f *flag.Flag) {
		seen++
		// The column of the flags, not a mention inside a description.
		if !regexp.MustCompile(`(?m)^      --` + regexp.QuoteMeta(f.Name) + `( |$)`).MatchString(Usage) {
			t.Errorf("the help does not describe --%s", f.Name)
		}
		if f.Usage != "" {
			t.Errorf("--%s has a usage text of its own, which nothing prints: the help is written by hand", f.Name)
		}
	})
	if seen == 0 {
		t.Fatal("the flag set defines no flags")
	}
	if !strings.Contains(Usage, "\n  -h, --help ") {
		t.Error("the help does not describe --help")
	}
}

func TestWriteError(t *testing.T) {
	syntax := func(pos int, msg string) error { return &filter.SyntaxError{Pos: pos, Msg: msg} }

	tests := []struct {
		name string
		err  error
		want string
	}{
		{name: "plain text", err: errors.New(`unexpected argument "file.log"`), want: `logpretty: unexpected argument "file.log"`},
		{
			name: "control characters of an argument are written out",
			err:  errors.New("flag provided but not defined: -\n\x1b]0;x\x07\t\u009b"),
			want: `logpretty: flag provided but not defined: -\n\x1b]0;x\x07\t\x9b`,
		},
		{
			name: "control characters in what is wrong with an expression are written out",
			err:  &whereError{expr: "x", err: syntax(1, "bad \x1b[31m\n")},
			want: "logpretty: --where 'x'\n" + strings.Repeat(" ", 21) + `^ bad \x1b[31m\n`,
		},
		{
			name: "a position past the end points at the end",
			err:  &whereError{expr: "x", err: syntax(99, "bad")},
			want: "logpretty: --where 'x'\n" + strings.Repeat(" ", 21) + "^ bad",
		},
		{
			name: "a position before the start points at the start",
			err:  &whereError{expr: "x", err: syntax(-1, "bad")},
			want: "logpretty: --where 'x'\n" + strings.Repeat(" ", 20) + "^ bad",
		},
		{
			name: "an expression that failed without a position gets one line",
			err:  &whereError{expr: "x\n", err: errors.New("boom")},
			want: `logpretty: invalid value "x\n" for flag -where: boom`,
		},
		{
			name: "a position without an expression gets one line",
			err:  syntax(0, "boom"),
			want: "logpretty: boom",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var out bytes.Buffer

			WriteError(&out, tt.err)

			if want := tt.want + "\nRun 'logpretty --help' for usage.\n"; out.String() != want {
				t.Errorf("WriteError wrote %q, want %q", out.String(), want)
			}
		})
	}
}
