package parse

import (
	"bytes"
	"encoding/json"
	"math/big"
	"regexp"
	"slices"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/ai-fn/logpretty/internal/entry"
)

// refField is a pair as encoding/json sees it.
type refField struct {
	key        string // decoded
	value      string // exactly as written
	keyEscaped bool
}

// referenceJSONFields is the obvious implementation appendJSONFields must agree with.
func referenceJSONFields(t *testing.T, line []byte) (fields []refField, ok bool) {
	t.Helper()
	if !json.Valid(line) {
		return nil, false
	}
	dec := json.NewDecoder(bytes.NewReader(line))
	if tok, err := dec.Token(); err != nil || tok != json.Delim('{') {
		return nil, false
	}
	for dec.More() {
		start := dec.InputOffset()
		key, err := dec.Token()
		if err != nil {
			t.Fatalf("reference: reading a key of %q: %v", line, err)
		}
		// Between two tokens only the key itself can hold a backslash.
		keyEscaped := bytes.IndexByte(line[start:dec.InputOffset()], '\\') >= 0
		var value json.RawMessage
		if err := dec.Decode(&value); err != nil {
			t.Fatalf("reference: reading a value of %q: %v", line, err)
		}
		fields = append(fields, refField{key: key.(string), value: string(value), keyEscaped: keyEscaped})
	}
	return fields, true
}

func FuzzJSON(f *testing.F) {
	for _, seed := range []string{
		`{}`, ` { } `, `{"msg":"hello"}`, `{"z":1,"a":2}`, `{"id":12345678901234567891,"e":1e400}`,
		`{"s":"x","n":-1.5e-3,"t":true,"f":false,"z":null,"o":{"a":[1,{"b":"}"}]},"a":[]}`,
		`{"a":"say \"hi\"\n","b":"\\","c":"\\\""}`, `{"a\"b":1,"caf\u00e9":2,"":3}`, `{"a":1,"a":2}`,
		"\t{ \"a\" :\t1 ,\r\n \"b\" : [ 1 , 2 ] }\r\n", `{"a":1} x`, `{"a":1}{"b":2}`, `[1]`, `{"a":1,}`, `{'a':1}`,
		"{\"a\":\"x\xffy\",\"\xfe\":1}", "\u00a0{\"a\":1}", `{"a":"\ud83d\ude00","\ud83d":"\ude00"}`, `{`, ``,
	} {
		f.Add([]byte(seed))
	}

	f.Fuzz(func(t *testing.T, line []byte) {
		// A copy, because the target overwrites the line below and must not touch the engine's data.
		// Clipped, so that a read past the end panics instead of finding spare capacity.
		line = slices.Clip(slices.Clone(line))
		want, wantOK := referenceJSONFields(t, line)

		fields, ok := appendJSONFields(nil, line)

		if ok != wantOK {
			t.Fatalf("appendJSONFields(%q) accepted = %v, encoding/json says %v", line, ok, wantOK)
		}
		if !ok {
			if len(fields) != 0 {
				t.Fatalf("appendJSONFields(%q) rejected the line but returned %d fields", line, len(fields))
			}
			return
		}
		if len(fields) != len(want) {
			t.Fatalf("appendJSONFields(%q) found %d fields, encoding/json finds %d", line, len(fields), len(want))
		}
		for i, field := range fields {
			key := string(field.Key)
			if !utf8.Valid(field.Key) {
				key = string([]rune(key)) // encoding/json replaces every invalid byte with U+FFFD
			}
			if key != want[i].key {
				t.Fatalf("appendJSONFields(%q): key %d = %q, encoding/json gives %q", line, i, key, want[i].key)
			}
			value := string(field.Value.Raw)
			if field.Value.Kind == entry.KindString {
				value = `"` + value + `"`
			}
			if value != want[i].value {
				t.Fatalf("appendJSONFields(%q): value %d = %q, encoding/json gives %q", line, i, value, want[i].value)
			}
			if escaped := bytes.IndexByte(field.Value.Raw, '\\') >= 0; field.Value.Kind == entry.KindString && field.Value.Escaped != escaped {
				t.Fatalf("appendJSONFields(%q): value %d has Escaped = %v, want %v", line, i, field.Value.Escaped, escaped)
			}
			if kind := kindOf(want[i].value); field.Value.Kind != kind {
				t.Fatalf("appendJSONFields(%q): value %d has kind %d, want %d", line, i, field.Value.Kind, kind)
			}
		}

		// Fields are windows into the line: flipping every byte of the line must show through them.
		before := pairsOf(fields)
		for i := range line {
			line[i] ^= 0xff
		}
		for i, field := range fields {
			// A key with escapes is the one thing that is decoded into memory of its own.
			if !want[i].keyEscaped && !isFlipped(field.Key, before[i].key) {
				t.Fatalf("key %d (%q) is not a clipped window into the line", i, before[i].key)
			}
			if !isFlipped(field.Value.Raw, before[i].raw) {
				t.Fatalf("value %d (%q) is not a clipped window into the line", i, before[i].raw)
			}
		}
	})
}

// isFlipped reports whether got is before with every byte inverted, and has no spare capacity.
func isFlipped(got []byte, before string) bool {
	if len(got) != len(before) || cap(got) != len(got) {
		return false
	}
	for i := range got {
		if got[i] != before[i]^0xff {
			return false
		}
	}
	return true
}

// kindOf names the kind of a JSON value by its first byte, as the reference for the fuzz target.
func kindOf(value string) entry.Kind {
	switch value[0] {
	case '"':
		return entry.KindString
	case '{':
		return entry.KindObject
	case '[':
		return entry.KindArray
	case 't', 'f':
		return entry.KindBool
	case 'n':
		return entry.KindNull
	default:
		return entry.KindNumber
	}
}

const (
	logfmtSpace = " \t\n\r"
	// A byte of invalid UTF-8 matches as U+FFFD, so the last range takes in every byte from 0x80 up.
	logfmtKeyPattern = `[A-Za-z_@\x{80}-\x{10FFFF}][-.0-9A-Za-z_@\x{80}-\x{10FFFF}]*`
)

// The rules of logfmt.go stated apart from it, one expression per kind of token.
var (
	logfmtKey        = regexp.MustCompile(`^` + logfmtKeyPattern + `$`)
	logfmtQuotedPair = regexp.MustCompile(`(?s)^(` + logfmtKeyPattern + `)="((?:[^"\\]|\\.)*)"`)
	logfmtBarePair   = regexp.MustCompile(`^(` + logfmtKeyPattern + `)=([^ \t\n\r]*)`)
	logfmtWord       = regexp.MustCompile(`^[^ \t\n\r]+`)
)

// refToken is a token as the reference sees it. One that is not a pair has only a key: its whole text.
type refToken struct {
	field      pair
	start, end int
}

// referenceLogfmtTokens is the obvious implementation nextLogfmtToken must agree with.
func referenceLogfmtTokens(line []byte) []refToken {
	var toks []refToken
	i := 0
	for {
		i = len(line) - len(bytes.TrimLeft(line[i:], logfmtSpace))
		if i == len(line) {
			return toks
		}
		rest := line[i:]
		n, field := len(logfmtWord.Find(rest)), pair{}
		switch quoted, bare := logfmtQuotedPair.FindSubmatch(rest), logfmtBarePair.FindSubmatch(rest); {
		case quoted != nil:
			n, field = len(quoted[0]), pair{string(quoted[1]), string(quoted[2]), entry.KindString, bytes.IndexByte(quoted[2], '\\') >= 0}
		case bare != nil && bytes.HasPrefix(bare[2], []byte{'"'}):
			n = len(bytes.TrimRight(rest, logfmtSpace)) // the quote is never closed: the rest of the line is one word
		case bare != nil:
			n, field = len(bare[0]), pair{string(bare[1]), string(bare[2]), entry.KindString, false}
		}
		if field.kind == entry.KindNone {
			field.key = string(rest[:n])
		}
		toks = append(toks, refToken{field, i, i + n})
		i += n
	}
}

// referenceLogfmtFields is the obvious implementation appendLogfmtFields must agree with.
func referenceLogfmtFields(line []byte) (fields []pair, prefix string, ok bool) {
	toks := referenceLogfmtTokens(line)
	first := slices.IndexFunc(toks, func(tok refToken) bool { return tok.field.kind == entry.KindString })
	if first < 0 {
		return nil, "", false
	}
	var pairs, pairBytes, otherBytes int
	for _, tok := range toks[first:] {
		if tok.field.kind == entry.KindString {
			pairs++
			pairBytes += tok.end - tok.start
		} else {
			otherBytes += tok.end - tok.start
		}
		fields = append(fields, tok.field)
	}
	if pairs < 2 || 10*pairBytes < 7*(pairBytes+otherBytes) {
		return nil, "", false
	}
	if first > 0 {
		prefix = string(line[toks[0].start:toks[first-1].end])
	}
	return fields, prefix, true
}

func FuzzLogfmt(f *testing.F) {
	for _, seed := range []string{
		``, " \t ", `k=v`, `k=`, `a= b=""`, `level=info msg="hello world" port=8080`, `k="a \"b\"" n=1`, `k="C:\\" n=1`,
		`path=C:\dir k=a"b c`, `k="a"b`, `k==v q=a=b`, `a.b-c@d=1 _x=1 @timestamp=2`, "ключ=значение a=1\tb=2\r",
		`http://x?a=1 =v 1a=2 -a=1 a:b=1 "k"=v`, `a=1 k="b c=2 d`, "k=\"b c \t", `k="abc\`, `a=1 bc=2 xyz`, `a=1 bc=2 wxyz`,
		`svc | level=info ready msg=x`, "2026/10/11 12:00:00 level=info msg=x", "a=\xff \xfe=1 b=\"\xff\\\xff\"",
		`k="a"b x=1`, "a=1\r\nb=2\r", "a=1\vb=2 c\fd", "a=1\u00a0b=2 \ufeffc=3", "p\x00=1 a=x\x00y b=2",
		`took 5ms (retries=3, status=ok, code=5)`, "a=1 b=2 " + strings.Repeat("w", 41), `a=1 k="` + strings.Repeat("w ", 30),
		// Read as key, value, key, value: what the round trip encodes.
		"level\x00info\x00msg\x00hello world\x00port\x008080", "k\x00a \"b\"\x00path\x00C:\\dir\x00ключ\x00значение\x00e\x00",
		"a\x00\"\x00b\x00\\\x00c\x00x=\"y\"\x00d\x00\xff\\n", "a\x001\x00b",
	} {
		f.Add([]byte(seed), uint8(0))
		f.Add([]byte(seed), uint8(0xa5))
	}

	f.Fuzz(func(t *testing.T, data []byte, quoted uint8) {
		checkLogfmtRoundTrip(t, data, quoted)
		// A copy, because the check overwrites the line and must not touch the engine's data.
		// Clipped, so that a read past the end panics instead of finding spare capacity.
		checkLogfmtAgainstReference(t, slices.Clip(slices.Clone(data)))
	})
}

// checkLogfmtAgainstReference compares the tokens and the fields of a line with the reference, then
// overwrites the line to see that the fields are windows into it.
func checkLogfmtAgainstReference(t *testing.T, line []byte) {
	t.Helper()
	wantToks := referenceLogfmtTokens(line)
	gotToks := make([]refToken, 0, len(wantToks))
	for tok, ok := nextLogfmtToken(line, 0); ok; tok, ok = nextLogfmtToken(line, tok.end) {
		if tok.end <= tok.start {
			t.Fatalf("%q: token [%d:%d) is empty, the next one would start at the same place", line, tok.start, tok.end)
		}
		field := pair{key: string(line[tok.start:tok.end])}
		if tok.pair {
			field = pair{string(line[tok.start:tok.keyEnd]), string(line[tok.valStart:tok.valEnd]), entry.KindString, tok.escaped}
		}
		gotToks = append(gotToks, refToken{field, tok.start, tok.end})
	}
	if !slices.Equal(gotToks, wantToks) {
		t.Fatalf("tokens of %q\n got %+v\nwant %+v", line, gotToks, wantToks)
	}

	want, wantPrefix, wantOK := referenceLogfmtFields(line)

	fields, prefix, ok := appendLogfmtFields(nil, line)

	if ok != wantOK {
		t.Fatalf("appendLogfmtFields(%q) accepted = %v, the reference says %v", line, ok, wantOK)
	}
	if !ok {
		if len(fields) != 0 || prefix != nil {
			t.Fatalf("appendLogfmtFields(%q) rejected the line but returned %d fields and the prefix %q", line, len(fields), prefix)
		}
		return
	}
	// No prefix is nil, not an empty window: callers may tell the two apart.
	if got := pairsOf(fields); string(prefix) != wantPrefix || (wantPrefix == "") != (prefix == nil) || !slices.Equal(got, want) {
		t.Fatalf("appendLogfmtFields(%q)\n got %q (nil: %v) %+v\nwant %q %+v", line, prefix, prefix == nil, got, wantPrefix, want)
	}

	for i := range line {
		line[i] ^= 0xff
	}
	if !isFlipped(prefix, wantPrefix) {
		t.Fatalf("%q: the prefix %q is not a clipped window into the line", line, wantPrefix)
	}
	for i, field := range fields {
		if !isFlipped(field.Key, want[i].key) || !isFlipped(field.Value.Raw, want[i].raw) {
			t.Fatalf("field %d (%q=%q) is not a pair of clipped windows into the line", i, want[i].key, want[i].raw)
		}
	}
}

// checkLogfmtRoundTrip reads data as NUL-separated keys and values in turn, writes them as a logfmt line
// and expects the same pairs back.
func checkLogfmtRoundTrip(t *testing.T, data []byte, quoted uint8) {
	t.Helper()
	parts := bytes.Split(data, []byte{0})
	n := len(parts) / 2
	if n < minLogfmtPairs {
		return
	}
	var line []byte
	for i := range n {
		key, value := parts[2*i], parts[2*i+1]
		if !logfmtKey.Match(key) {
			return
		}
		if i > 0 {
			line = append(line, ' ')
		}
		line = appendLogfmtPair(line, key, value, quoted>>(i%8)&1 == 1)
	}
	line = slices.Clip(line)

	fields, prefix, ok := appendLogfmtFields(nil, line)

	if !ok || len(prefix) != 0 || len(fields) != n {
		t.Fatalf("appendLogfmtFields(%q): ok = %v, prefix %q, %d fields; want the %d encoded pairs", line, ok, prefix, len(fields), n)
	}
	for i, field := range fields {
		key, value := parts[2*i], parts[2*i+1]
		if got := field.Value.AppendText(nil); !bytes.Equal(field.Key, key) || field.Value.Kind != entry.KindString || !bytes.Equal(got, value) {
			t.Fatalf("appendLogfmtFields(%q): pair %d = %q=%q (kind %d), want %q=%q", line, i, field.Key, got, field.Value.Kind, key, value)
		}
	}
}

// appendLogfmtPair writes key=value, quoting the value when it would not survive bare or when quote is set.
func appendLogfmtPair(dst, key, value []byte, quote bool) []byte {
	dst = append(append(dst, key...), '=')
	if !quote && !bytes.ContainsAny(value, logfmtSpace) && !bytes.HasPrefix(value, []byte{'"'}) {
		return append(dst, value...)
	}
	dst = append(dst, '"')
	for _, c := range value {
		if c == '"' || c == '\\' {
			dst = append(dst, '\\')
		}
		dst = append(dst, c)
	}
	return append(dst, '"')
}

// referenceDateTime is the obvious implementation parseDateTime must agree with: time.Parse over every layout.
func referenceDateTime(s string, loc *time.Location) (time.Time, bool) {
	// time.Parse also takes a run of spaces for one and an hour of one digit, which no logger writes.
	if len(s) < 13 || !isDigit(s[11]) || !isDigit(s[12]) {
		return time.Time{}, false
	}
	for _, sep := range []string{"T", " "} {
		for _, zone := range []string{"", "Z07:00", "Z0700", "Z07"} {
			if t, err := time.ParseInLocation("2006-01-02"+sep+"15:04:05"+zone, s, loc); err == nil {
				// The parsed time keeps the zone it was written in, so this is the year as written.
				return t, 2000 <= t.Year() && t.Year() <= 2099
			}
		}
	}
	return time.Time{}, false
}

func FuzzDateTime(f *testing.F) {
	for _, seed := range []string{
		"2026-10-11T12:00:00Z", "2026-10-11T12:00:00+03:00", "2026-10-11T12:00:00-0730", "2026-10-11T12:00:00-08",
		"2026-10-11T12:00:00.123456789Z", "2026-10-11T12:00:00.1234567899+24:60", "2026-10-11 12:00:00,123",
		"2026-10-11 12:00:00", "2028-02-29T23:59:59Z", "2026-02-29T00:00:00Z", "2026-04-31T00:00:00Z",
		"1999-12-31T23:59:59Z", "2100-01-01T00:00:00Z", "2000-01-01T00:00:00+14:00", "2099-12-31T23:59:59-12:00",
		"2026-10-11T24:00:00Z", "2026-10-11T12:60:00Z", "2026-10-11T12:00:60Z", "2026-10-11T12:00:00.Z",
		"2026-10-11T12:00:00+25:00", "2026-10-11T12:00:00+00:61", "2026-10-11T12:00:00+3", "2026-10-11T12:00:00 UTC",
		"2026-10-11t12:00:00z", "2026-10-11", "1791720000", "", "2000-10-01 0:00:00", "2000-10-01  0:00:00",
		"2000-10-01  00:00:00",
	} {
		f.Add(seed)
	}

	f.Fuzz(func(t *testing.T, s string) {
		want, wantOK := referenceDateTime(s, testLoc)

		// Clipped, so that a read past the end panics instead of finding spare capacity.
		got, ok := parseDateTime(slices.Clip([]byte(s)), testLoc)

		if ok != wantOK {
			t.Fatalf("parseDateTime(%q) accepted = %v, time.Parse says %v", s, ok, wantOK)
		}
		if ok && !got.Equal(want) {
			t.Fatalf("parseDateTime(%q) = %s, time.Parse gives %s", s, got.Format(time.RFC3339Nano), want.Format(time.RFC3339Nano))
		}
	})
}

var unixTimeNumber = regexp.MustCompile(`^[0-9]{1,19}(\.[0-9]+)?$`)

// referenceUnixTime is the obvious implementation parseUnixTime must agree with: exact arithmetic on fractions.
func referenceUnixTime(s string) (time.Time, bool) {
	if !unixTimeNumber.MatchString(s) {
		return time.Time{}, false
	}
	value, _ := new(big.Rat).SetString(s)
	for _, perSecond := range []int64{1, 1e3, 1e6, 1e9} {
		seconds := new(big.Rat).Quo(value, big.NewRat(perSecond, 1))
		if seconds.Cmp(big.NewRat(946684800, 1)) < 0 || seconds.Cmp(big.NewRat(4102444800, 1)) >= 0 {
			continue
		}
		nanos := new(big.Rat).Mul(seconds, big.NewRat(1e9, 1))
		// Integer division rounds down, since the number is not negative.
		return time.Unix(0, new(big.Int).Quo(nanos.Num(), nanos.Denom()).Int64()).UTC(), true
	}
	return time.Time{}, false
}

func FuzzUnixTime(f *testing.F) {
	for _, seed := range []string{
		"1791720000", "1791720000123", "1791720000123456", "1791720000123456789", "1791720000.1234567",
		"1791720000.1234567899", "1791720000123.5", "1791720000123456.25", "1791720000123456789.5", "001791720000",
		"946684800", "946684799", "4102444799", "4102444800", "946684799999.999", "4102444799999999999.9",
		"17917200001", "17917200001234567890", "18446744075501271616", "5", "0", "", "-1791720000", "1.79172e9",
		"1791720000.", ".5", "1791720000,5", "1791720000.5x",
	} {
		f.Add(seed)
	}

	f.Fuzz(func(t *testing.T, s string) {
		want, wantOK := referenceUnixTime(s)

		// Clipped, so that a read past the end panics instead of finding spare capacity.
		got, ok := parseUnixTime(slices.Clip([]byte(s)))

		if ok != wantOK {
			t.Fatalf("parseUnixTime(%q) accepted = %v, the reference says %v", s, ok, wantOK)
		}
		if ok && !got.Equal(want) {
			t.Fatalf("parseUnixTime(%q) = %s, the reference gives %s", s, got.Format(time.RFC3339Nano), want.Format(time.RFC3339Nano))
		}
	})
}
