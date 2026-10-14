package filter

import (
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/ai-fn/logpretty/internal/entry"
	"github.com/ai-fn/logpretty/internal/parse"
)

// matches compiles the expression and asks a new Filter about the line.
func matches(t *testing.T, loc *time.Location, expression, line string) bool {
	t.Helper()
	c, err := Compile(expression, loc)
	if err != nil {
		t.Fatalf("Compile(%q) fails: %v", expression, err)
	}
	p := parse.Parser{Loc: loc}
	f := Filter{Where: []Cond{c}}
	// Clipped, so that a read past the end of the line panics instead of finding spare capacity.
	return f.Keep(p.Parse(slices.Clip([]byte(line))))
}

func TestCondOnFields(t *testing.T) {
	const line = `{"user_id":42,"id":"42","code":"042","version":"1.10","ratio":0.5,"big":9007199254740993,"ok":true,"err":null,"empty":"",` +
		`"text":"slow request","note":"say \"hi\"\n","http":{"status":502,"path":"/a b"},"tags":["x","y"]}`

	tests := map[string]bool{
		// A number.
		`user_id=42`: true, `user_id=43`: false, `user_id!=42`: false, `user_id!=43`: true,
		`user_id=42.0`: true, `user_id=042`: true, `user_id=4.2e1`: true, `user_id=forty-two`: false,
		`user_id>41`: true, `user_id>42`: false, `user_id>=42`: true, `user_id>=43`: false,
		`user_id<43`: true, `user_id<42`: false, `user_id<=42`: true, `user_id<=41`: false,
		`user_id~^4`: true, `user_id!~^4`: false, `user_id`: true,
		`ratio<1`: true, `ratio>=0.5`: true, `ratio>0.5`: false, `ratio=0.50`: true, `ratio=.5`: false,
		// An integer that float64 cannot hold.
		`big=9007199254740993`: true, `big=9007199254740992`: false, `big>9007199254740992`: true, `big<9007199254740994`: true,
		// A string that holds a number is compared as one.
		`id=42`: true, `id=42.0`: true, `id!=42`: false, `id>=42`: true, `id<42`: false,
		`code=42`: true, `code=042`: true, `code!=42`: false, `version=1.1`: true, `version=1.10`: true,
		// In quotes a value is text for = and !=: only the very same text is equal to it.
		`user_id="42"`: true, `user_id="42.0"`: false, `user_id="042"`: false, `user_id!="42"`: false, `user_id!="42.0"`: true,
		`id="42"`: true, `id="42.0"`: false, `id!="42.0"`: true,
		`code="042"`: true, `code="42"`: false, `code!="42"`: true, `code!="042"`: false,
		`version="1.10"`: true, `version="1.1"`: false, `version!="1.1"`: true,
		`ratio="0.5"`: true, `ratio="0.50"`: false, `big="9007199254740993"`: true, `big="9007199254740993.0"`: false,
		`http.status="502"`: true, `http.status="502.0"`: false, `"user_id"="42"`: true, `"user_id"="42.0"`: false,
		// An order compares numbers, in quotes or not.
		`user_id>="42.0"`: true, `user_id>"41"`: true, `user_id<"42"`: false, `code<="42"`: true, `code>"42"`: false,
		// A string.
		`text="slow request"`: true, `text=slow`: false, `text!=slow`: true, `text!="slow request"`: false,
		`text~slow`: true, `text~^request`: false, `text!~^request`: true, `text!~slow`: false, `text~"w r"`: true,
		`text~"(?i)SLOW"`: true, `text~SLOW`: false, `text=Slow`: false,
		`text>5`: false, `text<5`: false, `text>=5`: false, `text<=5`: false,
		// Escapes are decoded before the comparison.
		"note=\"say \\\"hi\\\"\n\"": true, `note~"hi\"\n$"`: true, `note~"hi\"$"`: false, `note~"\\\\"`: false, `note~\s$`: true,
		// The rest of the JSON kinds are compared as they are written.
		`ok=true`: true, `ok=false`: false, `ok!=false`: true, `ok`: true, `ok>0`: false,
		`err=null`: true, `err!=null`: false, `err`: true, `err~^null$`: true,
		`empty=""`: true, `empty!=""`: false, `empty`: true, `empty~^$`: true, `empty=x`: false,
		// Nested objects and arrays.
		`http.status=502`: true, `http.status>=500`: true, `http.status<500`: false, `http.path="/a b"`: true,
		`http`: true, `http~status`: true, `http.nope`: false, `http=502`: false,
		`tags~x`: true, `tags`: true, `tags.0=x`: false, `tags.0`: false,
		// A field that the entry lacks: only the negations hold.
		`nope`: false, `nope=1`: false, `nope=""`: false, `nope!=1`: true, `nope!=""`: true,
		`nope>1`: false, `nope>=1`: false, `nope<1`: false, `nope<=1`: false, `nope~x`: false, `nope~""`: false, `nope!~x`: true,
		// A key in quotes is the same key here.
		`"user_id"=42`: true, `"http.status"=502`: false, `"nope"!=1`: true,
	}
	for expression, want := range tests {
		if got := matches(t, time.UTC, expression, line); got != want {
			t.Errorf("%s: matched = %v, want %v", expression, got, want)
		}
	}
}

func TestCondOnLogfmt(t *testing.T) {
	const line = `time=2026-10-11T12:00:00Z level=info msg="request done" status=200 dur=1.5 path=/a http.status=502 word note="a \"b\""`

	tests := map[string]bool{
		`status=200`: true, `status=200.0`: true, `status>=200`: true, `status<200`: false, `status!=404`: true,
		`status="200"`: true, `status="200.0"`: false, `status!="200.0"`: true, `dur="1.5"`: true, `dur="1.50"`: false, `dur=1.50`: true,
		`dur>1`: true, `dur<=1.5`: true, `path=/a`: true, `path~^/`: true, `path>1`: false,
		`http.status=502`: true, `"http.status"=502`: true,
		`note="a \"b\""`: true, `note~"\"b\"$"`: true,
		// A word without a value is a part of the text, not a key.
		`word`: false, `word!=x`: true, `"word"`: false, `"word"!=x`: true,
		`msg="request done"`: true, `level=info`: true, `level>=warn`: false, `time>=2026-10-11T12:00:00Z`: true,
	}
	for expression, want := range tests {
		if got := matches(t, time.UTC, expression, line); got != want {
			t.Errorf("%s: matched = %v, want %v", expression, got, want)
		}
	}
}

func TestCondOnLevel(t *testing.T) {
	tests := []struct {
		name string
		line string
		want map[string]bool
	}{
		{
			name: "level under its usual key",
			line: `{"level":"WARN","msg":"x"}`,
			want: map[string]bool{
				`level=warn`: true, `level=WARNING`: true, `level=error`: false, `level!=warn`: false, `level!=error`: true,
				`level>=warn`: true, `level>warn`: false, `level>info`: true, `level<error`: true, `level<warn`: false,
				`level<=warn`: true, `level<=info`: false,
				// A regular expression sees the name every log is brought to, not what this one wrote.
				`level~^warn$`: true, `level~WARN`: false, `level!~err`: true, `level!~^w`: false, `level`: true,
				`"level"=WARN`: true, `"level"=warn`: false,
				// Quotes do not make a level text.
				`level="warning"`: true, `level>="warn"`: true, `level!="WARN"`: false,
			},
		},
		{
			name: "level under another key",
			line: `{"severity":"ERROR","msg":"x"}`,
			want: map[string]bool{
				`level=error`: true, `level>=warn`: true, `level~^error$`: true, `level`: true,
				`"level"`: false, `"level"=error`: false, `"level"!=error`: true, `severity=ERROR`: true, `severity=error`: false,
			},
		},
		{
			name: "level as a number of pino",
			line: `{"level":40,"msg":"x"}`,
			want: map[string]bool{
				`level=warn`: true, `level>=warn`: true, `level~warn`: true, `"level"=40`: true, `"level">=40`: true,
				`"level"=40.0`: true, `"level"="40"`: true, `"level"="40.0"`: false,
			},
		},
		{
			name: "level implied by the format",
			line: `{"@t":"2026-10-11T12:00:00Z","@mt":"x"}`,
			want: map[string]bool{`level=info`: true, `level<warn`: true, `level~info`: true, `level`: true, `"level"`: false},
		},
		{
			name: "no level: only the negations hold",
			line: `{"msg":"x"}`,
			want: map[string]bool{
				`level=info`: false, `level!=info`: true, `level>=trace`: false, `level>trace`: false, `level<fatal`: false,
				`level<=fatal`: false, `level`: false, `level~.`: false, `level!~.`: true,
			},
		},
		{
			name: "a level that cannot be read is a field like any other",
			line: `{"level":"bogus","msg":"x"}`,
			want: map[string]bool{
				`level`: true, `level~bog`: true, `level!~bog`: false, `level=info`: false, `level!=info`: true,
				`level>=trace`: false, `"level"=bogus`: true,
			},
		},
		{
			// The parser reads a level as it is written, so this one has none; its text still decodes to "warn".
			name: "a level hidden behind an escape is compared as no level and matched as text",
			line: `{"level":"w\u0061rn","msg":"x"}`,
			want: map[string]bool{`level=warn`: false, `level!=warn`: true, `level>=trace`: false, `level~^warn$`: true, `level`: true, `"level"=warn`: true},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for expression, want := range tt.want {
				if got := matches(t, time.UTC, expression, tt.line); got != want {
					t.Errorf("%s: matched = %v, want %v", expression, got, want)
				}
			}
		})
	}
}

func TestCondOnTime(t *testing.T) {
	moscow := time.FixedZone("test", 3*60*60)

	tests := []struct {
		name string
		loc  *time.Location
		line string
		want map[string]bool
	}{
		{
			name: "time with a zone",
			loc:  time.UTC,
			line: `{"time":"2026-10-11T12:00:00Z","msg":"x"}`,
			want: map[string]bool{
				`time=2026-10-11T12:00:00Z`: true, `time=2026-10-11T15:00:00+03:00`: true, `time=2026-10-11T12:00:01Z`: false,
				`time!=2026-10-11T12:00:00Z`: false, `time!=2026-10-11T12:00:01Z`: true,
				`time>=2026-10-11T12:00:00Z`: true, `time>2026-10-11T12:00:00Z`: false, `time>2026-10-11T11:59:59.999Z`: true,
				`time<2026-10-11T12:00:01Z`: true, `time<2026-10-11T12:00:00Z`: false, `time<=2026-10-11T12:00:00Z`: true,
				`time<=2026-10-11T11:59:59Z`: false,
				// A time without a zone is in the zone of the output, here UTC.
				`time="2026-10-11 12:00:00"`: true, `time=2026-10-11T12:00:00`: true,
				`time=1791720000`: true, `time>=1791720000000`: true, `time<1791720000`: false,
				// Quotes do not make a time text.
				`time="2026-10-11T15:00:00+03:00"`: true, `time="1791720000"`: true,
				// A regular expression sees the text of the field.
				`time~T12:`: true, `time~^2026`: true, `time!~T12:`: false, `time~15:00`: false, `time`: true,
				`"time"~T12:`: true, `"time"=2026-10-11T12:00:00Z`: true, `"time"=2026-10-11T15:00:00+03:00`: false,
			},
		},
		{
			name: "time under another key",
			loc:  time.UTC,
			line: `{"ts":1791720000,"msg":"x"}`,
			want: map[string]bool{
				`time=2026-10-11T12:00:00Z`: true, `time~^179`: true, `time`: true, `"time"`: false, `ts=1791720000`: true,
				`ts=1791720000.0`: true, `ts="1791720000"`: true, `ts="1791720000.0"`: false,
			},
		},
		{
			name: "a time without a zone, in the zone of the output",
			loc:  moscow,
			line: `{"time":"2026-10-11 12:00:00","msg":"x"}`,
			want: map[string]bool{
				`time="2026-10-11 12:00:00"`: true, `time=2026-10-11T09:00:00Z`: true, `time=2026-10-11T12:00:00Z`: false,
				`time>="2026-10-11 11:59:59"`: true, `time<"2026-10-11 12:00:00"`: false,
			},
		},
		{
			name: "no time: only the negations hold",
			loc:  time.UTC,
			line: `{"msg":"x"}`,
			want: map[string]bool{
				`time=2026-10-11T12:00:00Z`: false, `time!=2026-10-11T12:00:00Z`: true, `time>=2000-01-01T00:00:00Z`: false,
				`time<2099-01-01T00:00:00Z`: false, `time`: false, `time~.`: false, `time!~.`: true,
			},
		},
		{
			name: "the time a collector put in front is the time of an entry without one",
			loc:  time.UTC,
			line: `[pod/web-1/app] 2026-10-11T15:00:00+03:00 {"msg":"x"}`,
			want: map[string]bool{
				`time`: true, `time=2026-10-11T12:00:00Z`: true, `time>=2026-10-11T12:00:00Z`: true, `time<2026-10-11T12:00:00Z`: false,
				`time!=2026-10-11T12:00:00Z`: false, `time~\+03:00$`: true, `time!~^2026`: false, `"time"`: false,
			},
		},
		{
			name: "the time of the entry wins over the one a collector put in front",
			loc:  time.UTC,
			line: `2026-10-11T12:00:00Z {"time":"2026-10-11T13:00:00Z","msg":"x"}`,
			want: map[string]bool{`time=2026-10-11T13:00:00Z`: true, `time=2026-10-11T12:00:00Z`: false, `time~T13`: true, `time~T12`: false},
		},
		{
			name: "a time a collector put in front without a zone, in the zone of the output",
			loc:  moscow,
			line: `web | 2026-10-11 12:00:00 {"msg":"x"}`,
			want: map[string]bool{`time=2026-10-11T09:00:00Z`: true, `time~"^2026-10-11 12"`: true},
		},
		{
			name: "a time that cannot be read is a field like any other",
			loc:  time.UTC,
			line: `{"time":"yesterday","msg":"x"}`,
			want: map[string]bool{`time`: true, `time~yest`: true, `time>=2000-01-01T00:00:00Z`: false, `time!=2000-01-01T00:00:00Z`: true, `"time"=yesterday`: true},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for expression, want := range tt.want {
				if got := matches(t, tt.loc, expression, tt.line); got != want {
					t.Errorf("%s: matched = %v, want %v", expression, got, want)
				}
			}
		})
	}
}

func TestCondOnMessage(t *testing.T) {
	tests := []struct {
		name string
		line string
		want map[string]bool
	}{
		{
			name: "message under another key",
			line: `{"message":"hello world","n":1}`,
			want: map[string]bool{
				`msg="hello world"`: true, `msg~^hello`: true, `msg!=bye`: true, `msg`: true, `message="hello world"`: true,
				`"msg"`: false, `"msg"="hello world"`: false,
			},
		},
		{
			name: "a msg that is not the message is a field like any other",
			line: `{"msg":42,"n":1}`,
			want: map[string]bool{`msg=42`: true, `msg>41`: true, `msg`: true, `"msg"=42`: true, `msg=42.0`: true, `msg="42"`: true, `msg="42.0"`: false},
		},
		{
			name: "a message that reads as a number",
			line: `{"message":"042","n":1}`,
			want: map[string]bool{`msg=42`: true, `msg="042"`: true, `msg="42"`: false, `msg!="42"`: true, `msg>="42"`: true},
		},
		{
			name: "no message",
			line: `{"n":1}`,
			want: map[string]bool{`msg`: false, `msg=x`: false, `msg!=x`: true, `msg~.`: false, `msg!~.`: true},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for expression, want := range tt.want {
				if got := matches(t, time.UTC, expression, tt.line); got != want {
					t.Errorf("%s: matched = %v, want %v", expression, got, want)
				}
			}
		})
	}
}

func TestCompileErrors(t *testing.T) {
	tests := []struct {
		in      string
		wantPos int
		wantMsg string
	}{
		{in: "a==1", wantPos: 2, wantMsg: `unexpected '='`},
		{in: "", wantPos: 0, wantMsg: "empty expression"},
		{in: "ms>abc", wantPos: 3, wantMsg: `"abc" is not a number`},
		{in: "ms >= 100ms", wantPos: 6, wantMsg: `"100ms" is not a number`},
		{in: `ms<""`, wantPos: 3, wantMsg: `"" is not a number`},
		{in: "ms<=1e", wantPos: 4, wantMsg: `"1e" is not a number`},
		{in: `msg>"a\tb"`, wantPos: 4, wantMsg: `"a\\tb" is not a number`},
		{in: `msg~"("`, wantPos: 4, wantMsg: "invalid regular expression: "},
		{in: "msg!~[", wantPos: 5, wantMsg: "invalid regular expression: "},
		{in: "level=loud", wantPos: 6, wantMsg: `invalid level "loud": want trace, debug, info, warn, error or fatal`},
		{in: "level>40", wantPos: 6, wantMsg: `invalid level "40"`},
		{in: "level<=info+2", wantPos: 7, wantMsg: `invalid level "info+2"`},
		{in: `level!=""`, wantPos: 7, wantMsg: `invalid level ""`},
		{in: "time>yesterday", wantPos: 5, wantMsg: `"yesterday" is not a time`},
		{in: `time=""`, wantPos: 5, wantMsg: `"" is not a time`},
		{in: "time<=12:00", wantPos: 6, wantMsg: `"12:00" is not a time`},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			c, err := Compile(tt.in, time.UTC)

			var syntax *SyntaxError
			if !errors.As(err, &syntax) {
				t.Fatalf("Compile(%q) = %+v, %v; want a *SyntaxError", tt.in, c, err)
			}
			if syntax.Pos != tt.wantPos || !strings.HasPrefix(syntax.Msg, tt.wantMsg) {
				t.Errorf("Compile(%q): position %d, %q; want %d, %q", tt.in, syntax.Pos, syntax.Msg, tt.wantPos, tt.wantMsg)
			}
		})
	}

	// What a typed key refuses, the same key takes in quotes or with a regular expression.
	for _, in := range []string{`level~loud`, `"level"=loud`, `"level">40`, `time~yesterday`, `"time"=yesterday`, `"time">5`, `msg=abc`, `msg>5`, `ms>"100"`, `ms="100ms"`} {
		if _, err := Compile(in, time.UTC); err != nil {
			t.Errorf("Compile(%q) fails: %v", in, err)
		}
	}
}

// The text of a failed regular expression holds the expression itself: it must stay on one line.
func TestCompileErrorOfARegexpHasNoLineBreak(t *testing.T) {
	_, err := Compile("msg~\"(\n\"", time.UTC)
	if err == nil || strings.ContainsAny(err.Error(), "\n\r") {
		t.Errorf("Compile of a broken expression with a line break: error = %q, want one line", err)
	}
}

func TestParseLevel(t *testing.T) {
	for in, want := range map[string]entry.Level{
		"trace": entry.LevelTrace, "debug": entry.LevelDebug, "info": entry.LevelInfo, "warn": entry.LevelWarn,
		"error": entry.LevelError, "fatal": entry.LevelFatal, "WARNING": entry.LevelWarn, "Err": entry.LevelError,
	} {
		if got, err := ParseLevel(in); err != nil || got != want {
			t.Errorf("ParseLevel(%q) = %v, %v; want %v", in, got, err, want)
		}
	}
	for _, in := range []string{"", "loud", "unknown", "40", " warn", "warn\n", "info+2", "debug-4"} {
		if got, err := ParseLevel(in); err == nil || got != entry.LevelUnknown || !strings.Contains(err.Error(), "want trace, debug, info, warn, error or fatal") {
			t.Errorf("ParseLevel(%q) = %v, %v; want an error that lists the levels", in, got, err)
		}
	}
}

func TestKeepWithSeveralConditions(t *testing.T) {
	compile := func(expressions ...string) []Cond {
		conds := make([]Cond, 0, len(expressions))
		for _, expression := range expressions {
			c, err := Compile(expression, time.UTC)
			if err != nil {
				t.Fatalf("Compile(%q) fails: %v", expression, err)
			}
			conds = append(conds, c)
		}
		return conds
	}
	const (
		slow = `{"level":"warn","msg":"slow","user_id":42,"ms":120}`
		fast = `{"level":"warn","msg":"fast","user_id":42,"ms":3}`
		info = `{"level":"info","msg":"slow","user_id":42,"ms":500}`
		them = `{"level":"error","msg":"slow","user_id":7,"ms":900}`
	)

	tests := []struct {
		name  string
		f     Filter
		lines []string
		want  []string
	}{
		{
			name:  "every condition must hold",
			f:     Filter{Where: compile("user_id=42", "ms>100")},
			lines: []string{slow, fast, info, them},
			want:  []string{slow, info},
		},
		{
			name:  "the conditions and the level",
			f:     Filter{MinLevel: entry.LevelWarn, Where: compile("user_id=42", "ms>100")},
			lines: []string{slow, fast, info, them},
			want:  []string{slow},
		},
		{
			name:  "plain lines go the way of the entry before them",
			f:     Filter{Where: compile("ms>100")},
			lines: []string{"banner", slow, "  at a.go:1", fast, "  at b.go:2", cut + "long", them, cut + "kept"},
			want:  []string{slow, "  at a.go:1", them, cut + "kept"},
		},
		{
			name:  "a condition that nothing passes hides the plain lines too",
			f:     Filter{Where: compile("nope")},
			lines: []string{"banner", slow, "tail"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := kept(&tt.f, tt.lines...); !slices.Equal(got, tt.want) {
				t.Errorf("kept\n %q\nwant\n %q", got, tt.want)
			}
		})
	}
}

func TestKeepWithConditionsDoesNotAllocate(t *testing.T) {
	expressions := []string{`user_id=42`, `user_id="42"`, `http.status>=500`, `note="say \"hi\""`, `level>=warn`, `time>=2026-10-11T00:00:00Z`, `nope!=1`}
	if !raceEnabled {
		expressions = append(expressions, `msg~slow`, `level~^warn$`, `time~T12:`)
	}
	p := parse.Parser{Loc: time.UTC}
	e := p.Parse([]byte(`time=2026-10-11T12:00:00Z level=warn msg="slow request" user_id=42 http.status=502 note="say \"hi\""`))

	for _, expression := range expressions {
		c, err := Compile(expression, time.UTC)
		if err != nil {
			t.Fatal(err)
		}
		f := Filter{Where: []Cond{c}}
		if !f.Keep(e) { // grows the scratch space once
			t.Fatalf("%s does not match the entry", expression)
		}
		if allocs := testing.AllocsPerRun(100, func() { f.Keep(e) }); allocs != 0 {
			t.Errorf("%s: Keep allocates %v times per call, want 0", expression, allocs)
		}
	}
}

func TestHolds(t *testing.T) {
	// For each operator: whether it holds when the left side is below, equal to and above the right one.
	tests := map[op][3]bool{
		opEq: {false, true, false}, opNe: {true, false, true},
		opGt: {false, false, true}, opGe: {false, true, true},
		opLt: {true, false, false}, opLe: {true, true, false},
		opExists: {}, opMatch: {}, opNoMatch: {},
	}
	for operator, want := range tests {
		for i, order := range []int{-1, 0, 1} {
			if got := holds(operator, order); got != want[i] {
				t.Errorf("holds(operator %d, %d) = %v, want %v", operator, order, got, want[i])
			}
		}
	}
}
