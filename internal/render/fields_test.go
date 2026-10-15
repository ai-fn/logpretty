package render

import (
	"slices"
	"testing"

	"github.com/ai-fn/logpretty/internal/parse"
)

// appended renders the line after a marker and returns what was added: nothing at all for an entry that is
// not printed.
func appended(r *Renderer, line string) string {
	p := parse.Parser{Loc: r.Loc}
	// Clipped, so that a read past the end of the line panics instead of finding spare capacity.
	out := string(r.AppendEntry([]byte(">"), p.Parse(slices.Clip([]byte(line)))))
	return out[1:]
}

func TestAppendEntryFields(t *testing.T) {
	const (
		full    = `{"time":"2026-10-11T12:00:00Z","level":"warn","msg":"slow","user_id":42,"latency":"1.2s","http":{"status":502,"path":"/x y"}}`
		renamed = `{"ts":1791720000,"severity":"error","message":"boom","a":1}`
		serilog = `{"@t":"2026-10-11T12:00:00Z","@mt":"hi","Name":"bob"}`
		logfmt  = `time=2026-10-11T12:00:00Z level=info msg="request done" status=200 path=/x`
	)

	tests := []struct {
		name   string
		fields []string
		line   string
		want   string // without its newline; empty when nothing is printed
	}{
		{name: "the order of the list", fields: []string{"latency", "user_id"}, line: full, want: `15:00:00.000 WARN  latency=1.2s user_id=42`},
		{name: "the other order", fields: []string{"user_id", "latency"}, line: full, want: `15:00:00.000 WARN  user_id=42 latency=1.2s`},
		{name: "the message first", fields: []string{"msg", "latency"}, line: full, want: `15:00:00.000 WARN  slow  latency=1.2s`},
		{name: "the message keeps its place", fields: []string{"latency", "msg", "user_id"}, line: full, want: `15:00:00.000 WARN  slow  latency=1.2s user_id=42`},
		{name: "the message alone", fields: []string{"msg"}, line: full, want: `15:00:00.000 WARN  slow`},
		{name: "a field the entry lacks is skipped", fields: []string{"msg", "nope", "latency"}, line: full, want: `15:00:00.000 WARN  slow  latency=1.2s`},
		{name: "a nested field", fields: []string{"http.status"}, line: full, want: `15:00:00.000 WARN  http.status=502`},
		{name: "a nested field that needs quotes", fields: []string{"http.path"}, line: full, want: `15:00:00.000 WARN  http.path="/x y"`},
		{name: "a whole object", fields: []string{"http"}, line: full, want: `15:00:00.000 WARN  http={"status":502,"path":"/x y"}`},
		{name: "a name given twice is shown once", fields: []string{"latency", "msg", "latency", "msg"}, line: full, want: `15:00:00.000 WARN  slow  latency=1.2s`},
		{name: "time and level are shown anyway", fields: []string{"time", "level", "msg"}, line: full, want: `15:00:00.000 WARN  slow`},
		{name: "the time alone", fields: []string{"time"}, line: full, want: `15:00:00.000 WARN`},
		{name: "the level alone", fields: []string{"level"}, line: full, want: `15:00:00.000 WARN`},
		{name: "none of the fields", fields: []string{"nope", "http.nope"}, line: full},
		{name: "the case of a name counts", fields: []string{"Latency"}, line: full},

		{name: "the message under another key", fields: []string{"msg", "a"}, line: renamed, want: `15:00:00.000 ERROR boom  a=1`},
		{name: "the key of the message is a field like any other", fields: []string{"message"}, line: renamed, want: `15:00:00.000 ERROR message=boom`},
		{name: "the keys of the time and the level too", fields: []string{"ts", "severity"}, line: renamed, want: `15:00:00.000 ERROR ts=1791720000 severity=error`},
		{name: "a field named msg that is no message", fields: []string{"msg"}, line: `{"level":"info","msg":42,"a":1}`, want: `INFO  msg=42`},
		{name: "the message wins over a field named msg", fields: []string{"msg"}, line: `{"message":"boom","msg":42}`, want: `boom`},
		{name: "a field named time that is no time", fields: []string{"msg", "time"}, line: `{"time":"yesterday","msg":"x"}`, want: `x  time=yesterday`},
		{name: "a field named level that is no level", fields: []string{"level"}, line: `{"level":"bogus","msg":"x"}`, want: `level=bogus`},
		{name: "a level without a field", fields: []string{"level"}, line: serilog, want: `15:00:00.000 INFO`},
		{name: "a level without a field and the message", fields: []string{"msg"}, line: serilog, want: `15:00:00.000 INFO  hi`},
		{name: "a level alone is not what was asked for", fields: []string{"msg"}, line: `{"level":"info","a":1}`},
		{name: "a time alone is not what was asked for", fields: []string{"a"}, line: `{"time":"2026-10-11T12:00:00Z","level":"info","msg":"ok"}`},
		{name: "a message of spaces is no message", fields: []string{"msg"}, line: `{"level":"info","msg":"  "}`},
		{name: "a message of spaces next to a field", fields: []string{"msg", "a"}, line: `{"msg":"  ","a":1}`, want: `a=1`},

		{name: "a key as the list has it, in quotes if it needs them", fields: []string{"user id"}, line: `{"user id":5}`, want: `"user id"=5`},
		{name: "a key with a dot wins over the way down", fields: []string{"http.status"}, line: `{"http":{"status":502},"http.status":200}`, want: `http.status=200`},
		{name: "the first of two keys", fields: []string{"x"}, line: `{"x":1,"x":2}`, want: `x=1`},
		{name: "values of every kind", fields: []string{"s", "n", "b", "z", "a", "e"}, line: `{"e":"","a":[1, 2],"z":null,"b":true,"n":-1.5e3,"s":"x y"}`, want: `s="x y" n=-1.5e3 b=true z=null a=[1,2] e=""`},
		{name: "a value with an escape", fields: []string{"a"}, line: `{"a":"x\ty \"z\""}`, want: `a="x\ty \"z\""`},
		{name: "a control character in a name", fields: []string{"a\x1bb"}, line: "{\"a\\u001bb\":1}", want: `a\x1bb=1`},

		{name: "logfmt", fields: []string{"path", "msg", "status"}, line: logfmt, want: `15:00:00.000 INFO  request done  path=/x status=200`},
		{name: "a logfmt word without a value is not a field", fields: []string{"verbose", "a"}, line: `level=info msg=x verbose a=1`, want: `INFO  a=1`},
		{name: "the prefix of a collector stays", fields: []string{"a"}, line: `web-1 | {"level":"info","msg":"started","a":1}`, want: `web-1 | INFO  a=1`},
		{name: "the text before the pairs stays", fields: []string{"a"}, line: `2026/10/11 12:00:00 level=info msg=x a=1`, want: `2026/10/11 12:00:00 INFO  a=1`},
		{name: "a prefix alone is not what was asked for", fields: []string{"b"}, line: `web-1 | {"a":1}`},
		{name: "the time a collector wrote is the column", fields: []string{"a"}, line: `2026-10-11T12:00:00Z {"level":"info","a":1}`, want: `15:00:00.000 INFO  a=1`},
		{name: "the time a collector wrote is the time asked for", fields: []string{"time"}, line: `[pod/web-1/app] 2026-10-11T12:00:00Z {"a":1}`, want: `[pod/web-1/app] 15:00:00.000`},
		{name: "a pod and a stamp alone are not what was asked for", fields: []string{"b"}, line: `[pod/web-1/app] 2026-10-11T12:00:00Z {"a":1}`},

		{name: "a plain line is not touched", fields: []string{"a"}, line: `hello a world`, want: `hello a world`},
		{name: "an empty line is not touched", fields: []string{"a"}, line: ``, want: ``},
		{name: "an entry without fields is not touched", fields: []string{"a"}, line: `{}`, want: `{}`},
		{name: "an entry without fields after a prefix", fields: []string{"a"}, line: `svc | {}`, want: `svc | {}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := Renderer{Loc: testLoc, Fields: Select(tt.fields)}
			want := tt.want + "\n"
			if tt.want == "" && tt.line != "" {
				want = ""
			}
			if got := appended(&r, tt.line); got != want {
				t.Errorf("fields %q, line %q\n got %q\nwant %q", tt.fields, tt.line, got, want)
			}
		})
	}
}

func TestAppendEntryNoTime(t *testing.T) {
	const full = `{"time":"2026-10-11T12:00:00Z","level":"warn","msg":"slow","a":1}`

	tests := []struct {
		name   string
		fields []string
		line   string
		want   string // with its newline; empty when nothing is printed
	}{
		{name: "the column is gone and its field does not come back", line: full, want: "WARN  slow  a=1\n"},
		{name: "without a level", line: `{"time":"2026-10-11T12:00:00Z","msg":"x"}`, want: "x\n"},
		{name: "an entry without a time", line: `{"level":"info","msg":"x"}`, want: "INFO  x\n"},
		{name: "a level without a field", line: `{"@t":"2026-10-11T12:00:00Z","@mt":"hi"}`, want: "INFO  hi\n"},
		{name: "after a prefix", line: `web-1 | {"time":"2026-10-11T12:00:00Z","msg":"x"}`, want: "web-1 | x\n"},
		{name: "a time that cannot be read is a field", line: `{"time":"yesterday","msg":"x"}`, want: "x  time=yesterday\n"},
		{name: "the time was all the entry had", line: `{"ts":1791720000}`},
		{name: "the time was all the entry had after its prefix", line: `svc | {"ts":1791720000}`},
		{name: "the time and a message of spaces", line: `{"ts":1791720000,"msg":" "}`},
		{name: "a message of spaces without a time is still an empty line", line: `{"msg":" "}`, want: "\n"},
		{name: "the time a collector wrote is hidden too", line: `2026-10-11T12:00:00.123456789Z {"level":"error","msg":"boom"}`, want: "ERROR boom\n"},
		{name: "the time a collector wrote before a plain line", line: "[pod/web-1/app] 2026-10-11T12:00:00Z \tat x", want: "[pod/web-1/app] \tat x\n"},
		{name: "a plain line of a collector's time alone is still a line", line: "2026-10-11T12:00:00Z", want: "\n"},
		{name: "a plain line of a collector's prefix and time", line: "web-1 | 2026-10-11T12:00:00Z", want: "web-1 |\n"},
		{name: "a plain line is not touched", line: `2026-10-11 12:00:00 started`, want: "2026-10-11 12:00:00 started\n"},
		{name: "an entry without fields is not touched", line: `{}`, want: "{}\n"},

		{name: "with fields", fields: []string{"msg", "a"}, line: full, want: "WARN  slow  a=1\n"},
		{name: "the time asked for is still hidden", fields: []string{"time", "msg"}, line: full, want: "WARN  slow\n"},
		{name: "the time asked for alone leaves nothing", fields: []string{"time"}, line: full},
		{name: "the level asked for alone", fields: []string{"level"}, line: full, want: "WARN\n"},
		{name: "the key of the time asked for by its own name is a field", fields: []string{"ts"}, line: `{"ts":1791720000,"msg":"x"}`, want: "ts=1791720000\n"},
		{name: "a field named time that is no time, asked for", fields: []string{"time"}, line: `{"time":"yesterday","msg":"x"}`, want: "time=yesterday\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := Renderer{Loc: testLoc, NoTime: true, Fields: Select(tt.fields)}
			if got := appended(&r, tt.line); got != tt.want {
				t.Errorf("fields %q, line %q\n got %q\nwant %q", tt.fields, tt.line, got, tt.want)
			}
		})
	}
}

func TestAppendEntryFieldsTheme(t *testing.T) {
	const line = `web-1 | {"time":"2026-10-11T12:00:00Z","level":"warn","msg":"slow","a":1,"b":"x y","http":{"status":502}}`

	tests := []struct {
		name   string
		noTime bool
		fields []string
		want   string
	}{
		{name: "fields", fields: []string{"b", "http.status", "a"}, want: `<p>web-1 |</> <t>15:00:00.000</> <l4>WARN</>  <k>b=</>"x y" <k>http.status=</>502 <k>a=</>1` + "\n"},
		{name: "the message", fields: []string{"msg"}, want: `<p>web-1 |</> <t>15:00:00.000</> <l4>WARN</>  slow` + "\n"},
		{name: "no time", noTime: true, want: `<p>web-1 |</> <l4>WARN</>  slow  <k>a=</>1 <k>b=</>"x y" <k>http=</>{"status":502}` + "\n"},
		{name: "nothing to print leaves no style behind", fields: []string{"nope"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := Renderer{Theme: tags, Loc: testLoc, NoTime: tt.noTime, Fields: Select(tt.fields)}
			if got := appended(&r, line); got != tt.want {
				t.Errorf("fields %q\n got %q\nwant %q", tt.fields, got, tt.want)
			}
		})
	}
}

func TestSelectNothingShowsEverything(t *testing.T) {
	if Select(nil) != nil || Select([]string{}) != nil {
		t.Error("Select of no names is a choice, want none: every field is shown")
	}
	const line = `{"level":"info","msg":"m","a":1,"b":2}`
	var all Renderer
	chosen := Renderer{Fields: Select(nil)}
	if got, want := appended(&chosen, line), appended(&all, line); got != want {
		t.Errorf("with Select(nil) the line is %q, want it as without: %q", got, want)
	}
}

// The names belong to whoever called Select: a later change of them must not reach the renderer.
func TestSelectCopiesTheNames(t *testing.T) {
	names := []string{"a", "b"}
	r := Renderer{Fields: Select(names)}
	names[0], names[1] = "x", "y"
	if got, want := appended(&r, `{"a":1,"b":2,"x":3}`), "a=1 b=2\n"; got != want {
		t.Errorf("after the names were changed the line is %q, want %q", got, want)
	}
}

// One renderer serves every line: nothing of a line, or of an entry that was not printed, may show in the next.
func TestAppendEntryFieldsMixedStream(t *testing.T) {
	lines := []string{
		`{"time":"2026-10-11T12:00:00Z","level":"error","msg":"a \"long\" message","http":{"status":502,"path":"/x"},"a":"b\tc"}`,
		`plain text`,
		`{"level":"info","msg":"nothing asked for"}`,
		`svc | level=info a=1 http.status=200`,
		`{}`,
		`{"http":{"a":{"status":1}},"http.status":"x\ny"}`,
	}
	fields := []string{"http.status", "a", "nope"}
	shared := Renderer{Theme: tags, Loc: testLoc, Fields: Select(fields)}
	for range 2 {
		for _, line := range lines {
			fresh := Renderer{Theme: tags, Loc: testLoc, Fields: Select(fields)}
			if got, want := appended(&shared, line), appended(&fresh, line); got != want {
				t.Errorf("line %q after other lines\n got %q\nwant %q", line, got, want)
			}
		}
	}
}

func TestRenderFieldsDoesNotAllocate(t *testing.T) {
	lines := []string{
		`{"time":"2026-10-11T12:00:00.123+03:00","level":"info","msg":"request done","status":200,"http":{"status":502,"path":"/x"}}`,
		`{"level":"warn","msg":"line \"two\"","path":"C:\\dir","user_id":42}`,
		`svc-1  | {"level":"info","msg":"nothing asked for"}`,
		`time=2026-10-11T12:00:00Z level=info msg="request \"done\"" status=200 verbose`,
		`plain text`,
	}
	for _, theme := range []Theme{{}, DefaultTheme()} {
		// The zone is set: the first use of time.Local loads it, which allocates once.
		r := Renderer{Theme: theme, Loc: testLoc, NoTime: true, Fields: Select([]string{"msg", "http.status", "status", "path", "nope"})}
		dst := make([]byte, 0, 4096)
		for _, line := range lines {
			p := parse.Parser{Loc: testLoc}
			e := p.Parse([]byte(line))
			dst = r.AppendEntry(dst[:0], e) // grows the scratch space
			if allocs := testing.AllocsPerRun(100, func() { dst = r.AppendEntry(dst[:0], e) }); allocs != 0 {
				t.Errorf("AppendEntry allocates %v times for %q, want 0", allocs, line)
			}
		}
	}
}
