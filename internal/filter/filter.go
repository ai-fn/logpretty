// Package filter decides which lines of the stream are shown.
package filter

import (
	"bytes"
	"cmp"
	"fmt"

	"github.com/ai-fn/logpretty/internal/entry"
	"github.com/ai-fn/logpretty/internal/parse"
)

// PlainMode is what becomes of the lines that are not entries, such as a stack trace.
type PlainMode uint8

// Plain modes. PlainFollow sends a plain line the way of the entry before it.
const (
	PlainFollow PlainMode = iota
	PlainShow
	PlainHide
)

func (m PlainMode) String() string {
	switch m {
	case PlainFollow:
		return "follow"
	case PlainShow:
		return "show"
	case PlainHide:
		return "hide"
	}
	return fmt.Sprintf("PlainMode(%d)", uint8(m))
}

// ParsePlainMode reads the mode as it is written on the command line.
func ParsePlainMode(s string) (PlainMode, error) {
	switch s {
	case "follow":
		return PlainFollow, nil
	case "show":
		return PlainShow, nil
	case "hide":
		return PlainHide, nil
	}
	return PlainFollow, fmt.Errorf("invalid plain mode %q: want follow, show or hide", s)
}

// Filter keeps the entries that pass its conditions. It remembers its last decision, so it is not safe
// for concurrent use. The zero Filter keeps every line.
type Filter struct {
	MinLevel entry.Level // the lowest level to keep; LevelUnknown keeps entries of any level and of none
	Where    []Cond      // conditions an entry must meet, all of them
	Plain    PlainMode   // what becomes of the plain lines; the zero mode sends them the way of the entry before

	keptLast bool           // the decision on the last entry that was not plain
	sources  map[string]int // the index in kept of a container or pod, so that an update does not allocate
	kept     []bool         // the decision on the last entry of each source
	limit    int            // the sources remembered at most; 0 means maxSources, a test sets less
	finder   parse.Finder
	scratch  []byte // a value with its escapes decoded
}

// maxSources bounds the sources remembered, so that a stream with a new name on every line cannot grow the memory.
const maxSources = 1024

// Keep reports whether to show the entry. A plain line goes the way of the entry before it, of its own
// container or pod if it has one, unless Plain says otherwise: a stack trace is of no use without its error.
func (f *Filter) Keep(e *entry.Entry) bool {
	if e.Format == entry.FormatPlain {
		return f.KeepPlain(e.Source)
	}
	// LevelUnknown is below every level, so an entry without one passes only when none is asked for.
	f.keptLast = e.Level >= f.MinLevel && f.meets(e)
	if len(e.Source) > 0 && f.Plain == PlainFollow && f.active() {
		f.remember(e.Source)
	}
	return f.keptLast
}

// remember records the decision on the last entry for its source; a new source past the limit is not remembered.
func (f *Filter) remember(source []byte) {
	if i, ok := f.sources[string(source)]; ok {
		f.kept[i] = f.keptLast
		return
	}
	if limit := cmp.Or(f.limit, maxSources); len(f.kept) >= limit {
		return
	}
	if f.sources == nil {
		f.sources = map[string]int{}
	}
	f.sources[string(source)] = len(f.kept)
	f.kept = append(f.kept, f.keptLast)
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

// KeepPlain reports whether to show a plain line of the source, which is nil for none. A line that was not
// parsed, such as one cut at the length limit, is a plain line without a source, whatever it was before the cut.
func (f *Filter) KeepPlain(source []byte) bool {
	switch f.Plain {
	case PlainShow:
		return true
	case PlainHide:
		return false
	}
	if !f.active() {
		return true
	}
	if i, ok := f.sources[string(source)]; ok {
		return f.kept[i]
	}
	return f.keptLast
}

// active reports whether any entry can be hidden; if none can, plain lines before the first entry are shown too.
func (f *Filter) active() bool {
	return f.MinLevel > entry.LevelUnknown || len(f.Where) > 0
}
