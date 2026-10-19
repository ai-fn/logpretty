package filter

import (
	"bytes"
	"cmp"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/ai-fn/logpretty/internal/entry"
	"github.com/ai-fn/logpretty/internal/parse"
)

// The grammar of an expression written once more, as a regular expression: a key, then maybe an operator
// and a value; a token is quoted or bare; a bare value may hold an operator or a bracket past its first byte.
var (
	exprSyntax = regexp.MustCompile(`(?s)^[ \t\r\n]*(?:"((?:[^"\\]|\\.)*)"|([^ \t\r\n"=!<>~()']+))[ \t\r\n]*` +
		`(?:(!=|!~|>=|<=|=|>|<|~)[ \t\r\n]*(?:"((?:[^"\\]|\\.)*)"|([^ \t\r\n"=!<>~()'][^ \t\r\n"]*))[ \t\r\n]*)?$`)
	exprEscape = regexp.MustCompile(`\\(["\\])`)

	refOps = map[string]op{"": opExists, "=": opEq, "!=": opNe, ">": opGt, ">=": opGe, "<": opLt, "<=": opLe, "~": opMatch, "!~": opNoMatch}
)

// referenceExpr is what parseExpr must agree with, apart from the positions.
func referenceExpr(s string) (e expr, ok bool) {
	m := exprSyntax.FindStringSubmatchIndex(s)
	if m == nil {
		return expr{}, false
	}
	group := func(n int) (string, bool) {
		if m[2*n] < 0 {
			return "", false
		}
		return s[m[2*n]:m[2*n+1]], true
	}
	if key, quoted := group(1); quoted {
		e.key, e.literal = exprEscape.ReplaceAllString(key, "$1"), true
	} else {
		e.key, _ = group(2)
	}
	operator, _ := group(3)
	e.op = refOps[operator]
	if value, quoted := group(4); quoted {
		e.value, e.quotedValue = exprEscape.ReplaceAllString(value, "$1"), true
	} else {
		e.value, _ = group(5)
	}
	return e, true
}

func FuzzParseExpr(f *testing.F) {
	for _, seed := range []string{
		"user_id=42", "status!=200", "ms>=100", "ms<5", "msg~timeout", "msg!~^ok$", "http.status", ` a = "b c" `,
		`"my key"`, `"a=b"!="c \" d"`, `p="C:\\dir\\"`, `m~"\d+\s"`, "url~^https://x/?a=1&b>2", "a=b=", `""=""`,
		"", " ", "=5", "user_id=>42", "a==1", "a=~x", "a!", "a=", `a="x`, `a="x\"`, `a="x"y`, "a=b c", "a b", `a=b"c`, `"a"b`,
		"a=\xff", "\xff=\xfe", "(a=1)", "msg~^(a|b)$", "msg='x'", "a=it's", "a=x)", "a=)", "a(b=1", "a=1 (b", "ключ=значение", `a="\`, `a="\\`, "a<>1", "a=1 b=2", "a\n=\r1",
	} {
		f.Add(seed)
	}

	f.Fuzz(func(t *testing.T, s string) {
		want, wantOK := referenceExpr(s)

		got, err := parseExpr(s)

		if (err == nil) != wantOK {
			t.Fatalf("parseExpr(%q) error = %v, the reference accepts it: %v", s, err, wantOK)
		}
		if err != nil {
			var syntax *SyntaxError
			if !errors.As(err, &syntax) || syntax.Pos < 0 || syntax.Pos > len(s) || syntax.Msg == "" || got != (expr{}) {
				t.Fatalf("parseExpr(%q) = %+v, %#v; want the zero value and a SyntaxError inside the expression", s, got, err)
			}
			return
		}
		if got.key != want.key || got.literal != want.literal || got.op != want.op || got.value != want.value || got.quotedValue != want.quotedValue {
			t.Fatalf("parseExpr(%q)\n got %+v\nwant %+v", s, got, want)
		}
		if got.op == opExists {
			if got.valuePos != 0 {
				t.Fatalf("parseExpr(%q): a key alone has a value at %d", s, got.valuePos)
			}
			return
		}
		// The value starts where the position says: at its quote, or at its own first byte.
		if rest := s[got.valuePos:]; rest[0] != '"' && rest[:len(got.value)] != got.value {
			t.Fatalf("parseExpr(%q): the value %q is not at %d", s, got.value, got.valuePos)
		}
	})
}

var numberSyntax = regexp.MustCompile(`^-?[0-9]+(\.[0-9]+)?([eE][+-]?[0-9]+)?\z`)

func FuzzNumber(f *testing.F) {
	for _, seed := range [][2]string{
		{"42", "42"},
		{"0042", "42"},
		{"-0", "0"},
		{"-10", "-9"},
		{"9007199254740993", "9007199254740992"},
		{"18446744073709551615", "18446744073709551616"},
		{"-9223372036854775809", "-9223372036854775808"},
		{"42", "42.0"},
		{"1e2", "100"},
		{"-2", "-1.5"},
		{"1e400", "1e308"},
		{"1e-400", "0"},
		{"-0.0", "0"},
		{"", "1"},
		{"+1", "1."},
		{".5", "0x10"},
		{"1_000", "NaN"},
		{"1e", "1e+"},
		{" 1", "1 "},
		{"--1", "1-"},
	} {
		f.Add(seed[0], seed[1])
	}

	f.Fuzz(func(t *testing.T, x, y string) {
		a, okA := parseNumber([]byte(x))
		b, okB := parseNumber([]byte(y))
		if okA != numberSyntax.MatchString(x) || okB != numberSyntax.MatchString(y) {
			t.Fatalf("parseNumber(%q), parseNumber(%q) = %v, %v; the reference says %v, %v", x, y, okA, okB, numberSyntax.MatchString(x), numberSyntax.MatchString(y))
		}
		if !okA || !okB {
			return
		}

		got := a.compare(b)
		if back := b.compare(a); back != -got || a.compare(a) != 0 || b.compare(b) != 0 {
			t.Fatalf("%s compare %s = %d, the other way %d, and with themselves %d and %d", x, y, got, back, a.compare(a), b.compare(b))
		}

		// Integers are checked against exact arithmetic, the rest against float64 itself.
		var want int
		bigA, intA := new(big.Int).SetString(x, 10)
		bigB, intB := new(big.Int).SetString(y, 10)
		if intA && intB {
			want = bigA.Cmp(bigB)
		} else {
			// An error here is a number out of range, which comes back as an infinity or zero.
			floatA, _ := strconv.ParseFloat(x, 64)
			floatB, _ := strconv.ParseFloat(y, 64)
			want = cmp.Compare(floatA, floatB)
		}
		if got != want {
			t.Fatalf("%s compare %s = %d, want %d", x, y, got, want)
		}
	})
}

// quoteToken writes a text as a quoted token of an expression.
func quoteToken(s string) string {
	return `"` + strings.NewReplacer(`\`, `\\`, `"`, `\"`).Replace(s) + `"`
}

func FuzzKeep(f *testing.F) {
	lines := []string{
		`{"time":"2026-10-11T12:00:00Z","level":"warn","msg":"slow request","user_id":42,"http":{"status":502},"note":"a \"b\"\n"}`,
		`time=2026-10-11T12:00:00Z level=info msg="request done" status=200 word`,
		`{"level":"bogus","time":"yesterday","msg":42,"big":9007199254740993,"ok":true,"err":null,"code":"042"}`,
		`web-1 | {"severity":"ERROR","message":"failed"}`, `{}`, `plain text`, ``,
		"msg=x a=\xff",
		`{"msg":"x","trace":"stack","traceId":"4bf92f35","trace_id":null}`,
		`{"dd.trace_id":1234567890123456789,"logging.googleapis.com/trace":"projects/p/traces/abc"}`,
		`{"trace":{"id":"a \"b\""}}`, `level=info msg=x traceID=4bf92f35`, `{"trace_id":"4BF92F35","msg":"x"}`,
		`{"Scopes":[{"TraceId":"4bf92f35"}]}`, `{"logging.googleapis.com/trace":"projects/p/traces/abc"}`, `trace=stack msg=x`,
	}
	for _, seed := range [][2]string{
		{"user_id", "42"},
		{"user_id", "42.0"},
		{"http.status", "502"},
		{"msg", "slow request"},
		{"msg", "^slow"},
		{"msg", "42"},
		{"level", "warn"},
		{"level", "bogus"},
		{"time", "2026-10-11T12:00:00Z"},
		{"time", "yesterday"},
		{"note", "a \"b\"\n"},
		{"big", "9007199254740992"},
		{"code", "42"},
		{"code", "042"},
		{"ok", "true"},
		{"err", "null"},
		{"nope", ""},
		{"", ""},
		{"status", "2e2"},
		{"msg", "("},
		{"a", "\ufffd"},
		{"a", "\xff"},
		{"trace", "4bf92f35"},
		{"trace", "^4bf9"},
		{"trace", "1234567890123456789.0"},
		{"trace", "abc"},
		{"trace", "stack"},
	} {
		for _, line := range lines {
			f.Add([]byte(line), seed[0], seed[1])
		}
	}

	f.Fuzz(func(t *testing.T, line []byte, key, value string) {
		// Clipped, so that a read past the end panics instead of finding spare capacity.
		line = slices.Clip(slices.Clone(line))
		p := parse.Parser{Loc: time.UTC}
		parsed := p.Parse(line)
		if parsed.Format == entry.FormatPlain {
			return // a plain line is not asked about its fields
		}
		// Taken before keep parses the line again and the entry goes stale.
		refLine, refKey, refOK := traceReference(line, parsed)

		// keepOn compiles the expression and asks about the line; ok is false if it does not compile.
		keepOn := func(on []byte, expression string) (kept, ok bool) {
			c, err := Compile(expression, time.UTC)
			if err != nil {
				var syntax *SyntaxError
				if !errors.As(err, &syntax) || syntax.Pos < 0 || syntax.Pos > len(expression) {
					t.Fatalf("Compile(%q) error = %#v, want a SyntaxError inside the expression", expression, err)
				}
				return false, false
			}
			fresh := Filter{Where: []Cond{c}}
			kept = fresh.Keep(p.Parse(on))

			// A Filter that has seen other entries must decide the same.
			used := Filter{Where: []Cond{c}}
			for _, other := range []string{`{"a":"x \" y","b":{"c":{"d":1}}}`, `a=1 b="x \" y"`, `text`} {
				used.Keep(p.Parse([]byte(other)))
			}
			if got := used.Keep(p.Parse(on)); got != kept {
				t.Fatalf("%s on %q: %v with a new Filter, %v with a used one", expression, on, kept, got)
			}
			return kept, true
		}
		keep := func(expression string) (kept, ok bool) { return keepOn(line, expression) }

		if refOK {
			rests := []string{""}
			for _, v := range []string{quoteToken(value), value} {
				for _, operator := range []string{"=", "!=", ">", ">=", "<", "<=", "~", "!~"} {
					rests = append(rests, operator+v)
				}
			}
			for _, rest := range rests {
				got, okGot := keep("trace" + rest)
				want, okWant := keepOn(refLine, refKey+rest)
				if okGot != okWant || got != want {
					t.Fatalf("%q: trace%s gives %v (compiles %v); %s%s on %q gives %v (compiles %v)", line, rest, got, okGot, refKey, rest, refLine, want, okWant)
				}
			}
		}

		// --trace ID is --where 'trace="ID"'.
		want, ok := keep("trace=" + quoteToken(value))
		traced := Filter{Traces: [][]byte{[]byte(value)}}
		if got := traced.Keep(p.Parse(line)); !ok || got != want {
			t.Fatalf("%q: Traces %q gives %v, trace=%s gives %v (compiles %v)", line, value, got, quoteToken(value), want, ok)
		}

		// A key in quotes is a field of that name; a bare one may be a role. Both are tried.
		for _, k := range []string{quoteToken(key), key} {
			// A text that is not a bare key by itself would run into the operator: "a!" and "=" make "a" and "!=".
			if bare, err := parseExpr(k); err != nil || bare.op != opExists || bare.key != key {
				continue
			}
			// The level and the time are compared as what they are, in quotes or not.
			typed := k == key && (roleOf(key) == roleLevel || roleOf(key) == roleTime)
			// The value is tried in quotes and, if it is a bare token by itself, without them.
			results := map[string]map[string]bool{}
			for _, v := range []string{quoteToken(value), value} {
				if e, err := parseExpr(k + "=" + v); err != nil || e.value != value {
					continue
				}
				results[v] = map[string]bool{}
				for _, operator := range []string{"=", "!=", ">", ">=", "<", "<=", "~", "!~"} {
					if kept, ok := keep(k + operator + v); ok {
						results[v][operator] = kept
					}
				}

				check := func(a, b string, rule func(a, b bool) bool, what string) {
					x, okX := results[v][a]
					y, okY := results[v][b]
					if okX != okY {
						t.Fatalf("%s%s%s compiles and %s%s%s does not, or the other way", k, a, v, k, b, v)
					}
					if okX && !rule(x, y) {
						t.Fatalf("%q, key %s, value %s: %s gives %v and %s gives %v, want them %s", line, k, v, a, x, b, y, what)
					}
				}
				opposite := func(a, b bool) bool { return a != b }
				notBoth := func(a, b bool) bool { return !a || !b }
				implies := func(a, b bool) bool { return !a || b }
				check("=", "!=", opposite, "opposite")
				check("~", "!~", opposite, "opposite")
				check(">", "<=", notBoth, "not both true")
				check(">=", "<", notBoth, "not both true")
				check(">", "<", notBoth, "not both true")
				// What is equal is neither above nor below, as text or as a number.
				if _, ordered := results[v][">="]; ordered {
					check("=", ">=", implies, "the second with the first")
					check("=", "<=", implies, "the second with the first")
				}
				// Equal by an order means equal, unless the quotes made the value text.
				if r := results[v]; (v == value || typed) && r[">="] && r["<="] && !r["="] {
					t.Fatalf("%q, key %s, value %s: >= and <= hold and = does not", line, k, v)
				}
			}

			quoted, bare := results[quoteToken(value)], results[value]
			if quoted == nil {
				t.Fatalf("key %s: the value %q does not come back out of its quotes", k, value)
			}
			if bare != nil {
				// Quotes change = and != alone, and only by taking away what is equal as a number.
				if quoted["="] && !bare["="] {
					t.Fatalf("%q, key %s, value %q: equal in quotes and not without them", line, k, value)
				}
				unchanged := []string{">", ">=", "<", "<=", "~", "!~"}
				if typed {
					unchanged = append(unchanged, "=", "!=")
				}
				for _, operator := range unchanged {
					if quoted[operator] != bare[operator] {
						t.Fatalf("%q, key %s, value %q: %s gives %v in quotes and %v without", line, k, value, operator, quoted[operator], bare[operator])
					}
				}
			}

			// In quotes, equal is the very same text: a regular expression for that text and nothing else agrees.
			// It reads a byte that is not UTF-8 as U+FFFD, so a value with that rune would match other text.
			if !typed && !strings.ContainsRune(value, utf8.RuneError) {
				if same, ok := keep(k + "~" + quoteToken("^"+regexp.QuoteMeta(value)+"$")); ok && same != quoted["="] {
					t.Fatalf("%q, key %s, value %q: equal in quotes is %v, the same text is %v", line, k, value, quoted["="], same)
				}
			}
		}
	})
}

// traceReference returns a line and a key in quotes that must decide as the key trace on the entry: the field
// trace when there is no id, the field of the id, or else a line of the id alone. An id that is not UTF-8 has no reference.
func traceReference(line []byte, e *entry.Entry) (refLine []byte, refKey string, ok bool) {
	var finder parse.Finder
	id, field, found := finder.Trace(e)
	if !found {
		return line, quoteToken("trace"), true
	}
	// Elsewhere the quoted key misses the id: it is nested, cut out of a Google Cloud path, or behind an earlier field.
	if field >= 0 && !strings.EqualFold(string(e.Fields[field].Key), "logging.googleapis.com/trace") {
		key := e.Fields[field].Key
		if !slices.ContainsFunc(e.Fields[:field], func(fl entry.Field) bool { return fl.Value.Kind != entry.KindNone && bytes.Equal(fl.Key, key) }) {
			return line, quoteToken(string(key)), true
		}
	}
	if !utf8.Valid(id.Raw) {
		return nil, "", false // encoding/json would write U+FFFD in its place
	}
	text, _ := json.Marshal(string(id.Raw)) // a string always marshals
	return slices.Clip(append(append([]byte(`{"x":`), text...), '}')), `"x"`, true
}

// FuzzKeepSources plays a stream of entries and plain lines of a few sources against a model of the rule:
// a plain line goes the way of the last entry of its source, or of the stream when the source has none.
func FuzzKeepSources(f *testing.F) {
	f.Add(byte(0), []byte{0x10, 0x31, 0x02, 0x13, 0x03, 0x40})
	f.Add(byte(0), []byte{0x02, 0x10, 0x12, 0x21, 0x02, 0x33, 0x42})
	f.Add(byte(0), []byte{0x04, 0x08, 0x0c, 0x10, 0x15, 0x12, 0x06})
	f.Add(byte(1), []byte{0x10, 0x31, 0x02, 0x13})
	f.Add(byte(2), []byte{0x10, 0x31, 0x02, 0x13})
	f.Fuzz(func(t *testing.T, mode byte, events []byte) {
		// A limit of three sources, so that a short stream runs into it.
		const limit = 3
		p := parse.Parser{Loc: time.UTC}
		filter := Filter{MinLevel: entry.LevelError, Plain: PlainMode(mode % 3), limit: limit}
		lastKept, bySource := false, map[string]bool{}
		for i, ev := range events {
			// The low two bits: an error, an info entry, a plain line or a cut one; the rest: the source, 0 for none.
			kind, src := ev&3, ev>>2
			name := ""
			if src > 0 {
				name = fmt.Sprintf("s%d", src)
			}
			line := map[byte]string{0: `{"level":"error","msg":"x"}`, 1: `{"level":"info","msg":"x"}`, 2: "plain"}[kind]
			if name != "" {
				line = name + " | " + line
			}
			var got, want bool
			switch {
			case kind >= 2 && filter.Plain != PlainFollow:
				if kind == 3 {
					got = filter.KeepPlain(nil)
				} else {
					got = filter.Keep(p.Parse([]byte(line)))
				}
				want = filter.Plain == PlainShow
			case kind == 3:
				got, want = filter.KeepPlain(nil), lastKept
			case kind == 2:
				got = filter.Keep(p.Parse([]byte(line)))
				want = lastKept
				if v, ok := bySource[name]; ok && name != "" {
					want = v
				}
			default:
				got, want = filter.Keep(p.Parse([]byte(line))), kind == 0
				lastKept = want
				if _, ok := bySource[name]; name != "" && (ok || len(bySource) < limit) {
					bySource[name] = want
				}
			}
			if got != want {
				t.Fatalf("event %d (%q) of %x: kept %v, want %v", i, line, events, got, want)
			}
		}
	})
}
