package filter

import (
	"bytes"
	"cmp"
	"errors"
	"fmt"
	"regexp"
	"regexp/syntax"
	"strings"
	"time"

	"github.com/ai-fn/logpretty/internal/entry"
	"github.com/ai-fn/logpretty/internal/parse"
)

// role is what a key means besides a field of its name.
type role uint8

const (
	roleNone role = iota
	roleMsg
	roleLevel
	roleTime
)

// Cond is one compiled condition: a key, an operator and a value that has been given its type.
type Cond struct {
	key     []byte
	literal bool // the key was in quotes: it is a field of that very name, with no role and no way down
	role    role
	op      op

	text  []byte         // the value as written
	num   number         // the value as a number, if it is compared as one
	isNum bool           // whether num holds the value
	re    *regexp.Regexp // for ~ and !~
	level entry.Level    // for a comparison with the level of an entry
	when  time.Time      // for a comparison with the time of an entry
}

// Compile reads a condition and checks its value: a regular expression for ~ and !~, a level or a time for
// those keys, a number for an order. A time written without a zone is in loc, or local if nil.
func Compile(expression string, loc *time.Location) (Cond, error) {
	e, err := parseExpr(expression)
	if err != nil {
		return Cond{}, err
	}
	c := Cond{key: []byte(e.key), literal: e.literal, op: e.op, text: []byte(e.value)}
	if !e.literal {
		c.role = roleOf(e.key)
	}

	msg := ""
	switch {
	case c.op == opExists:
	case c.op == opMatch || c.op == opNoMatch:
		if c.re, err = regexp.Compile(e.value); err != nil {
			// The whole text of the error would repeat the expression, line breaks and all.
			msg = "invalid regular expression"
			var cause *syntax.Error
			if errors.As(err, &cause) {
				msg += ": " + string(cause.Code)
			}
		}
	case c.role == roleLevel:
		if c.level, err = ParseLevel(e.value); err != nil {
			msg = err.Error()
		}
	case c.role == roleTime:
		var ok bool
		if c.when, ok = parse.Time(c.text, loc); !ok {
			msg = fmt.Sprintf("%q is not a time: want one such as 2026-10-11T12:00:00Z", e.value)
		}
	default:
		// In quotes a value is text for = and !=: "042" is not 42 there. An order takes a number whatever the
		// quotes, since an order of strings would take "90ms" for more than "100ms" without a word of warning.
		order := c.op != opEq && c.op != opNe
		if order || !e.quotedValue {
			c.num, c.isNum = parseNumber(c.text)
		}
		if order && !c.isNum {
			msg = fmt.Sprintf("%q is not a number, and >, >=, < and <= compare numbers", e.value)
		}
	}
	if msg != "" {
		return Cond{}, &SyntaxError{Pos: e.valuePos, Msg: msg}
	}
	return c, nil
}

func roleOf(key string) role {
	switch key {
	case "msg":
		return roleMsg
	case "level":
		return roleLevel
	case "time":
		return roleTime
	}
	return roleNone
}

// ParseLevel reads a level by the names the parser knows, so "warning" and "ERR" work as they do in a log.
// The offsets of slog are turned down: "info+2" would read as a level above info and filter as info.
func ParseLevel(s string) (entry.Level, error) {
	if level := entry.ParseLevel([]byte(s)); level != entry.LevelUnknown && !strings.ContainsAny(s, "+- \t\r\n") {
		return level, nil
	}
	return entry.LevelUnknown, fmt.Errorf("invalid level %q: want trace, debug, info, warn, error or fatal", s)
}

// match reports whether the entry meets the condition. A field the entry lacks meets only != and !~.
func (c *Cond) match(e *entry.Entry, f *Filter) bool {
	typed := c.op != opExists && c.op != opMatch && c.op != opNoMatch
	switch {
	case c.role == roleLevel && e.Level != entry.LevelUnknown:
		switch {
		case c.op == opExists:
			return true
		case !typed:
			// The name every spelling is brought to: "WARNING", "wrn" and 40 are all "warn".
			return c.re.MatchString(e.Level.String()) == (c.op == opMatch)
		}
		return holds(c.op, cmp.Compare(e.Level, c.level))
	case c.role == roleTime && e.TimeField() >= 0:
		if typed {
			return holds(c.op, e.Time.Compare(c.when))
		}
		return c.matchValue(e.Fields[e.TimeField()].Value, f)
	case c.role == roleMsg && e.MsgField() >= 0:
		return c.matchValue(e.Fields[e.MsgField()].Value, f)
	case typed && (c.role == roleLevel || c.role == roleTime):
		// A field of that name which is not a level or a time has nothing to compare.
		return c.op == opNe
	}

	value, ok := f.lookup(e, c)
	if !ok {
		return c.op == opNe || c.op == opNoMatch
	}
	return c.matchValue(value, f)
}

// matchValue compares the text of a value: a string with its escapes decoded, anything else as written.
func (c *Cond) matchValue(v entry.Value, f *Filter) bool {
	if c.op == opExists {
		return true
	}
	text := v.Raw
	if v.Escaped {
		f.scratch = v.AppendText(f.scratch[:0])
		text = f.scratch
	}
	switch c.op {
	case opMatch:
		return c.re.Match(text)
	case opNoMatch:
		return !c.re.Match(text)
	}

	// logfmt has no numbers and JSON often quotes them, so the kind of the field does not decide.
	if c.isNum {
		if n, ok := parseNumber(text); ok {
			return holds(c.op, n.compare(c.num))
		}
	}
	// An order needs two numbers.
	if c.op != opEq && c.op != opNe {
		return false
	}
	return bytes.Equal(text, c.text) == (c.op == opEq)
}

// holds reports whether the result of a comparison, below, equal or above zero, is what the operator asks.
// An operator that does not compare holds for nothing.
func holds(o op, order int) bool {
	switch o {
	case opEq:
		return order == 0
	case opNe:
		return order != 0
	case opGt:
		return order > 0
	case opGe:
		return order >= 0
	case opLt:
		return order < 0
	case opLe:
		return order <= 0
	}
	return false
}
