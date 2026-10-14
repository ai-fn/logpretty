package parse

import (
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/ai-fn/logpretty/internal/entry"
)

// testLoc stands in for the local zone, so that the tests do not depend on the machine they run on.
var testLoc = time.FixedZone("test", 3*60*60)

// roles is what assignRoles found, in a form that is easy to write and compare in tests.
type roles struct {
	time  string // RFC 3339 in UTC, empty when there is none
	level entry.Level
	// Keys of the fields, empty when there is none.
	timeKey, levelKey, msgKey string
}

// entryOf splits a JSON or logfmt line into fields and assigns the roles.
func entryOf(t *testing.T, line string) *entry.Entry {
	t.Helper()
	// Clipped, so that a read past the end of the line panics instead of finding spare capacity.
	b := slices.Clip([]byte(line))
	e := &entry.Entry{Line: b}
	fields, ok := appendJSONFields(nil, b)
	if !ok {
		if fields, _, ok = appendLogfmtFields(nil, b); !ok {
			t.Fatalf("%q is neither JSON nor logfmt", line)
		}
	}
	e.Fields = fields
	assignRoles(e, testLoc)
	return e
}

func rolesOf(t *testing.T, e *entry.Entry) roles {
	t.Helper()
	key := func(i int) string {
		if i < 0 {
			return ""
		}
		return string(e.Fields[i].Key)
	}
	got := roles{level: e.Level, timeKey: key(e.TimeField()), levelKey: key(e.LevelField()), msgKey: key(e.MsgField())}
	if !e.Time.IsZero() {
		got.time = e.Time.UTC().Format(time.RFC3339Nano)
	}
	// The one time without a field is the one a collector wrote.
	if (got.time == "") != (e.TimeField() < 0 && e.Stamp == nil) {
		t.Errorf("Time = %v with TimeField = %d and Stamp %q: one is set without the other", e.Time, e.TimeField(), e.Stamp)
	}
	// The one level without a field is the implied information of a Serilog event.
	implied := e.Level == entry.LevelInfo && e.LevelField() < 0 && strings.EqualFold(got.timeKey, "@t")
	if (e.Level == entry.LevelUnknown) != (e.LevelField() < 0) && !implied {
		t.Errorf("Level = %v with LevelField = %d: one is set without the other", e.Level, e.LevelField())
	}
	return got
}

func TestAssignRoles(t *testing.T) {
	tests := []struct {
		name string
		line string
		want roles
	}{
		{
			name: "zap",
			line: `{"level":"info","ts":1791720000.1234567,"caller":"app/main.go:42","msg":"server started","port":8080}`,
			want: roles{"2026-10-11T12:00:00.1234567Z", entry.LevelInfo, "ts", "level", "msg"},
		},
		{
			name: "logrus JSON",
			line: `{"level":"warning","msg":"disk almost full","time":"2026-10-11T12:00:00+03:00","used":91}`,
			want: roles{"2026-10-11T09:00:00Z", entry.LevelWarn, "time", "level", "msg"},
		},
		{
			name: "logrus text",
			line: `time="2026-10-11T12:00:00Z" level=info msg="server started" port=8080`,
			want: roles{"2026-10-11T12:00:00Z", entry.LevelInfo, "time", "level", "msg"},
		},
		{
			name: "zerolog with Unix time",
			line: `{"level":"debug","time":1791720000,"message":"cache miss"}`,
			want: roles{"2026-10-11T12:00:00Z", entry.LevelDebug, "time", "level", "message"},
		},
		{
			name: "zerolog with RFC 3339 time",
			line: `{"level":"error","error":"boom","time":"2026-10-11T12:00:00Z","message":"request failed"}`,
			want: roles{"2026-10-11T12:00:00Z", entry.LevelError, "time", "level", "message"},
		},
		{
			name: "slog JSON",
			line: `{"time":"2026-10-11T12:00:00.123456789+02:00","level":"INFO","msg":"hello","count":3}`,
			want: roles{"2026-10-11T10:00:00.123456789Z", entry.LevelInfo, "time", "level", "msg"},
		},
		{
			name: "slog text with a custom level",
			line: `time=2026-10-11T12:00:00.123Z level=INFO+2 msg="custom level" count=3`,
			want: roles{"2026-10-11T12:00:00.123Z", entry.LevelInfo, "time", "level", "msg"},
		},
		{
			name: "slog text with a level above error",
			line: `time=2026-10-11T12:00:00.123Z level=ERROR+4 msg=down`,
			want: roles{"2026-10-11T12:00:00.123Z", entry.LevelFatal, "time", "level", "msg"},
		},
		{
			name: "pino",
			line: `{"level":30,"time":1791720000123,"pid":4242,"hostname":"web-1","msg":"request completed"}`,
			want: roles{"2026-10-11T12:00:00.123Z", entry.LevelInfo, "time", "level", "msg"},
		},
		{
			name: "bunyan",
			line: `{"name":"app","hostname":"web-1","pid":4242,"level":50,"msg":"boom","time":"2026-10-11T12:00:00.123Z","v":0}`,
			want: roles{"2026-10-11T12:00:00.123Z", entry.LevelError, "time", "level", "msg"},
		},
		{
			name: "Serilog compact",
			line: `{"@t":"2026-10-11T12:00:00.1234567Z","@mt":"User {Name} logged in","@m":"User bob logged in","@l":"Warning","Name":"bob"}`,
			want: roles{"2026-10-11T12:00:00.1234567Z", entry.LevelWarn, "@t", "@l", "@m"},
		},
		{
			name: "Serilog compact without the rendered message and the level",
			line: `{"@t":"2026-10-11T12:00:00.1234567Z","@mt":"User {Name} logged in","Name":"bob"}`,
			want: roles{"2026-10-11T12:00:00.1234567Z", entry.LevelInfo, "@t", "", "@mt"},
		},
		{
			name: "Serilog compact keeps the level it names",
			line: `{"@t":"2026-10-11T12:00:00Z","@mt":"x","@l":"Error"}`,
			want: roles{"2026-10-11T12:00:00Z", entry.LevelError, "@t", "@l", "@mt"},
		},
		{
			name: "Serilog compact with a level that cannot be read has none",
			line: `{"@t":"2026-10-11T12:00:00Z","@mt":"x","@l":"Bogus"}`,
			want: roles{time: "2026-10-11T12:00:00Z", timeKey: "@t", msgKey: "@mt"},
		},
		{
			name: "Serilog compact with another level key uses it",
			line: `{"@t":"2026-10-11T12:00:00Z","@mt":"x","level":"warn"}`,
			want: roles{"2026-10-11T12:00:00Z", entry.LevelWarn, "@t", "level", "@mt"},
		},
		{
			name: "Serilog compact with a level key that cannot be read is still information",
			line: `{"@t":"2026-10-11T12:00:00Z","@mt":"x","level":"bogus"}`,
			want: roles{"2026-10-11T12:00:00Z", entry.LevelInfo, "@t", "", "@mt"},
		},
		{
			name: "Serilog compact keys in another letter case",
			line: `{"@T":"2026-10-11T12:00:00Z","@MT":"x"}`,
			want: roles{"2026-10-11T12:00:00Z", entry.LevelInfo, "@T", "", "@MT"},
		},
		{
			name: "Serilog compact level key in another letter case",
			line: `{"@T":"2026-10-11T12:00:00Z","@MT":"x","@L":"Bogus"}`,
			want: roles{time: "2026-10-11T12:00:00Z", timeKey: "@T", msgKey: "@MT"},
		},
		{
			name: "Serilog compact in logfmt",
			line: `@t=2026-10-11T12:00:00Z @mt=hello Name=bob`,
			want: roles{"2026-10-11T12:00:00Z", entry.LevelInfo, "@t", "", "@mt"},
		},
		{
			name: "Serilog compact with an unreadable time",
			line: `{"@t":"yesterday","@mt":"x"}`,
			want: roles{msgKey: "@mt"},
		},
		{
			name: "a time in another key leaves the level unknown",
			line: `{"time":"2026-10-11T12:00:00Z","msg":"x"}`,
			want: roles{time: "2026-10-11T12:00:00Z", timeKey: "time", msgKey: "msg"},
		},
		{
			name: "the time of a lower @t key does not imply a level",
			line: `{"time":"2026-10-11T12:00:00Z","@t":"2026-10-11T13:00:00Z"}`,
			want: roles{time: "2026-10-11T12:00:00Z", timeKey: "time"},
		},
		{
			name: "ECS",
			line: `{"@timestamp":"2026-10-11T12:00:00.123Z","log.level":"error","message":"boom","ecs.version":"1.6.0"}`,
			want: roles{"2026-10-11T12:00:00.123Z", entry.LevelError, "@timestamp", "log.level", "message"},
		},
		{
			name: "structlog JSON",
			line: `{"event":"user logged in","level":"info","timestamp":"2026-10-11T12:00:00.123456Z"}`,
			want: roles{"2026-10-11T12:00:00.123456Z", entry.LevelInfo, "timestamp", "level", "event"},
		},
		{
			name: "structlog key-value",
			line: `event="user logged in" level=info timestamp=2026-10-11T12:00:00Z`,
			want: roles{"2026-10-11T12:00:00Z", entry.LevelInfo, "timestamp", "level", "event"},
		},
		{
			name: "log15",
			line: `t=2026-10-11T12:00:00+0000 lvl=eror msg="write failed" err=EOF`,
			want: roles{"2026-10-11T12:00:00Z", entry.LevelError, "t", "lvl", "msg"},
		},
		{
			name: "python-json-logger",
			line: `{"asctime": "2026-10-11 12:00:00,123", "levelname": "INFO", "name": "app", "message": "started"}`,
			want: roles{"2026-10-11T09:00:00.123Z", entry.LevelInfo, "asctime", "levelname", "message"},
		},
		{
			name: ".NET console JSON",
			line: `{"Timestamp":"2026-10-11T12:00:00.123Z","EventId":0,"LogLevel":"Information","Category":"Program","Message":"Started"}`,
			want: roles{"2026-10-11T12:00:00.123Z", entry.LevelInfo, "Timestamp", "LogLevel", "Message"},
		},
		{
			name: "Ruby time with a space before the offset",
			line: `{"timestamp":"2026-10-11 12:00:00 +0000","level":"info","msg":"x"}`,
			want: roles{"2026-10-11T12:00:00Z", entry.LevelInfo, "timestamp", "level", "msg"},
		},
		{
			name: "keys in another letter case",
			line: `{"Timestamp":"2026-10-11T12:00:00Z","Level":"Information","MESSAGE":"Started"}`,
			want: roles{"2026-10-11T12:00:00Z", entry.LevelInfo, "Timestamp", "Level", "MESSAGE"},
		},
		{
			name: "time without a zone is local",
			line: `{"timestamp":"2026-10-11 12:00:00,123","severity":"WARNING","message":"slow query"}`,
			want: roles{"2026-10-11T09:00:00.123Z", entry.LevelWarn, "timestamp", "severity", "message"},
		},
		{
			name: "numbers written as logfmt strings",
			line: `ts=1791720000 level=30 msg=ok`,
			want: roles{"2026-10-11T12:00:00Z", entry.LevelInfo, "ts", "level", "msg"},
		},
		{
			name: "numbers written as JSON strings",
			line: `{"ts":"1791720000","level":"30","msg":"ok"}`,
			want: roles{"2026-10-11T12:00:00Z", entry.LevelInfo, "ts", "level", "msg"},
		},

		{name: "no known keys", line: `{"a":1,"b":"x"}`},
		{name: "no fields", line: `{}`},
		{name: "longer keys that start like known ones", line: `{"times":"2026-10-11T12:00:00Z","levels":"info","msgs":"x"}`},
		{name: "shorter keys that known ones start with", line: `{"tim":"2026-10-11T12:00:00Z","leve":"info","ms":"x"}`},
		{name: "a small number is not a time", line: `t=5 a=1`},
		{name: "an unknown word is not a level", line: `level=notaword a=1`},
		{name: "a number between two levels is not a level", line: `level=35 a=1`},
		{name: "values of other kinds", line: `{"time":null,"level":true,"msg":42,"message":{"text":"x"},"event":["x"]}`},
		{name: "a key without a value is not a message", line: `a=1 msg bc=23`},
		{
			name: "a message that is not a string gives way to one that is",
			line: `{"msg":42,"message":"real"}`,
			want: roles{msgKey: "message"},
		},
		{name: "an empty message is not a message", line: `{"msg":"","a":1}`},
		{
			name: "an empty message gives way to one with text",
			line: `{"msg":"","message":"real"}`,
			want: roles{msgKey: "message"},
		},
		{
			name: "a time that cannot be read gives way to one that can",
			line: `{"time":"yesterday","ts":1791720000}`,
			want: roles{time: "2026-10-11T12:00:00Z", timeKey: "ts"},
		},
		{
			name: "a level that cannot be read gives way to one that can",
			line: `{"level":"notaword","severity":"error"}`,
			want: roles{level: entry.LevelError, levelKey: "severity"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := rolesOf(t, entryOf(t, tt.line)); got != tt.want {
				t.Errorf("roles of %q\n got %+v\nwant %+v", tt.line, got, tt.want)
			}
		})
	}
}

// The first key of each list wins over the rest, wherever in the line it stands.
func TestAssignRolesPriority(t *testing.T) {
	tests := []struct {
		role  string
		keys  []string
		value string
		field func(*entry.Entry) int
	}{
		{"time", []string{"time", "timestamp", "@timestamp", "asctime", "ts", "@t", "t"}, "2026-10-11T12:00:00Z", (*entry.Entry).TimeField},
		{"level", []string{"level", "lvl", "severity", "log.level", "levelname", "log_level", "loglevel", "@l"}, "info", (*entry.Entry).LevelField},
		{"message", []string{"msg", "message", "@message", "@m", "event", "@mt"}, "x", (*entry.Entry).MsgField},
	}

	object := func(keys []string, value string) string {
		pairs := make([]string, 0, len(keys))
		for _, key := range keys {
			pairs = append(pairs, `"`+key+`":"`+value+`"`)
		}
		return "{" + strings.Join(pairs, ",") + "}"
	}

	for _, tt := range tests {
		for i, want := range tt.keys {
			// The best key of those left, first and last in the line.
			reversed := slices.Clone(tt.keys[i:])
			slices.Reverse(reversed)
			for _, keys := range [][]string{tt.keys[i:], reversed} {
				line := object(keys, tt.value)
				e := entryOf(t, line)
				if got := tt.field(e); got < 0 || string(e.Fields[got].Key) != want {
					t.Errorf("%s of %q is field %d, want the one with the key %q", tt.role, line, got, want)
				}
			}
		}
	}
}

// An entry is reused for every line: what the last one left must not survive in a line without roles.
func TestAssignRolesClearsOldRoles(t *testing.T) {
	e := entryOf(t, `{"a":1,"b":2,"time":"2026-10-11T12:00:00Z","level":"error","msg":"x"}`)
	if got := [3]int{e.TimeField(), e.LevelField(), e.MsgField()}; got != [3]int{2, 3, 4} {
		t.Fatalf("time, level and message are fields %v, want 2, 3 and 4", got)
	}

	e.Fields = e.Fields[:1]
	assignRoles(e, testLoc)

	if got := rolesOf(t, e); got != (roles{}) {
		t.Errorf("roles of an entry without known keys are %+v, want none", got)
	}
}

// The information a Serilog event implies belongs to that event: it must not reach the next line.
func TestAssignRolesClearsImpliedLevel(t *testing.T) {
	e := entryOf(t, `{"@t":"2026-10-11T12:00:00Z","@mt":"x"}`)
	if e.Level != entry.LevelInfo {
		t.Fatalf("level = %v, want info", e.Level)
	}

	e.Fields = e.Fields[:0]
	assignRoles(e, testLoc)

	if got := rolesOf(t, e); got != (roles{}) {
		t.Errorf("roles of an entry without fields are %+v, want none", got)
	}

	var p Parser
	p.Parse([]byte(`{"@t":"2026-10-11T12:00:00Z","@mt":"x"}`))
	if got := p.Parse([]byte(`a=1 b=2`)); got.Level != entry.LevelUnknown {
		t.Errorf("level of a line after a Serilog event = %v, want unknown", got.Level)
	}
}

func TestAssignRolesDuplicateKeys(t *testing.T) {
	tests := []struct {
		name string
		line string
		want [3]int // time, level, message
	}{
		{name: "the first of two messages", line: `{"msg":"a","msg":"b"}`, want: [3]int{-1, -1, 0}},
		{name: "the first of two times", line: `{"ts":1791720000,"ts":1791720001}`, want: [3]int{0, -1, -1}},
		{name: "the first of two levels", line: `{"level":"info","level":"warn"}`, want: [3]int{-1, 0, -1}},
		{name: "the first level that can be read", line: `{"level":"notaword","level":"warn"}`, want: [3]int{-1, 1, -1}},
		{name: "the first time that can be read", line: `{"ts":5,"ts":1791720000}`, want: [3]int{1, -1, -1}},
		{name: "the first message that is a string", line: `{"msg":1,"msg":"b"}`, want: [3]int{-1, -1, 1}},
		{name: "the first message with text", line: `{"msg":"","msg":"b"}`, want: [3]int{-1, -1, 1}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := entryOf(t, tt.line)
			if got := [3]int{e.TimeField(), e.LevelField(), e.MsgField()}; got != tt.want {
				t.Errorf("time, level and message of %q are fields %v, want %v", tt.line, got, tt.want)
			}
		})
	}
}

func TestParseTime(t *testing.T) {
	str := func(raw string) entry.Value { return entry.Value{Raw: []byte(raw), Kind: entry.KindString} }
	num := func(raw string) entry.Value { return entry.Value{Raw: []byte(raw), Kind: entry.KindNumber} }

	tests := []struct {
		name  string
		value entry.Value
		want  string // RFC 3339 in UTC
	}{
		{"RFC 3339 in UTC", str("2026-10-11T12:00:00Z"), "2026-10-11T12:00:00Z"},
		{"offset east", str("2026-10-11T12:00:00+03:00"), "2026-10-11T09:00:00Z"},
		{"offset west", str("2026-10-11T12:00:00-07:00"), "2026-10-11T19:00:00Z"},
		{"offset with minutes and no colon", str("2026-10-11T12:00:00+0530"), "2026-10-11T06:30:00Z"},
		{"offset with minutes west", str("2026-10-11T12:00:00-03:30"), "2026-10-11T15:30:00Z"},
		{"offset in hours", str("2026-10-11T12:00:00-08"), "2026-10-11T20:00:00Z"},
		{"an offset as loose as time.Parse allows", str("2026-10-11T12:00:00+24:60"), "2026-10-10T11:00:00Z"},
		{"one digit of fraction", str("2026-10-11T12:00:00.5Z"), "2026-10-11T12:00:00.5Z"},
		{"nanoseconds", str("2026-10-11T12:00:00.123456789Z"), "2026-10-11T12:00:00.123456789Z"},
		{"digits past nanoseconds are dropped", str("2026-10-11T12:00:00.1234567899Z"), "2026-10-11T12:00:00.123456789Z"},
		{"fraction with an offset", str("2026-10-11T12:00:00.25+03:00"), "2026-10-11T09:00:00.25Z"},
		{"comma before the fraction", str("2026-10-11T12:00:00,123Z"), "2026-10-11T12:00:00.123Z"},
		{"space between date and time", str("2026-10-11 12:00:00Z"), "2026-10-11T12:00:00Z"},
		{"no zone", str("2026-10-11T12:00:00"), "2026-10-11T09:00:00Z"},
		{"space and no zone", str("2026-10-11 12:00:00"), "2026-10-11T09:00:00Z"},
		{"space before the offset", str("2026-10-11 12:00:00 +0000"), "2026-10-11T12:00:00Z"},
		{"space before an offset east with a colon", str("2026-10-11 12:00:00 +03:00"), "2026-10-11T09:00:00Z"},
		{"space before an offset west in hours", str("2026-10-11T12:00:00 -08"), "2026-10-11T20:00:00Z"},
		{"fraction and a space before the offset", str("2026-10-11 12:00:00.5 +0530"), "2026-10-11T06:30:00.5Z"},
		{"space, comma and no zone", str("2026-10-11 12:00:00,123"), "2026-10-11T09:00:00.123Z"},
		{"leap day", str("2028-02-29T00:00:00Z"), "2028-02-29T00:00:00Z"},
		{"last day of a 30-day month", str("2026-04-30T00:00:00Z"), "2026-04-30T00:00:00Z"},
		{"last day of a 31-day month", str("2026-12-31T00:00:00Z"), "2026-12-31T00:00:00Z"},
		{"last day of February", str("2026-02-28T00:00:00Z"), "2026-02-28T00:00:00Z"},
		{"first second of the window", str("2000-01-01T00:00:00Z"), "2000-01-01T00:00:00Z"},
		{"last second of the day and of the window", str("2099-12-31T23:59:59.999999999Z"), "2099-12-31T23:59:59.999999999Z"},

		{"seconds", num("1791720000"), "2026-10-11T12:00:00Z"},
		{"milliseconds", num("1791720000123"), "2026-10-11T12:00:00.123Z"},
		{"microseconds", num("1791720000123456"), "2026-10-11T12:00:00.123456Z"},
		{"nanoseconds as an integer", num("1791720000123456789"), "2026-10-11T12:00:00.123456789Z"},
		{"fractional seconds", num("1791720000.1234567"), "2026-10-11T12:00:00.1234567Z"},
		{"fractional seconds past nanoseconds", num("1791720000.1234567899"), "2026-10-11T12:00:00.123456789Z"},
		{"fractional milliseconds", num("1791720000123.5"), "2026-10-11T12:00:00.1235Z"},
		{"fractional microseconds", num("1791720000123456.25"), "2026-10-11T12:00:00.12345625Z"},
		{"fractional nanoseconds", num("1791720000123456789.5"), "2026-10-11T12:00:00.123456789Z"},
		{"seconds in a string", str("1791720000"), "2026-10-11T12:00:00Z"},
		{"leading zeros", num("001791720000"), "2026-10-11T12:00:00Z"},
		{"first second of the window in seconds", num("946684800"), "2000-01-01T00:00:00Z"},
		{"last second of the window in seconds", num("4102444799"), "2099-12-31T23:59:59Z"},
		{"first millisecond of the window", num("946684800000"), "2000-01-01T00:00:00Z"},
		{"last millisecond of the window", num("4102444799999"), "2099-12-31T23:59:59.999Z"},
		{"first microsecond of the window", num("946684800000000"), "2000-01-01T00:00:00Z"},
		{"last microsecond of the window", num("4102444799999999"), "2099-12-31T23:59:59.999999Z"},
		{"first nanosecond of the window", num("946684800000000000"), "2000-01-01T00:00:00Z"},
		{"last nanosecond of the window", num("4102444799999999999"), "2099-12-31T23:59:59.999999999Z"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.value.Raw = slices.Clip(tt.value.Raw)
			got, ok := parseTime(tt.value, testLoc)
			if !ok {
				t.Fatalf("parseTime(%q) did not read the time", tt.value.Raw)
			}
			if got := got.UTC().Format(time.RFC3339Nano); got != tt.want {
				t.Errorf("parseTime(%q) = %s, want %s", tt.value.Raw, got, tt.want)
			}
		})
	}
}

func TestParseTimeRejects(t *testing.T) {
	tests := []struct {
		name string
		raw  string
	}{
		{"empty", ""},
		{"word", "yesterday"},
		{"date only", "2026-10-11"},
		{"time only", "12:00:00"},
		{"no seconds", "2026-10-11T12:00"},
		{"one character short", "2026-10-11T12:00:0"},
		{"one digit of hour", "2026-10-11 9:00:00"},
		{"one digit of hour and a zone", "2026-10-11T9:00:00Z"},
		{"two spaces between date and time", "2026-10-11  12:00:00"},
		{"two spaces and one digit of hour", "2026-10-11  9:00:00"},
		{"lower-case t", "2026-10-11t12:00:00Z"},
		{"two spaces before the offset", "2026-10-11 12:00:00  +0000"},
		{"a space before Z", "2026-10-11T12:00:00 Z"},
		{"a space and nothing after it", "2026-10-11T12:00:00 "},
		{"a zone name after the offset", "2026-10-11 12:00:00 +0000 UTC"},
		{"lower-case z", "2026-10-11T12:00:00z"},
		{"slash after the year", "2026/10-11T12:00:00Z"},
		{"slash after the month", "2026-10/11T12:00:00Z"},
		{"underscore between date and time", "2026-10-11_12:00:00Z"},
		{"dash after the hour", "2026-10-11T12-00:00Z"},
		{"dot after the minute", "2026-10-11T12:00.00Z"},
		{"letter in the year", "20x6-10-11T12:00:00Z"},
		{"sign in the year", "+026-10-11T12:00:00Z"},
		{"letter in the month", "2026-1x-11T12:00:00Z"},
		{"letter in the day", "2026-10-x1T12:00:00Z"},
		{"letter in the hour", "2026-10-11T1x:00:00Z"},
		{"letter in the minute", "2026-10-11T12:x0:00Z"},
		{"letter in the second", "2026-10-11T12:00:0xZ"},
		{"month 0", "2026-00-11T12:00:00Z"},
		{"month 13", "2026-13-11T12:00:00Z"},
		{"day 0", "2026-10-00T12:00:00Z"},
		{"day 32", "2026-10-32T12:00:00Z"},
		{"April 31", "2026-04-31T12:00:00Z"},
		{"June 31", "2026-06-31T12:00:00Z"},
		{"September 31", "2026-09-31T12:00:00Z"},
		{"November 31", "2026-11-31T12:00:00Z"},
		{"February 29 of a common year", "2026-02-29T12:00:00Z"},
		{"February 30 of a leap year", "2028-02-30T12:00:00Z"},
		{"hour 24", "2026-10-11T24:00:00Z"},
		{"minute 60", "2026-10-11T12:60:00Z"},
		{"second 60", "2026-10-11T12:00:60Z"},
		{"the year before the window", "1999-12-31T23:59:59Z"},
		{"the year after the window", "2100-01-01T00:00:00Z"},
		{"dot without digits", "2026-10-11T12:00:00.Z"},
		{"dot at the end", "2026-10-11T12:00:00."},
		{"letter in the fraction", "2026-10-11T12:00:00.12xZ"},
		{"semicolon before the fraction", "2026-10-11T12:00:00;123Z"},
		{"the character before 0 in the fraction", "2026-10-11T12:00:00./Z"},
		{"the character after 9 in the fraction", "2026-10-11T12:00:00.:Z"},
		{"text after the zone", "2026-10-11T12:00:00Zx"},
		{"space after the zone", "2026-10-11T12:00:00Z "},
		{"zone name", "2026-10-11T12:00:00 UTC"},
		{"sign alone", "2026-10-11T12:00:00+"},
		{"one digit of offset", "2026-10-11T12:00:00+3"},
		{"three digits of offset", "2026-10-11T12:00:00+030"},
		{"one digit of minutes", "2026-10-11T12:00:00+03:0"},
		{"five digits of offset", "2026-10-11T12:00:00+03000"},
		{"three digits of minutes", "2026-10-11T12:00:00+03:000"},
		{"letter in the offset hours", "2026-10-11T12:00:00+0x:00"},
		{"letter in the offset minutes", "2026-10-11T12:00:00+03:x0"},
		{"letter in an offset without a colon", "2026-10-11T12:00:00+03x0"},
		{"letter in an offset in hours", "2026-10-11T12:00:00+x3"},
		{"offset of 25 hours", "2026-10-11T12:00:00+25:00"},
		{"offset of 61 minutes", "2026-10-11T12:00:00+00:61"},
		{"asterisk for the sign", "2026-10-11T12:00:00*03:00"},
		{"escaped zone", `2026-10-11T12:00:00\u005a`},

		{"small number", "5"},
		{"zero", "0"},
		{"the second before the window", "946684799"},
		{"the second after the window", "4102444800"},
		{"between seconds and milliseconds", "17917200001"},
		{"the millisecond before the window", "946684799999"},
		{"the millisecond after the window", "4102444800000"},
		{"between milliseconds and microseconds", "17917200001234"},
		{"the microsecond before the window", "946684799999999"},
		{"the microsecond after the window", "4102444800000000"},
		{"between microseconds and nanoseconds", "17917200001234567"},
		{"the nanosecond before the window", "946684799999999999"},
		{"the nanosecond after the window", "4102444800000000000"},
		{"twenty digits", "17917200001234567890"},
		{"twenty digits that overflow into the window", "18446744075501271616"},
		{"minus", "-1791720000"},
		{"plus", "+1791720000"},
		{"exponent", "1.79172e9"},
		{"dot at the end of a number", "1791720000."},
		{"no integer part", ".5"},
		{"letter in the fraction of a number", "1791720000.5x"},
		{"comma in a number", "1791720000,5"},
		{"space after a number", "1791720000 "},
		{"two dots", "1791720000.5.5"},
	}

	for _, tt := range tests {
		for _, kind := range []entry.Kind{entry.KindString, entry.KindNumber} {
			t.Run(tt.name, func(t *testing.T) {
				value := entry.Value{Raw: slices.Clip([]byte(tt.raw)), Kind: kind}
				if got, ok := parseTime(value, testLoc); ok {
					t.Errorf("parseTime(%q, kind %d) = %s, want it rejected", tt.raw, kind, got.UTC().Format(time.RFC3339Nano))
				}
			})
		}
	}
}

// Only a string holds a date, and only a string or a number holds a Unix time.
func TestParseTimeKinds(t *testing.T) {
	for _, kind := range []entry.Kind{entry.KindNone, entry.KindBool, entry.KindNull, entry.KindObject, entry.KindArray} {
		for _, raw := range []string{"1791720000", "2026-10-11T12:00:00Z"} {
			if got, ok := parseTime(entry.Value{Raw: []byte(raw), Kind: kind}, testLoc); ok {
				t.Errorf("parseTime(%q, kind %d) = %s, want it rejected", raw, kind, got)
			}
		}
	}
	if got, ok := parseTime(entry.Value{Raw: []byte("2026-10-11T12:00:00Z"), Kind: entry.KindNumber}, testLoc); ok {
		t.Errorf("parseTime read the date %s out of a number", got)
	}
}

// A time without a zone is in the zone it was read in; any other is an instant and comes back in UTC.
func TestParseTimeLocation(t *testing.T) {
	tests := []struct {
		value entry.Value
		want  *time.Location
	}{
		{entry.Value{Raw: []byte("2026-10-11 12:00:00"), Kind: entry.KindString}, testLoc},
		{entry.Value{Raw: []byte("2026-10-11T12:00:00Z"), Kind: entry.KindString}, time.UTC},
		{entry.Value{Raw: []byte("2026-10-11T12:00:00+03:00"), Kind: entry.KindString}, time.UTC},
		{entry.Value{Raw: []byte("1791720000"), Kind: entry.KindNumber}, time.UTC},
	}

	for _, tt := range tests {
		if got, ok := parseTime(tt.value, testLoc); !ok || got.Location() != tt.want {
			t.Errorf("parseTime(%q) = %v, %v; want a time in %v", tt.value.Raw, got, ok, tt.want)
		}
	}
}

func TestParseTimeWithoutLocationIsLocal(t *testing.T) {
	got, ok := parseTime(entry.Value{Raw: []byte("2026-10-11 12:00:00"), Kind: entry.KindString}, nil)

	if want := time.Date(2026, 10, 11, 12, 0, 0, 0, time.Local); !ok || !got.Equal(want) {
		t.Errorf("parseTime = %v, %v; want %v", got, ok, want)
	}
}

func TestParseLevelValue(t *testing.T) {
	tests := []struct {
		raw  string
		kind entry.Kind
		want entry.Level
	}{
		{"info", entry.KindString, entry.LevelInfo},
		{"WARN", entry.KindString, entry.LevelWarn},
		{" error ", entry.KindString, entry.LevelError},
		{"INFO+2", entry.KindString, entry.LevelInfo},
		{"10", entry.KindNumber, entry.LevelTrace},
		{"20", entry.KindNumber, entry.LevelDebug},
		{"30", entry.KindNumber, entry.LevelInfo},
		{"40", entry.KindNumber, entry.LevelWarn},
		{"50", entry.KindNumber, entry.LevelError},
		{"60", entry.KindNumber, entry.LevelFatal},
		{"30", entry.KindString, entry.LevelInfo},

		{"", entry.KindString, entry.LevelUnknown},
		{"notaword", entry.KindString, entry.LevelUnknown},
		{"0", entry.KindNumber, entry.LevelUnknown},
		{"00", entry.KindNumber, entry.LevelUnknown},
		{"3", entry.KindNumber, entry.LevelUnknown},
		{"35", entry.KindNumber, entry.LevelUnknown},
		{"31", entry.KindNumber, entry.LevelUnknown},
		{"39", entry.KindNumber, entry.LevelUnknown},
		{"70", entry.KindNumber, entry.LevelUnknown},
		{"300", entry.KindNumber, entry.LevelUnknown},
		{"30.0", entry.KindNumber, entry.LevelUnknown},
		{"a0", entry.KindString, entry.LevelUnknown},
		{"/0", entry.KindString, entry.LevelUnknown},
		{"info", entry.KindNone, entry.LevelUnknown},
		{"true", entry.KindBool, entry.LevelUnknown},
		{"null", entry.KindNull, entry.LevelUnknown},
		{`{"a":1}`, entry.KindObject, entry.LevelUnknown},
		{`["info"]`, entry.KindArray, entry.LevelUnknown},
		{"30", entry.KindBool, entry.LevelUnknown},
	}

	for _, tt := range tests {
		value := entry.Value{Raw: slices.Clip([]byte(tt.raw)), Kind: tt.kind}
		if got := parseLevel(value); got != tt.want {
			t.Errorf("parseLevel(%q, kind %d) = %v, want %v", tt.raw, tt.kind, got, tt.want)
		}
	}
}

func TestAssignRolesDoesNotAllocate(t *testing.T) {
	lines := []string{
		`{"time":"2026-10-11T12:00:00.123456789Z","level":"INFO","msg":"hello","count":3}`,
		`{"time":"2026-10-11T12:00:00+05:30","level":"warning","msg":"hello"}`,
		`{"timestamp":"2026-10-11 12:00:00,123","severity":"ERROR","message":"hello"}`,
		`{"level":"info","ts":1791720000.1234567,"msg":"hello"}`,
		`time=2026-10-11T12:00:00Z level=INFO+2 msg="hello" count=3`,
		`{"time":"yesterday","level":"notaword","msg":42,"a":1}`,
		`{"@t":"2026-10-11T12:00:00.1234567Z","@mt":"User {Name} logged in","Name":"bob"}`,
		`@t=2026-10-11T12:00:00Z @mt=hello Name=bob`,
	}

	for _, line := range lines {
		e := entryOf(t, line)
		if allocs := testing.AllocsPerRun(100, func() { assignRoles(e, testLoc) }); allocs != 0 {
			t.Errorf("assignRoles allocates %v times for %q, want 0", allocs, line)
		}
	}
}

func TestTimeOfAText(t *testing.T) {
	want := time.Date(2026, 10, 11, 12, 0, 0, 0, time.UTC)
	zone := time.FixedZone("test", 3*60*60)

	for text, loc := range map[string]*time.Location{
		"2026-10-11T12:00:00Z":      nil,
		"2026-10-11T15:00:00+03:00": time.UTC,
		"2026-10-11 12:00:00":       time.UTC,
		"2026-10-11 15:00:00":       zone,
		"1791720000":                nil,
		"1791720000000":             zone,
	} {
		if got, ok := Time([]byte(text), loc); !ok || !got.Equal(want) {
			t.Errorf("Time(%q, %v) = %v, %v; want %v", text, loc, got, ok, want)
		}
	}
	for _, text := range []string{"", "yesterday", "12:00", "2026-10-11", "42"} {
		if got, ok := Time([]byte(text), time.UTC); ok || !got.IsZero() {
			t.Errorf("Time(%q) = %v, %v; want no time", text, got, ok)
		}
	}
}
