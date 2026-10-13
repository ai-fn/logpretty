package filter

import (
	"errors"
	"strings"
	"testing"
)

func TestParseExpr(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want expr
	}{
		{name: "key alone", in: "user_id", want: expr{key: "user_id", op: opExists}},
		{name: "key alone with spaces around", in: " \tuser_id  ", want: expr{key: "user_id", op: opExists}},
		{name: "equal", in: "user_id=42", want: expr{key: "user_id", op: opEq, value: "42", valuePos: 8}},
		{name: "not equal", in: "status!=200", want: expr{key: "status", op: opNe, value: "200", valuePos: 8}},
		{name: "greater", in: "ms>100", want: expr{key: "ms", op: opGt, value: "100", valuePos: 3}},
		{name: "greater or equal", in: "ms>=100", want: expr{key: "ms", op: opGe, value: "100", valuePos: 4}},
		{name: "less", in: "ms<100", want: expr{key: "ms", op: opLt, value: "100", valuePos: 3}},
		{name: "less or equal", in: "ms<=100", want: expr{key: "ms", op: opLe, value: "100", valuePos: 4}},
		{name: "match", in: "msg~timeout", want: expr{key: "msg", op: opMatch, value: "timeout", valuePos: 4}},
		{name: "no match", in: "msg!~timeout", want: expr{key: "msg", op: opNoMatch, value: "timeout", valuePos: 5}},
		{name: "spaces around the operator", in: "user_id = 42", want: expr{key: "user_id", op: opEq, value: "42", valuePos: 10}},
		{name: "tabs and spaces everywhere", in: "\t ms \t>=\t 100 \t", want: expr{key: "ms", op: opGe, value: "100", valuePos: 10}},
		{name: "line breaks are spaces", in: "a\n=\r1\n", want: expr{key: "a", op: opEq, value: "1", valuePos: 4}},
		{name: "brackets inside a value", in: "msg~^(a|b)$", want: expr{key: "msg", op: opMatch, value: "^(a|b)$", valuePos: 4}},
		{name: "closing bracket at the end of a value", in: "a=x)", want: expr{key: "a", op: opEq, value: "x)", valuePos: 2}},
		{name: "single quote inside a value", in: "a=it's", want: expr{key: "a", op: opEq, value: "it's", valuePos: 2}},
		{name: "brackets in a quoted value", in: `msg~"(a|b)"`, want: expr{key: "msg", op: opMatch, value: "(a|b)", quotedValue: true, valuePos: 4}},
		{name: "single quote in a quoted key", in: `"it's"`, want: expr{key: "it's", literal: true, op: opExists}},
		{name: "NUL in a bare value", in: "a=\x00", want: expr{key: "a", op: opEq, value: "\x00", valuePos: 2}},
		{name: "control character in a bare key", in: "a\x1b=1", want: expr{key: "a\x1b", op: opEq, value: "1", valuePos: 3}},
		{name: "bytes that are not UTF-8", in: "\xff=\xfe", want: expr{key: "\xff", op: opEq, value: "\xfe", valuePos: 2}},
		{name: "negative number", in: "delta<-5", want: expr{key: "delta", op: opLt, value: "-5", valuePos: 6}},
		{name: "operators inside a value", in: "url~^https://x/?a=1&b>2", want: expr{key: "url", op: opMatch, value: "^https://x/?a=1&b>2", valuePos: 4}},
		{name: "value that ends in an operator", in: "a=b=", want: expr{key: "a", op: opEq, value: "b=", valuePos: 2}},
		{name: "backslashes in a bare value stay", in: `msg~^\d+\s$`, want: expr{key: "msg", op: opMatch, value: `^\d+\s$`, valuePos: 4}},
		{name: "key with a dot", in: "http.status>=500", want: expr{key: "http.status", op: opGe, value: "500", valuePos: 13}},
		{name: "key with an at sign", in: "@timestamp", want: expr{key: "@timestamp", op: opExists}},
		{name: "key with a dash and a slash", in: "x-request/id=7", want: expr{key: "x-request/id", op: opEq, value: "7", valuePos: 13}},
		{name: "key and value that are not ASCII", in: "ключ=значение", want: expr{key: "ключ", op: opEq, value: "значение", valuePos: 9}},

		{name: "quoted value", in: `msg="connection refused"`, want: expr{key: "msg", op: opEq, value: "connection refused", quotedValue: true, valuePos: 4}},
		{name: "quoted empty value", in: `msg=""`, want: expr{key: "msg", op: opEq, quotedValue: true, valuePos: 4}},
		{name: "quoted number", in: `id="042"`, want: expr{key: "id", op: opEq, value: "042", quotedValue: true, valuePos: 3}},
		{name: "quoted key and quoted value", in: `"id"!="042"`, want: expr{key: "id", literal: true, op: opNe, value: "042", quotedValue: true, valuePos: 6}},
		{name: "quoted value that starts with an operator", in: `msg~"!important"`, want: expr{key: "msg", op: opMatch, value: "!important", quotedValue: true, valuePos: 4}},
		{name: "quoted value with spaces after it", in: `msg = "a b"  `, want: expr{key: "msg", op: opEq, value: "a b", quotedValue: true, valuePos: 6}},
		{name: "escaped quote", in: `msg="say \"hi\""`, want: expr{key: "msg", op: opEq, value: `say "hi"`, quotedValue: true, valuePos: 4}},
		{name: "escaped backslash", in: `path="C:\\dir"`, want: expr{key: "path", op: opEq, value: `C:\dir`, quotedValue: true, valuePos: 5}},
		{name: "escaped backslash before the closing quote", in: `path="dir\\"`, want: expr{key: "path", op: opEq, value: `dir\`, quotedValue: true, valuePos: 5}},
		{name: "any other backslash stays", in: `msg~"\d+ ms\n"`, want: expr{key: "msg", op: opMatch, value: `\d+ ms\n`, quotedValue: true, valuePos: 4}},
		{name: "quoted key", in: `"level"=3`, want: expr{key: "level", literal: true, op: opEq, value: "3", valuePos: 8}},
		{name: "quoted key alone", in: `"my key"`, want: expr{key: "my key", literal: true, op: opExists}},
		{name: "quoted key with an operator in it", in: `"a=b" != c`, want: expr{key: "a=b", literal: true, op: opNe, value: "c", valuePos: 9}},
		{name: "quoted empty key", in: `""=1`, want: expr{literal: true, op: opEq, value: "1", valuePos: 3}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseExpr(tt.in)
			if err != nil {
				t.Fatalf("parseExpr(%q) fails: %v", tt.in, err)
			}
			if got != tt.want {
				t.Errorf("parseExpr(%q)\n got %+v\nwant %+v", tt.in, got, tt.want)
			}
		})
	}
}

func TestParseExprErrors(t *testing.T) {
	const quoteHint = "an expression is one condition, and a value with spaces needs double quotes"

	tests := []struct {
		name    string
		in      string
		wantPos int
		wantMsg string
	}{
		{name: "empty", in: "", wantPos: 0, wantMsg: "empty expression"},
		{name: "only spaces", in: "  \t", wantPos: 3, wantMsg: "empty expression"},
		{name: "no key", in: "=5", wantPos: 0, wantMsg: `unexpected '='`},
		{name: "no key after spaces", in: "  >5", wantPos: 2, wantMsg: `unexpected '>'`},
		{name: "two operators", in: "user_id=>42", wantPos: 8, wantMsg: `unexpected '>'`},
		{name: "two equals signs", in: "a==1", wantPos: 2, wantMsg: `unexpected '='`},
		{name: "the match of another language", in: "a=~x", wantPos: 2, wantMsg: `unexpected '~'`},
		{name: "less and greater", in: "a<>1", wantPos: 2, wantMsg: `unexpected '>'`},
		{name: "operator after spaces", in: "a = > 1", wantPos: 4, wantMsg: `unexpected '>'`},
		{name: "bang alone", in: "a!", wantPos: 1, wantMsg: `unexpected '!'`},
		{name: "bang before a value", in: "a!x", wantPos: 1, wantMsg: `unexpected '!'`},
		{name: "bang before a space", in: "a! =x", wantPos: 1, wantMsg: `unexpected '!'`},
		{name: "no value", in: "a=", wantPos: 2, wantMsg: "missing value"},
		{name: "no value after spaces", in: "a>= ", wantPos: 4, wantMsg: "missing value"},
		{name: "quote not closed", in: `a="x`, wantPos: 2, wantMsg: "unterminated quote"},
		{name: "quote closed by an escaped one", in: `a="x\"`, wantPos: 2, wantMsg: "unterminated quote"},
		{name: "quote that ends in a backslash", in: `a="x\`, wantPos: 2, wantMsg: "unterminated quote"},
		{name: "key quote not closed", in: `"a`, wantPos: 0, wantMsg: "unterminated quote"},
		{name: "text after a quoted value", in: `a="x"y`, wantPos: 5, wantMsg: `unexpected 'y'`},
		{name: "second quoted value", in: `a="x" "y"`, wantPos: 6, wantMsg: `unexpected '"'`},
		{name: "value with a space", in: "a=b c", wantPos: 4, wantMsg: `unexpected 'c': ` + quoteHint},
		{name: "another condition", in: "a=1 b=2", wantPos: 4, wantMsg: `unexpected 'b': ` + quoteHint},
		{name: "two keys", in: "a b", wantPos: 2, wantMsg: `unexpected 'b'`},
		{name: "quote inside a bare value", in: `a=b"c`, wantPos: 3, wantMsg: `unexpected '"'`},
		{name: "quote inside a bare key", in: `a"b`, wantPos: 1, wantMsg: `unexpected '"'`},
		{name: "text after a quoted key", in: `"a"b`, wantPos: 3, wantMsg: `unexpected 'b'`},
		{name: "grouping", in: "(a=1)", wantPos: 0, wantMsg: `unexpected '(': text with it needs double quotes`},
		{name: "bracket inside a key", in: "a(b=1", wantPos: 1, wantMsg: `unexpected '(': text with it needs double quotes`},
		{name: "closing bracket inside a key", in: "a)=1", wantPos: 1, wantMsg: `unexpected ')': text with it needs double quotes`},
		{name: "value that starts with a bracket", in: "msg~(a|b)", wantPos: 4, wantMsg: `unexpected '(': text with it needs double quotes`},
		{name: "value that starts with a closing bracket", in: "a=)", wantPos: 2, wantMsg: `unexpected ')': text with it needs double quotes`},
		{name: "value in single quotes", in: "msg='x'", wantPos: 4, wantMsg: `unexpected '\'': text with it needs double quotes`},
		{name: "key in single quotes", in: "'a'=1", wantPos: 0, wantMsg: `unexpected '\'': text with it needs double quotes`},
		{name: "single quote inside a key", in: "it's=1", wantPos: 2, wantMsg: `unexpected '\'': text with it needs double quotes`},
		{name: "bracket after a value", in: "a=1 (b=2)", wantPos: 4, wantMsg: `unexpected '(': text with it needs double quotes`},
		{name: "what follows is not UTF-8", in: "a \xff", wantPos: 2, wantMsg: "unexpected '\ufffd'"},
		{name: "what follows is not ASCII", in: "a=b я", wantPos: 4, wantMsg: `unexpected 'я'`},
		{name: "what follows is a control character", in: "a \x1b", wantPos: 2, wantMsg: `unexpected '\x1b'`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseExpr(tt.in)

			var syntax *SyntaxError
			if !errors.As(err, &syntax) {
				t.Fatalf("parseExpr(%q) = %+v, %v; want a *SyntaxError", tt.in, got, err)
			}
			if syntax.Pos != tt.wantPos || !strings.HasPrefix(syntax.Msg, tt.wantMsg) || syntax.Error() != syntax.Msg {
				t.Errorf("parseExpr(%q): position %d, %q; want %d, %q", tt.in, syntax.Pos, syntax.Error(), tt.wantPos, tt.wantMsg)
			}
			if got != (expr{}) {
				t.Errorf("parseExpr(%q) = %+v with an error, want the zero value", tt.in, got)
			}
		})
	}
}

// The hint about quotes is only for text after a bare value: after a quoted one it would mislead.
func TestParseExprHintsQuotesOnlyAfterABareValue(t *testing.T) {
	for in, want := range map[string]bool{"a=b c": true, "a=1 and b=2": true, `a="b" c`: false, "a b": false, "a c=1": false, `a=b"c`: false, `a=b "c"`: false, "a=1 (b": false} {
		_, err := parseExpr(in)
		if err == nil {
			t.Fatalf("parseExpr(%q) succeeds", in)
		}
		if got := strings.Contains(err.Error(), "one condition"); got != want {
			t.Errorf("parseExpr(%q) error = %q; hint about quotes = %v, want %v", in, err, got, want)
		}
	}
}
