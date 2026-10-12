// Package filter decides which lines of the stream are shown.
package filter

import "github.com/ai-fn/logpretty/internal/entry"

// Filter keeps the entries that pass its conditions. It remembers its last decision, so it is not safe
// for concurrent use. The zero Filter keeps every line.
type Filter struct {
	MinLevel entry.Level // the lowest level to keep; LevelUnknown keeps entries of any level and of none

	keptLast bool // the decision on the last entry that was not plain
}

// Keep reports whether to show the entry. A plain line goes the way of the entry before it: a stack
// trace is of no use without its error, and noise next to a hidden one.
func (f *Filter) Keep(e *entry.Entry) bool {
	if e.Format == entry.FormatPlain {
		return f.KeepPlain()
	}
	// LevelUnknown is below every level, so an entry without one passes only when none is asked for.
	f.keptLast = e.Level >= f.MinLevel
	return f.keptLast
}

// KeepPlain reports whether to show a line that was not parsed, such as one cut at the length limit.
func (f *Filter) KeepPlain() bool {
	return !f.active() || f.keptLast
}

// active reports whether any entry can be hidden; if none can, plain lines before the first entry are shown too.
func (f *Filter) active() bool {
	return f.MinLevel > entry.LevelUnknown
}
