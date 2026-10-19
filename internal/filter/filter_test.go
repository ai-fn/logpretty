package filter

import (
	"fmt"
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
			keep = f.KeepPlain(nil)
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

func TestKeepByTrace(t *testing.T) {
	const (
		aInfo  = `{"level":"info","msg":"start","trace_id":"aaa"}`
		aError = `level=error msg=failed traceID=aaa`
		bInfo  = `{"level":"info","msg":"start","traceId":"bbb","user_id":42}`
		bError = `{"level":"error","msg":"failed","trace":{"id":"bbb"}}`
		cInfo  = `{"level":"info","msg":"start","dd.trace_id":123}`
		field  = `{"level":"error","msg":"no id","trace":"aaa"}`
		upper  = `{"level":"info","msg":"start","trace_id":"AAA"}`
		noID   = `{"level":"error","msg":"no id"}`
		web    = `web-1  | {"trace_id":"aaa","msg":"x"}`
		api    = `api-1  | {"trace_id":"bbb","msg":"y"}`
	)
	stream := []string{
		"banner", aInfo, bInfo, "  at b.go:1", aError, "  at a.go:2", cInfo, bError, cut + "long",
		field, "  at f.go:3", upper, noID, "tail",
	}
	traces := func(ids ...string) [][]byte {
		out := make([][]byte, 0, len(ids))
		for _, id := range ids {
			out = append(out, []byte(id))
		}
		return out
	}
	where := func(expression string) []Cond {
		c, err := Compile(expression, time.UTC)
		if err != nil {
			t.Fatal(err)
		}
		return []Cond{c}
	}

	tests := []struct {
		name  string
		f     Filter
		lines []string // the stream above if nil
		want  []string
	}{
		{
			name: "one trace, whatever its key, with its plain lines",
			f:    Filter{Traces: traces("aaa")},
			want: []string{aInfo, aError, "  at a.go:2", field, "  at f.go:3"},
		},
		{
			name: "two traces: an entry of either",
			f:    Filter{Traces: traces("aaa", "bbb")},
			want: []string{aInfo, bInfo, "  at b.go:1", aError, "  at a.go:2", bError, cut + "long", field, "  at f.go:3"},
		},
		{name: "a number is matched by its text", f: Filter{Traces: traces("123")}, want: []string{cInfo}},
		{name: "not as a number", f: Filter{Traces: traces("123.0")}},
		{name: "not by a prefix", f: Filter{Traces: traces("aa")}},
		{name: "not by a part", f: Filter{Traces: traces("aaaa", "a")}},
		{name: "not in another case", f: Filter{Traces: traces("AAA")}, want: []string{upper}},
		{
			name: "and the level",
			f:    Filter{MinLevel: entry.LevelError, Traces: traces("aaa", "bbb")},
			want: []string{aError, "  at a.go:2", bError, cut + "long", field, "  at f.go:3"},
		},
		{
			name: "and the conditions",
			f:    Filter{Traces: traces("aaa", "bbb"), Where: where("user_id=42")},
			want: []string{bInfo, "  at b.go:1"},
		},
		{
			name: "a trace that nothing has hides the plain lines too",
			f:    Filter{Traces: traces("zzz")},
		},
		{
			name: "plain lines are shown when asked",
			f:    Filter{Traces: traces("bbb"), Plain: PlainShow},
			want: []string{"banner", bInfo, "  at b.go:1", "  at a.go:2", bError, cut + "long", "  at f.go:3", "tail"},
		},
		{
			name:  "a plain line follows the entry of its source",
			f:     Filter{Traces: traces("aaa")},
			lines: []string{web, api, "web-1  | at x", "api-1  | at y"},
			want:  []string{web, "web-1  | at x"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			lines := tt.lines
			if lines == nil {
				lines = stream
			}
			if got := kept(&tt.f, lines...); !slices.Equal(got, tt.want) {
				t.Errorf("kept\n %q\nwant\n %q", got, tt.want)
			}
		})
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
	if !f.KeepPlain(nil) || !f.Keep(&entry.Entry{}) || !f.Keep(&entry.Entry{Format: entry.FormatJSON}) || !f.KeepPlain(nil) {
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
	// The sources are remembered once, before the runs: a new one costs its name.
	sourced := &entry.Entry{Format: entry.FormatJSON, Level: entry.LevelError, Source: []byte("web-1")}
	trace := &entry.Entry{Line: []byte("web-1 | at x"), Source: []byte("web-1")}
	f.Keep(sourced)

	if allocs := testing.AllocsPerRun(100, func() {
		f.Keep(e)
		f.Keep(plain)
		f.KeepPlain(nil)
		f.Keep(sourced)
		f.Keep(trace)
		f.KeepPlain([]byte("web-1"))
		f.KeepPlain([]byte("unknown"))
	}); allocs != 0 {
		t.Errorf("Keep allocates %v times per call, want 0", allocs)
	}

	traced := Filter{Traces: [][]byte{[]byte("zzz"), []byte("4bf92f35")}}
	e = p.Parse([]byte(`level=warn msg=slow trace_id=4bf92f35`))
	if !traced.Keep(e) {
		t.Fatal("the entry of a trace asked for is hidden")
	}
	if allocs := testing.AllocsPerRun(100, func() { traced.Keep(e) }); allocs != 0 {
		t.Errorf("Keep with traces allocates %v times per call, want 0", allocs)
	}
	// Without an id, the field trace is decoded into the scratch space.
	e = p.Parse([]byte(`level=warn msg=slow trace="a \"b\""`))
	traced.Traces = [][]byte{[]byte(`a "b"`)}
	if !traced.Keep(e) {
		t.Fatal("the entry with the field trace asked for is hidden")
	}
	if allocs := testing.AllocsPerRun(100, func() { traced.Keep(e) }); allocs != 0 {
		t.Errorf("Keep with traces and the field trace allocates %v times per call, want 0", allocs)
	}
}

func TestKeepPlainLinesFollowTheEntryOfTheirSource(t *testing.T) {
	const (
		webFail = `web-1  | {"level":"error","msg":"failed"}`
		apiInfo = `api-1  | {"level":"info","msg":"ok"}`
		apiFail = `api-1  | {"level":"error","msg":"down"}`
		info    = `{"level":"info","msg":"started"}`
		fail    = `{"level":"error","msg":"failed"}`
	)

	tests := []struct {
		name  string
		lines []string
		want  []string
	}{
		{
			name:  "a stack trace of one container among the lines of another",
			lines: []string{webFail, "web-1  | Traceback:", apiInfo, `web-1  |   File "x.py"`, "api-1  | note", "web-1  | ValueError"},
			want:  []string{webFail, "web-1  | Traceback:", `web-1  |   File "x.py"`, "web-1  | ValueError"},
		},
		{
			name: "pods with timestamps",
			lines: []string{
				`[pod/web-1/app] 2026-10-11T12:00:00Z {"level":"error","msg":"panic"}`,
				`[pod/api-1/app] 2026-10-11T12:00:00Z {"level":"info","msg":"ok"}`,
				"[pod/web-1/app] 2026-10-11T12:00:00Z \tat main.go:12",
				"[pod/api-1/app] 2026-10-11T12:00:00Z \tat api.go:3",
			},
			want: []string{`[pod/web-1/app] 2026-10-11T12:00:00Z {"level":"error","msg":"panic"}`, "[pod/web-1/app] 2026-10-11T12:00:00Z \tat main.go:12"},
		},
		{
			// INFO and id are not containers: with no entries of their own they change nothing.
			name:  "a source without entries goes the way of the entry before",
			lines: []string{fail, "INFO | boom", "id | name", info, "INFO | boom", "1 | foo"},
			want:  []string{fail, "INFO | boom", "id | name"},
		},
		{
			name:  "a line without a source goes the way of the entry before",
			lines: []string{webFail, apiInfo, "no source", apiFail, "no source either"},
			want:  []string{webFail, apiFail, "no source either"},
		},
		{
			name:  "a cut line has no source",
			lines: []string{webFail, apiInfo, cut + "web-1  | long"},
			want:  []string{webFail},
		},
		{
			name:  "an entry of a source decides for it again",
			lines: []string{webFail, "web-1  | a", `web-1  | {"level":"info","msg":"x"}`, "web-1  | b", webFail, "web-1  | c"},
			want:  []string{webFail, "web-1  | a", webFail, "web-1  | c"},
		},
		{
			name:  "plain lines of a source before its first entry",
			lines: []string{"web-1  | banner", webFail, "web-1  | after"},
			want:  []string{webFail, "web-1  | after"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := Filter{MinLevel: entry.LevelError}
			if got := kept(&f, tt.lines...); !slices.Equal(got, tt.want) {
				t.Errorf("kept\n %q\nwant\n %q", got, tt.want)
			}
		})
	}

	// show and hide do not look at the source.
	lines := []string{webFail, apiInfo, "web-1  | a", "api-1  | b"}
	for mode, want := range map[PlainMode][]string{PlainShow: {webFail, "web-1  | a", "api-1  | b"}, PlainHide: {webFail}} {
		f := Filter{MinLevel: entry.LevelError, Plain: mode}
		if got := kept(&f, lines...); !slices.Equal(got, want) {
			t.Errorf("%v: kept\n %q\nwant\n %q", mode, got, want)
		}
	}
}

// A stream with a new name on every line cannot grow the memory: sources past the limit go the general way.
func TestKeepRemembersALimitedNumberOfSources(t *testing.T) {
	lines := make([]string, 0, maxSources+4)
	for i := range maxSources {
		lines = append(lines, fmt.Sprintf(`s%d | {"level":"error","msg":"x"}`, i))
	}
	over := fmt.Sprintf(`s%d | {"level":"error","msg":"x"}`, maxSources)
	lines = append(lines, over, `s0 | {"level":"info","msg":"x"}`, fmt.Sprintf("s%d | past the limit", maxSources), "s1 | remembered")
	f := Filter{MinLevel: entry.LevelError}
	got := kept(&f, lines...)
	if want := append(slices.Clone(lines[:maxSources+1]), "s1 | remembered"); !slices.Equal(got, want) {
		t.Errorf("kept the last lines %q, want %q", got[max(0, len(got)-3):], want[len(want)-3:])
	}
}
