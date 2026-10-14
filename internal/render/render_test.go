package render

import (
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/ai-fn/logpretty/internal/entry"
	"github.com/ai-fn/logpretty/internal/parse"
)

// testLoc stands in for the local zone, so that the tests do not depend on the machine they run on.
var testLoc = time.FixedZone("test", 3*60*60)

// tags is a theme whose styles can be read in the output, to see where each one starts and ends.
var tags = Theme{
	Time:   "<t>",
	Level:  [...]string{"<l0>", "<l1>", "<l2>", "<l3>", "<l4>", "<l5>", "<l6>"},
	Key:    "<k>",
	Prefix: "<p>",
	Marker: "<m>",
	Reset:  "</>",
}

// rendered parses the line and renders it the way the pipeline does.
func rendered(t *testing.T, r *Renderer, line string) string {
	t.Helper()
	p := parse.Parser{Loc: r.Loc}
	// Clipped, so that a read past the end of the line panics instead of finding spare capacity.
	out := string(r.AppendEntry(nil, p.Parse(slices.Clip([]byte(line)))))
	text, ok := strings.CutSuffix(out, "\n")
	if !ok || strings.Contains(text, "\n") {
		t.Fatalf("AppendEntry(%q) = %q, want one line that ends in a newline", line, out)
	}
	return text
}

func TestAppendEntry(t *testing.T) {
	tests := []struct {
		name string
		line string
		want string
	}{
		{
			name: "everything",
			line: `{"time":"2026-10-11T12:00:00.123Z","level":"warn","msg":"slow request","latency":"12ms","status":200}`,
			want: `15:00:00.123 WARN  slow request  latency=12ms status=200`,
		},
		{name: "no time", line: `{"level":"info","msg":"started","port":8080}`, want: `INFO  started  port=8080`},
		{name: "no level", line: `{"time":"2026-10-11T12:00:00Z","msg":"hello","a":1}`, want: `15:00:00.000 hello  a=1`},
		{name: "no message", line: `{"time":"2026-10-11T12:00:00Z","level":"error","code":500}`, want: `15:00:00.000 ERROR code=500`},
		{name: "no message after a short level", line: `{"level":"info","code":1}`, want: `INFO  code=1`},
		{name: "fields only", line: `{"a":1,"b":"x y"}`, want: `a=1 b="x y"`},
		{name: "message only", line: `{"msg":"hello"}`, want: `hello`},
		{name: "level only", line: `{"level":"debug"}`, want: `DEBUG`},
		{name: "short level only has no padding", line: `{"level":"info"}`, want: `INFO`},
		{name: "time only", line: `{"ts":1791720000}`, want: `15:00:00.000`},
		{name: "time and level", line: `{"ts":1791720000,"level":"warn"}`, want: `15:00:00.000 WARN`},
		{name: "time and message", line: `{"ts":1791720000,"msg":"x"}`, want: `15:00:00.000 x`},
		{name: "every level", line: `{"level":"trace","msg":"x"}`, want: `TRACE x`},
		{name: "fatal", line: `{"level":"panic","msg":"x"}`, want: `FATAL x`},
		{
			name: "fields keep their order and the roles are not repeated",
			line: `{"b":2,"msg":"m","a":1,"level":"info","c":3,"time":"2026-10-11T12:00:00Z"}`,
			want: `15:00:00.000 INFO  m  b=2 a=1 c=3`,
		},
		{name: "duplicate keys", line: `{"msg":"a","msg":"b","x":1,"x":2}`, want: `a  msg=b x=1 x=2`},
		{name: "a level that cannot be read stays a field", line: `{"level":"bogus","msg":"x"}`, want: `x  level=bogus`},
		{name: "a time that cannot be read stays a field", line: `{"time":"yesterday","msg":"x"}`, want: `x  time=yesterday`},
		{
			name: "level implied by the format has no field to hide",
			line: `{"@t":"2026-10-11T12:00:00Z","@mt":"User {Name} logged in","Name":"bob"}`,
			want: `15:00:00.000 INFO  User {Name} logged in  Name=bob`,
		},
		{
			name: "logfmt",
			line: `time=2026-10-11T12:00:00Z level=info msg="request done" status=200`,
			want: `15:00:00.000 INFO  request done  status=200`,
		},
		{name: "logfmt word without a value", line: `level=info msg=x verbose a=1`, want: `INFO  x  verbose a=1`},
		{name: "logfmt word with a quote in it is not a key", line: `level=info msg=x "odd a=1 b=2`, want: `INFO  x  "odd a=1 b=2`},
		{name: "logfmt empty value", line: `a= b=2`, want: `a="" b=2`},
		{name: "slog level with an offset", line: `level=INFO+2 msg=custom a=1`, want: `INFO  custom  a=1`},

		{name: "collector prefix", line: `web-1    | {"level":"info","msg":"started"}`, want: `web-1    | INFO  started`},
		{
			name: "collector timestamp is not the time of the entry",
			line: `2026-10-11T12:00:00.123456789Z {"level":"error","msg":"boom"}`,
			want: `2026-10-11T12:00:00.123456789Z ERROR boom`,
		},
		{
			name: "collector prefix and a time of its own",
			line: `web-1 | {"time":"2026-10-11T12:00:00Z","msg":"x"}`,
			want: `web-1 | 15:00:00.000 x`,
		},
		{
			name: "text before the pairs stays as written",
			line: `E1011 12:00:02.000003   12345 controller.go:10] "Failed to sync" err="timeout" key="ns/x"`,
			want: `E1011 12:00:02.000003   12345 controller.go:10] "Failed to sync" err=timeout key=ns/x`,
		},
		{name: "text before pairs that have roles", line: `2026/10/11 12:00:00 level=info msg=x`, want: `2026/10/11 12:00:00 INFO  x`},
		{name: "control character in a prefix", line: "a\x1bb | {\"msg\":\"x\"}", want: `a\x1bb | x`},

		{name: "empty object is printed as it is", line: `{}`, want: `{}`},
		{name: "empty object in whitespace", line: ` {} `, want: ` {} `},
		{name: "empty object after a prefix", line: `svc | {}`, want: `svc | {}`},
		{name: "plain", line: `hello world`, want: `hello world`},
		{name: "empty line", line: ``, want: ``},
		{name: "leading tab", line: "\tat main.go:30 +0x45", want: "\tat main.go:30 +0x45"},
		{name: "leading and trailing spaces", line: `  indented  `, want: `  indented  `},
		{name: "control characters in a plain line", line: "a\x1b[31mb\rc", want: `a\x1b[31mb\rc`},
		{name: "invalid UTF-8 in a plain line", line: "a\xffb", want: "a\ufffdb"},

		{name: "byte order mark before a plain line", line: "\ufeffhello", want: `hello`},
		{name: "byte order mark before JSON", line: "\ufeff" + `{"msg":"x"}`, want: `x`},
		{name: "byte order mark before an empty object", line: "\ufeff{}", want: `{}`},
		{name: "byte order mark alone", line: "\ufeff", want: ``},
		{name: "byte order mark in the middle", line: "a\ufeffb", want: `a\ufeffb`},
		{name: "two byte order marks", line: "\ufeff\ufeffa", want: `\ufeffa`},

		{name: "escapes in a message", line: `{"msg":"a\u001b[31mb\nc"}`, want: `a\x1b[31mb\nc`},
		{name: "message is not quoted", line: `{"msg":"say \"hi\" there"}`, want: `say "hi" there`},
		{name: "tab in a message and in a value is written out", line: `{"msg":"a\tb","k":"c\td"}`, want: `a\tb  k=c\td`},
		{name: "tab in the text before logfmt pairs stays", line: "one\ttwo three a=1 b=2", want: "one\ttwo three a=1 b=2"},
		{name: "tab in a quoted logfmt value is written out", line: "aaaaaaaa=1 b=\"x\ty\"", want: `aaaaaaaa=1 b=x\ty`},
		{name: "tab after a quote that is not closed is written out", line: "aaaaaaaaaa=1 bbbbbbbbbb=2 c=\"x\ty", want: `aaaaaaaaaa=1 bbbbbbbbbb=2 c="x\ty`},
		{name: "spaces at the end of a message are dropped", line: `{"level":"info","msg":"x  ","a":1}`, want: `INFO  x  a=1`},
		{name: "tabs at the end of a message are written out", line: `{"msg":"x\t \t","a":1}`, want: `x\t \t  a=1`},
		{name: "a message of a tab", line: `{"msg":"\t"}`, want: `\t`},
		{name: "spaces at the end of the last part are dropped", line: `{"level":"info","msg":"x  "}`, want: `INFO  x`},
		{name: "spaces at the start of a message are kept", line: `{"msg":"  x"}`, want: `  x`},
		{name: "a message of spaces is not printed", line: `{"level":"warn","msg":"  ","a":1}`, want: `WARN  a=1`},
		{name: "a message of spaces alone", line: `{"msg":" "}`, want: ``},
		{name: "message that only logfmt quotes", line: `msg="a b" x=1`, want: `a b  x=1`},
		{
			name: "nested values",
			line: `{"msg":"req","http":{"method":"GET", "path":"/x y"},"tags":["a", "b"]}`,
			want: `req  http={"method":"GET","path":"/x y"} tags=["a","b"]`,
		},
		{name: "keys that need quotes", line: `{"user id":5,"a=b":1,"":2}`, want: `"user id"=5 "a=b"=1 ""=2`},
		{name: "key with an escape", line: `{"k\u00e9y":1,"a\nb":2}`, want: `kéy=1 a\nb=2`},
		{name: "literals", line: `{"ok":true,"err":null,"n":-1.50e3}`, want: `ok=true err=null n=-1.50e3`},
		{name: "empty string", line: `{"a":""}`, want: `a=""`},
		{name: "value with an escape character", line: `{"a":"x\u001by"}`, want: `a=x\x1by`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := Renderer{Loc: testLoc}
			if got := rendered(t, &r, tt.line); got != tt.want {
				t.Errorf("line %q\n got %q\nwant %q", tt.line, got, tt.want)
			}
		})
	}
}

func TestAppendEntryTime(t *testing.T) {
	tests := []struct {
		name string
		line string
		want string
	}{
		{name: "UTC", line: `{"time":"2026-10-11T12:00:00Z"}`, want: `15:00:00.000`},
		{name: "another zone", line: `{"time":"2026-10-11T12:00:00+02:00"}`, want: `13:00:00.000`},
		{name: "the zone of the reader", line: `{"time":"2026-10-11T12:00:00+03:00"}`, want: `12:00:00.000`},
		{name: "no zone is shown as written", line: `{"time":"2026-10-11 12:00:00,123"}`, want: `12:00:00.123`},
		{name: "Unix seconds with a fraction", line: `{"ts":1791720000.5}`, want: `15:00:00.500`},
		{name: "Unix milliseconds", line: `{"time":1791720000123}`, want: `15:00:00.123`},
		{name: "fraction is cut, not rounded", line: `{"time":"2026-10-11T12:00:00.9999Z"}`, want: `15:00:00.999`},
		{name: "nanoseconds", line: `{"time":"2026-10-11T12:00:00.123456789Z"}`, want: `15:00:00.123`},
		{name: "past midnight in the zone of the reader", line: `{"time":"2026-10-11T22:30:00Z"}`, want: `01:30:00.000`},
		{name: "single digits are padded", line: `{"time":"2026-10-11T01:02:03.004Z"}`, want: `04:02:03.004`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := Renderer{Loc: testLoc}
			if got := rendered(t, &r, tt.line); got != tt.want {
				t.Errorf("line %q\n got %q\nwant %q", tt.line, got, tt.want)
			}
		})
	}
}

func TestAppendEntryWithoutLocationIsLocal(t *testing.T) {
	const line = `{"time":"2026-10-11T12:00:00Z","msg":"x"}`
	var local Renderer
	explicit := Renderer{Loc: time.Local}
	if got, want := rendered(t, &local, line), rendered(t, &explicit, line); got != want {
		t.Errorf("without a location the line is %q, want it as in time.Local: %q", got, want)
	}
}

func TestAppendEntryTheme(t *testing.T) {
	tests := []struct {
		name  string
		theme Theme
		line  string
		want  string
	}{
		{
			name:  "every part",
			theme: tags,
			line:  `web-1 | {"time":"2026-10-11T12:00:00Z","level":"warn","msg":"slow","a":1,"b":"x y"}`,
			want:  `<p>web-1 |</> <t>15:00:00.000</> <l4>WARN</>  slow  <k>a=</>1 <k>b=</>"x y"`,
		},
		{
			name:  "text before the pairs and a word without a value have no style",
			theme: tags,
			line:  `2026/10/11 12:00:00 level=error msg=x verbose a=1`,
			want:  `2026/10/11 12:00:00 <l5>ERROR</> x  verbose <k>a=</>1`,
		},
		{name: "each level has its style", theme: tags, line: `{"level":"trace"}`, want: `<l1>TRACE</>`},
		{name: "the quotes of a key are inside its style", theme: tags, line: `{"a b":1}`, want: `<k>"a b"=</>1`},
		{name: "a plain line has no style", theme: tags, line: `hello`, want: `hello`},
		{name: "an empty object has no style, what the collector wrote has", theme: tags, line: `svc | {}`, want: `<p>svc |</> {}`},
		{name: "a plain line after a container name", theme: tags, line: `web-1 | Traceback`, want: `<p>web-1 |</> Traceback`},
		{
			name:  "a plain line after a pod name and a timestamp",
			theme: tags,
			line:  "[pod/web-1/app] 2026-10-11T12:00:00Z \tat x",
			want:  "<p>[pod/web-1/app]</> <p>2026-10-11T12:00:00Z</> \tat x",
		},
		{
			name:  "a part without a style is not reset",
			theme: Theme{Time: "<t>", Reset: "</>"},
			line:  `svc | {"time":"2026-10-11T12:00:00Z","level":"info","msg":"m","a":1}`,
			want:  `svc | <t>15:00:00.000</> INFO  m  a=1`,
		},
		{
			name: "no theme",
			line: `web-1 | {"time":"2026-10-11T12:00:00Z","level":"warn","msg":"slow","a":1}`,
			want: `web-1 | 15:00:00.000 WARN  slow  a=1`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := Renderer{Theme: tt.theme, Loc: testLoc}
			if got := rendered(t, &r, tt.line); got != tt.want {
				t.Errorf("line %q\n got %q\nwant %q", tt.line, got, tt.want)
			}
		})
	}
}

// An entry built by hand may hold a level, a format or a time that the parser never gives.
func TestAppendEntryOddEntries(t *testing.T) {
	field := entry.Field{Key: []byte("a"), Value: num("1")}
	tests := []struct {
		name string
		e    entry.Entry
		want string
	}{
		{name: "zero entry", e: entry.Entry{}, want: "\n"},
		{name: "level out of range", e: entry.Entry{Format: entry.FormatJSON, Level: 100, Fields: []entry.Field{field}}, want: "a=1\n"},
		{name: "negative level", e: entry.Entry{Format: entry.FormatJSON, Level: -1, Fields: []entry.Field{field}}, want: "a=1\n"},
		{name: "plain with fields", e: entry.Entry{Line: []byte("raw"), Fields: []entry.Field{field}}, want: "raw\n"},
		{
			name: "tab in a word without a value",
			e:    entry.Entry{Format: entry.FormatLogfmt, Fields: []entry.Field{{Key: []byte("a\tb")}, field}},
			want: `a\tb a=1` + "\n",
		},
		{
			name: "time and level without fields of their own",
			e: entry.Entry{
				Format: entry.FormatJSON, Fields: []entry.Field{field},
				Time: time.Date(2026, 10, 11, 12, 0, 0, 0, time.UTC), Level: entry.LevelWarn,
			},
			want: "12:00:00.000 WARN  a=1\n",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := Renderer{Loc: time.UTC}
			if got := string(r.AppendEntry(nil, &tt.e)); got != tt.want {
				t.Errorf("AppendEntry = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestAppendEntryKeepsDst(t *testing.T) {
	p := parse.Parser{Loc: testLoc}
	r := Renderer{Loc: testLoc}
	dst := r.AppendEntry([]byte("first\n"), p.Parse([]byte(`{"msg":"second"}`)))
	dst = r.AppendTruncated(dst, []byte("third"), 9)
	if want := "first\nsecond\nthird…[+9B]\n"; string(dst) != want {
		t.Errorf("after three lines dst = %q, want %q", dst, want)
	}
}

// One renderer serves every line: nothing of a line may show in the next one.
func TestAppendEntryMixedStream(t *testing.T) {
	lines := []string{
		`{"time":"2026-10-11T12:00:00Z","level":"error","msg":"a long message with \"escapes\" \n in it","a":"b\tc"}`,
		`plain text`,
		`svc | level=info msg=started`,
		``,
		`{"msg":"x"}`,
		`{}`,
		`a=1 b=2`,
		`{"level":"debug","k":"v\u00e9"}`,
	}
	shared := Renderer{Theme: tags, Loc: testLoc}
	for range 2 {
		for _, line := range lines {
			fresh := Renderer{Theme: tags, Loc: testLoc}
			if got, want := rendered(t, &shared, line), rendered(t, &fresh, line); got != want {
				t.Errorf("line %q after other lines\n got %q\nwant %q", line, got, want)
			}
		}
	}
}

func TestAppendTruncated(t *testing.T) {
	tests := []struct {
		name    string
		theme   Theme
		line    string
		dropped int
		want    string
	}{
		{name: "marker", line: "abc", dropped: 5, want: "abc…[+5B]"},
		{name: "larger unit", line: "abc", dropped: 3000, want: "abc…[+2.9KiB]"},
		{name: "rune cut by the limit", line: "ab\xd0", dropped: 1, want: "ab\ufffd…[+1B]"},
		{name: "control characters", line: "a\x1bb", dropped: 1, want: `a\x1bb…[+1B]`},
		{name: "tab stays", line: "a\tb", dropped: 1, want: "a\tb…[+1B]"},
		{name: "JSON is not parsed", line: `{"msg":"x","a":"`, dropped: 10, want: `{"msg":"x","a":"…[+10B]`},
		{name: "byte order mark", line: "\ufeffabc", dropped: 2, want: "abc…[+2B]"},
		{name: "nothing kept", line: "", dropped: 7, want: "…[+7B]"},
		{name: "theme", theme: tags, line: "abc", dropped: 5, want: "abc<m>…[+5B]</>"},
		{name: "theme without a marker style", theme: Theme{Reset: "</>"}, line: "abc", dropped: 5, want: "abc…[+5B]"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := Renderer{Theme: tt.theme}
			if got := string(r.AppendTruncated(nil, []byte(tt.line), tt.dropped)); got != tt.want+"\n" {
				t.Errorf("AppendTruncated(%q, %d) = %q, want %q and a newline", tt.line, tt.dropped, got, tt.want)
			}
		})
	}
}

func TestRenderDoesNotAllocate(t *testing.T) {
	lines := []string{
		`{"time":"2026-10-11T12:00:00.123+03:00","level":"info","msg":"request done","status":200,"tags":["a","b"]}`,
		`{"time":"2026-10-11T12:00:00Z","level":"warn","msg":"line one\nline \"two\"","path":"C:\\dir \u00e9","http":{"a": 1}}`,
		`svc-1  | {"level":"info","msg":"request done"}`,
		`time=2026-10-11T12:00:00Z level=info msg="request \"done\"" status=200 verbose`,
		`2026/10/11 12:00:01 INFO hello a=1 b=2`,
		`2026-10-11 12:00:00 INFO request done in 5ms`,
		"\ufeffplain with \x1b and \xff",
		``,
	}
	for _, theme := range []Theme{{}, tags, DefaultTheme()} {
		// The zone is set: the first use of time.Local loads it, which allocates once.
		r := Renderer{Theme: theme, Loc: testLoc}
		dst := make([]byte, 0, 4096)
		for _, line := range lines {
			// The entry is parsed once, outside the measured function: the parser has its own allocation tests.
			p := parse.Parser{Loc: testLoc}
			e := p.Parse([]byte(line))
			dst = r.AppendEntry(dst[:0], e) // grows the scratch space
			if allocs := testing.AllocsPerRun(100, func() { dst = r.AppendEntry(dst[:0], e) }); allocs != 0 {
				t.Errorf("AppendEntry allocates %v times for %q, want 0", allocs, line)
			}
		}
		line := []byte("a truncated line \x1b")
		if allocs := testing.AllocsPerRun(100, func() { dst = r.AppendTruncated(dst[:0], line, 123456) }); allocs != 0 {
			t.Errorf("AppendTruncated allocates %v times, want 0", allocs)
		}
	}
}
