package cli

import (
	"bytes"
	"errors"
	"flag"
	"math"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/ai-fn/logpretty/internal/entry"
	"github.com/ai-fn/logpretty/internal/golden"
	"github.com/ai-fn/logpretty/internal/linereader"
	"github.com/ai-fn/logpretty/internal/term"
)

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
				if got != tt.want {
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
			if got != (Config{}) {
				t.Errorf("Parse(%q) = %+v with an error, want the zero Config", tt.args, got)
			}
		})
	}
}

func TestParseHelp(t *testing.T) {
	for _, args := range [][]string{{"-h"}, {"--help"}, {"-help"}, {"--h"}, {"--utc", "--help"}, {"--help", "--nope"}, {"-h", "file.log"}} {
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
	tests := []struct {
		name string
		err  string
		want string
	}{
		{name: "plain text", err: `unexpected argument "file.log"`, want: `logpretty: unexpected argument "file.log"`},
		{
			name: "control characters of an argument are written out",
			err:  "flag provided but not defined: -\x1b]0;x\x07\t\u009b\n",
			want: `logpretty: flag provided but not defined: -\x1b]0;x\x07\t\x9b\n`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var out bytes.Buffer

			WriteError(&out, errors.New(tt.err))

			if want := tt.want + "\nRun 'logpretty --help' for usage.\n"; out.String() != want {
				t.Errorf("WriteError wrote %q, want %q", out.String(), want)
			}
		})
	}
}
