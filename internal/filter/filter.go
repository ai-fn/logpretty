// Package filter decides which lines of the stream are shown.
package filter

import (
	"bytes"

	"github.com/ai-fn/logpretty/internal/entry"
	"github.com/ai-fn/logpretty/internal/parse"
)

// Filter keeps the entries that pass its conditions. It remembers its last decision, so it is not safe
// for concurrent use. The zero Filter keeps every line.
type Filter struct {
	MinLevel entry.Level // the lowest level to keep; LevelUnknown keeps entries of any level and of none
	Where    []Cond      // conditions an entry must meet, all of them

	keptLast bool // the decision on the last entry that was not plain
	finder   parse.Finder
	scratch  []byte // a value with its escapes decoded
}

// Keep reports whether to show the entry. A plain line goes the way of the entry before it: a stack
// trace is of no use without its error, and noise next to a hidden one.
func (f *Filter) Keep(e *entry.Entry) bool {
	if e.Format == entry.FormatPlain {
		return f.KeepPlain()
	}
	// LevelUnknown is below every level, so an entry without one passes only when none is asked for.
	f.keptLast = e.Level >= f.MinLevel && f.meets(e)
	return f.keptLast
}

func (f *Filter) meets(e *entry.Entry) bool {
	for i := range f.Where {
		if !f.Where[i].match(e, f) {
			return false
		}
	}
	return true
}

// lookup finds the field a condition names: by the path, or for a key in quotes by that very name.
func (f *Filter) lookup(e *entry.Entry, c *Cond) (entry.Value, bool) {
	if !c.literal {
		return f.finder.Find(e, c.key)
	}
	for _, field := range e.Fields {
		if field.Value.Kind != entry.KindNone && bytes.Equal(field.Key, c.key) {
			return field.Value, true
		}
	}
	return entry.Value{}, false
}

// KeepPlain reports whether to show a line that was not parsed, such as one cut at the length limit.
func (f *Filter) KeepPlain() bool {
	return !f.active() || f.keptLast
}

// active reports whether any entry can be hidden; if none can, plain lines before the first entry are shown too.
func (f *Filter) active() bool {
	return f.MinLevel > entry.LevelUnknown || len(f.Where) > 0
}
