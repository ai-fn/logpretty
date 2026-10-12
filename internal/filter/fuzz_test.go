package filter

import (
	"cmp"
	"errors"
	"math/big"
	"regexp"
	"strconv"
	"testing"
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
		e.value = exprEscape.ReplaceAllString(value, "$1")
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
		if got.key != want.key || got.literal != want.literal || got.op != want.op || got.value != want.value {
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
