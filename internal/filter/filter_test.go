package filter

import (
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/ai-fn/logpretty/internal/entry"
	"github.com/ai-fn/logpretty/internal/parse"
)

// cut marks a line of a test stream as one that was cut at the length limit: such a line is not parsed.
const cut = "CUT:"

// kept runs the lines through the filter the way the pipeline does and returns the ones it keeps.
func kept(f *Filter, lines ...string) []string {
	p := parse.Parser{Loc: time.UTC}
	var out []string
	for _, line := range lines {
		keep := false
		if strings.HasPrefix(line, cut) {
			keep = f.KeepPlain()
		} else {
			// Clipped, so that a read past the end of the line panics instead of finding spare capacity.
			keep = f.Keep(p.Parse(slices.Clip([]byte(line))))
		}
		if keep {
			out = append(out, line)
		}
	}
	return out
}

func TestKeepByLevel(t *testing.T) {
	lines := map[entry.Level]string{
		entry.LevelUnknown: `{"msg":"x"}`,
		entry.LevelTrace:   `{"level":"trace","msg":"x"}`,
		entry.LevelDebug:   `level=debug msg=x`,
		entry.LevelInfo:    `{"level":"info","msg":"x"}`,
		entry.LevelWarn:    `{"severity":"WARNING","msg":"x"}`,
		entry.LevelError:   `web-1 | {"level":"error","msg":"x"}`,
		entry.LevelFatal:   `level=fatal msg=x`,
	}

	for lowest := entry.LevelUnknown; lowest <= entry.LevelFatal; lowest++ {
		for level, line := range lines {
			// An entry without a level passes only when no level is asked for.
			want := lowest == entry.LevelUnknown || (level != entry.LevelUnknown && level >= lowest)

			f := Filter{MinLevel: lowest}
			if got := len(kept(&f, line)) == 1; got != want {
				t.Errorf("MinLevel %v, entry of level %v: kept = %v, want %v", lowest, level, got, want)
			}
		}
	}
}

func TestKeepReadsTheLevelTheParserFound(t *testing.T) {
	tests := []struct {
		name string
		line string
		min  entry.Level
		want bool
	}{
		{name: "pino number", line: `{"level":40,"msg":"x"}`, min: entry.LevelWarn, want: true},
		{name: "pino number below", line: `{"level":30,"msg":"x"}`, min: entry.LevelWarn, want: false},
		{name: "level implied by Serilog", line: `{"@t":"2026-10-11T12:00:00Z","@mt":"x"}`, min: entry.LevelInfo, want: true},
		{name: "level implied by Serilog, below", line: `{"@t":"2026-10-11T12:00:00Z","@mt":"x"}`, min: entry.LevelWarn, want: false},
		{name: "a level that cannot be read is no level", line: `{"level":"bogus","msg":"x"}`, min: entry.LevelTrace, want: false},
		{name: "an entry without fields has no level", line: `{}`, min: entry.LevelTrace, want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := Filter{MinLevel: tt.min}
			if got := len(kept(&f, tt.line)) == 1; got != tt.want {
				t.Errorf("MinLevel %v: kept = %v, want %v", tt.min, got, tt.want)
			}
		})
	}
}

// A plain line follows the entry before it: a stack trace stays with its error and goes with a hidden one.
func TestKeepPlainLinesFollowTheEntryBefore(t *testing.T) {
	const (
		info  = `{"level":"info","msg":"started"}`
		fail  = `{"level":"error","msg":"failed"}`
		warn  = `level=warn msg=slow`
		empty = `{}`
	)

	tests := []struct {
		name  string
		min   entry.Level
		lines []string
		want  []string
	}{
		{
			name:  "no filter keeps everything",
			lines: []string{"banner", "", info, "  at a.go:1", fail, "  at b.go:2", cut + "long", empty, "tail"},
			want:  []string{"banner", "", info, "  at a.go:1", fail, "  at b.go:2", cut + "long", empty, "tail"},
		},
		{
			name:  "plain lines before the first entry are hidden",
			min:   entry.LevelWarn,
			lines: []string{"banner", "", cut + "long", fail, "  at b.go:2"},
			want:  []string{fail, "  at b.go:2"},
		},
		{
			name:  "a stack trace stays with its error",
			min:   entry.LevelError,
			lines: []string{info, fail, "panic: boom", "", "goroutine 1 [running]:", "\tmain.go:30", info, "after"},
			want:  []string{fail, "panic: boom", "", "goroutine 1 [running]:", "\tmain.go:30"},
		},
		{
			name:  "plain lines after a hidden entry are hidden",
			min:   entry.LevelError,
			lines: []string{fail, info, "note", "", fail},
			want:  []string{fail, fail},
		},
		{
			name:  "a cut line is a plain one",
			min:   entry.LevelWarn,
			lines: []string{warn, cut + "kept", info, cut + "hidden", warn, cut + "kept again"},
			want:  []string{warn, cut + "kept", warn, cut + "kept again"},
		},
		{
			name:  "an entry without fields is an entry: it has no level and hides what follows",
			min:   entry.LevelWarn,
			lines: []string{warn, "kept", empty, "hidden"},
			want:  []string{warn, "kept"},
		},
		{
			name:  "only plain lines",
			min:   entry.LevelTrace,
			lines: []string{"a", "b"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := Filter{MinLevel: tt.min}
			if got := kept(&f, tt.lines...); !slices.Equal(got, tt.want) {
				t.Errorf("kept\n %q\nwant\n %q", got, tt.want)
			}
		})
	}
}

func TestKeepPlainLinesByMode(t *testing.T) {
	const (
		info = `{"level":"info","msg":"started","user_id":7}`
		fail = `{"level":"error","msg":"failed","user_id":42}`
	)
	stream := []string{"banner", "", info, "  after info", cut + "cut after info", fail, "  after fail", cut + "cut after fail", `{}`, "after empty"}
	where, err := Compile("user_id=42", time.UTC)
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name string
		f    Filter
		want []string
	}{
		{name: "follow without a filter shows every line", f: Filter{Plain: PlainFollow}, want: stream},
		{name: "show without a filter shows every line", f: Filter{Plain: PlainShow}, want: stream},
		{
			// An entry without fields is an entry, not a plain line.
			name: "hide without a filter shows the entries alone",
			f:    Filter{Plain: PlainHide},
			want: []string{info, fail, `{}`},
		},
		{
			name: "follow with a level",
			f:    Filter{MinLevel: entry.LevelError},
			want: []string{fail, "  after fail", cut + "cut after fail"},
		},
		{
			name: "show with a level shows the plain lines wherever they are",
			f:    Filter{MinLevel: entry.LevelError, Plain: PlainShow},
			want: []string{"banner", "", "  after info", cut + "cut after info", fail, "  after fail", cut + "cut after fail", "after empty"},
		},
		{name: "hide with a level", f: Filter{MinLevel: entry.LevelError, Plain: PlainHide}, want: []string{fail}},
		{
			name: "follow with a condition",
			f:    Filter{Where: []Cond{where}},
			want: []string{fail, "  after fail", cut + "cut after fail"},
		},
		{
			name: "show with a condition",
			f:    Filter{Where: []Cond{where}, Plain: PlainShow},
			want: []string{"banner", "", "  after info", cut + "cut after info", fail, "  after fail", cut + "cut after fail", "after empty"},
		},
		{name: "hide with a condition", f: Filter{Where: []Cond{where}, Plain: PlainHide}, want: []string{fail}},
		{
			name: "show with a filter that nothing passes",
			f:    Filter{MinLevel: entry.LevelFatal, Plain: PlainShow},
			want: []string{"banner", "", "  after info", cut + "cut after info", "  after fail", cut + "cut after fail", "after empty"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := kept(&tt.f, stream...); !slices.Equal(got, tt.want) {
				t.Errorf("kept\n %q\nwant\n %q", got, tt.want)
			}
		})
	}
}

func TestParsePlainMode(t *testing.T) {
	tests := []struct {
		in      string
		want    PlainMode
		wantErr bool
	}{
		{in: "follow", want: PlainFollow},
		{in: "show", want: PlainShow},
		{in: "hide", want: PlainHide},
		{in: "", wantErr: true},
		{in: "Show", wantErr: true},
		{in: " hide", wantErr: true},
		{in: "all", wantErr: true},
		{in: "0", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			got, err := ParsePlainMode(tt.in)

			if (err != nil) != tt.wantErr {
				t.Fatalf("ParsePlainMode(%q) error = %v, want an error: %v", tt.in, err, tt.wantErr)
			}
			if got != tt.want {
				t.Errorf("ParsePlainMode(%q) = %v, want %v", tt.in, got, tt.want)
			}
			if want := `invalid plain mode "` + tt.in + `": want follow, show or hide`; tt.wantErr && err.Error() != want {
				t.Errorf("ParsePlainMode(%q) error = %q, want %q", tt.in, err, want)
			}
		})
	}
}

func TestPlainModeString(t *testing.T) {
	for mode, want := range map[PlainMode]string{PlainFollow: "follow", PlainShow: "show", PlainHide: "hide", 9: "PlainMode(9)"} {
		if got := mode.String(); got != want {
			t.Errorf("PlainMode(%d).String() = %q, want %q", uint8(mode), got, want)
		}
	}
	for _, mode := range []PlainMode{PlainFollow, PlainShow, PlainHide} {
		if got, err := ParsePlainMode(mode.String()); err != nil || got != mode {
			t.Errorf("ParsePlainMode(%q) = %v, %v; want %v", mode.String(), got, err, mode)
		}
	}
}

func TestZeroFilterKeepsEverything(t *testing.T) {
	var f Filter
	if !f.KeepPlain() || !f.Keep(&entry.Entry{}) || !f.Keep(&entry.Entry{Format: entry.FormatJSON}) || !f.KeepPlain() {
		t.Error("the zero Filter hides a line")
	}
	if f.Plain != PlainFollow {
		t.Errorf("the zero Filter has the plain mode %v, want it to follow", f.Plain)
	}
}

func TestKeepDoesNotAllocate(t *testing.T) {
	p := parse.Parser{Loc: time.UTC}
	f := Filter{MinLevel: entry.LevelWarn}
	e := p.Parse([]byte(`level=warn msg=slow`))
	plain := &entry.Entry{Line: []byte("text")}

	if allocs := testing.AllocsPerRun(100, func() {
		f.Keep(e)
		f.Keep(plain)
		f.KeepPlain()
	}); allocs != 0 {
		t.Errorf("Keep allocates %v times per call, want 0", allocs)
	}
}
