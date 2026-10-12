package filter

import (
	"fmt"
	"strings"
	"unicode/utf8"
)

// op is the operator of a condition.
type op uint8

const (
	opExists op = iota // a key alone
	opEq
	opNe
	opGt
	opGe
	opLt
	opLe
	opMatch
	opNoMatch
)

// expr is a condition as it is written, before its value is given a type.
type expr struct {
	key      string
	literal  bool // the key was in quotes
	op       op
	value    string
	valuePos int // where the value starts in the expression, in bytes
}

// SyntaxError is an expression that cannot be read.
type SyntaxError struct {
	Pos int // where the trouble is, in bytes from the start of the expression
	Msg string
}

func (e *SyntaxError) Error() string { return e.Msg }

// parseExpr reads "key" or "key OP value". A bare token has no space, quote, operator or bracket; a value
// may hold the last two past its first byte. It is strict, so that "and", "or" and groups can be added later.
func parseExpr(s string) (expr, error) {
	var e expr
	i := skipSpace(s, 0)
	if i == len(s) {
		return expr{}, &SyntaxError{Pos: i, Msg: "empty expression"}
	}

	var err error
	if s[i] == '"' {
		e.literal = true
		e.key, i, err = quoted(s, i)
		if err != nil {
			return expr{}, err
		}
	} else {
		start := i
		for i < len(s) && !isSpace(s[i]) && s[i] != '"' && !isOpByte(s[i]) && !isReserved(s[i]) {
			i++
		}
		if i == start {
			return expr{}, unexpected(s, i)
		}
		e.key = s[start:i]
	}

	if i = skipSpace(s, i); i == len(s) {
		return e, nil
	}
	var width int
	if e.op, width = opAt(s, i); width == 0 {
		return expr{}, unexpected(s, i)
	}

	i = skipSpace(s, i+width)
	e.valuePos = i
	bare := false
	switch {
	case i == len(s):
		return expr{}, &SyntaxError{Pos: i, Msg: `missing value; an empty one is written ""`}
	case s[i] == '"':
		e.value, i, err = quoted(s, i)
		if err != nil {
			return expr{}, err
		}
	case isOpByte(s[i]), isReserved(s[i]):
		return expr{}, unexpected(s, i)
	default:
		// Past its first byte a value may hold an operator or a bracket, as a URL or a regular expression does.
		start := i
		for i < len(s) && !isSpace(s[i]) && s[i] != '"' {
			i++
		}
		e.value = s[start:i]
		bare = true
	}

	if i = skipSpace(s, i); i < len(s) {
		err := unexpected(s, i)
		if bare && s[i] != '"' && !isReserved(s[i]) {
			err.Msg += ": an expression is one condition, and a value with spaces needs double quotes"
		}
		return expr{}, err
	}
	return e, nil
}

// quoted reads the token whose opening quote is at s[start] and returns it with the index past its closing
// quote. Only a quote and a backslash are escaped: any other backslash stays, as a regular expression needs.
func quoted(s string, start int) (text string, end int, err error) {
	var b strings.Builder
	for i := start + 1; i < len(s); i++ {
		switch c := s[i]; {
		case c == '"':
			return b.String(), i + 1, nil
		case c == '\\' && i+1 < len(s) && (s[i+1] == '"' || s[i+1] == '\\'):
			i++
			b.WriteByte(s[i])
		default:
			b.WriteByte(c)
		}
	}
	return "", 0, &SyntaxError{Pos: start, Msg: "unterminated quote"}
}

// opAt returns the operator that starts at s[i] and its length, or a length of zero.
func opAt(s string, i int) (op, int) {
	next := byte(0)
	if i+1 < len(s) {
		next = s[i+1]
	}
	switch s[i] {
	case '=':
		return opEq, 1
	case '~':
		return opMatch, 1
	case '!':
		switch next {
		case '=':
			return opNe, 2
		case '~':
			return opNoMatch, 2
		}
	case '>':
		if next == '=' {
			return opGe, 2
		}
		return opGt, 1
	case '<':
		if next == '=' {
			return opLe, 2
		}
		return opLt, 1
	}
	return opExists, 0
}

// unexpected reports the character at s[i], quoted so that a control character cannot reach the terminal.
func unexpected(s string, i int) *SyntaxError {
	r, _ := utf8.DecodeRuneInString(s[i:])
	err := &SyntaxError{Pos: i, Msg: fmt.Sprintf("unexpected %q", r)}
	if isReserved(s[i]) {
		err.Msg += ": text with it needs double quotes"
	}
	return err
}

// isReserved reports the bytes kept out of a bare token for later: brackets for grouping, and the single
// quote, which is no quote here, so that 'text' fails instead of matching nothing.
func isReserved(c byte) bool {
	return c == '(' || c == ')' || c == '\''
}

func isOpByte(c byte) bool {
	return c == '=' || c == '!' || c == '<' || c == '>' || c == '~'
}

func isSpace(c byte) bool {
	return c == ' ' || c == '\t' || c == '\n' || c == '\r'
}

func skipSpace(s string, i int) int {
	for i < len(s) && isSpace(s[i]) {
		i++
	}
	return i
}
