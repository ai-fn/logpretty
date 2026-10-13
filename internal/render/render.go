package render

import (
	"bytes"
	"time"

	"github.com/ai-fn/logpretty/internal/entry"
	"github.com/ai-fn/logpretty/internal/parse"
)

const (
	timeLayout    = "15:04:05.000"
	byteOrderMark = "\xef\xbb\xbf"
	// levelWidth is the room a level takes: the longest name.
	levelWidth = 5
)

var levelNames = [entry.LevelFatal + 1]string{"", "TRACE", "DEBUG", "INFO", "WARN", "ERROR", "FATAL"}

// Renderer formats entries as lines of text. It keeps scratch space between calls, so it is not safe for
// concurrent use.
type Renderer struct {
	Theme  Theme
	Loc    *time.Location // the zone the time is shown in; nil means time.Local
	NoTime bool           // leave the time out
	Fields *Selection     // what to show besides the time and the level; nil shows every field

	formatter
	finder parse.Finder
}

// AppendEntry appends the entry as one line: prefix, time, level, message, then the other fields; a line without
// fields is printed as it is, and an entry that Fields and NoTime leave empty adds nothing. dst must not share memory with e.
func (r *Renderer) AppendEntry(dst []byte, e *entry.Entry) []byte {
	if e.Format == entry.FormatPlain || len(e.Fields) == 0 {
		return append(appendSafe(dst, trimByteOrderMark(e.Line)), '\n')
	}
	th, sel := &r.Theme, r.Fields
	start := len(dst)
	// The separator is written before a part, not after it, so that a part the entry lacks leaves no spaces behind.
	sep := ""

	// The prefix, like a plain line, keeps its tabs; only the parts built here write them out.
	if len(e.Prefix) > 0 {
		style := ""
		if e.CollectorPrefix {
			style = th.Prefix
		}
		dst = th.end(appendSafe(append(dst, style...), e.Prefix), style)
		sep = " "
	}
	hasTime := !e.Time.IsZero()
	hideTime := hasTime && r.NoTime
	// found: whether anything asked for is shown. Without a selection only a hidden time can empty an entry,
	// so a blank message still gets its line.
	found := sel == nil && !hideTime
	if hasTime && !hideTime {
		loc := r.Loc
		if loc == nil {
			loc = time.Local
		}
		dst = append(append(dst, sep...), th.Time...)
		dst = th.end(e.Time.In(loc).AppendFormat(dst, timeLayout), th.Time)
		sep = " "
		if sel != nil && sel.time {
			found = true
		}
	}
	// The level may be known without a field: a Serilog event implies it.
	hasLevel := e.Level > entry.LevelUnknown && int(e.Level) < len(levelNames)
	if hasLevel {
		name, style := levelNames[e.Level], th.Level[e.Level]
		dst = append(append(append(dst, sep...), style...), name...)
		dst = th.end(dst, style)
		sep = " "
		if len(name) < levelWidth {
			sep = "  " // INFO and WARN are a column short of the other names
		}
		if sel == nil || sel.level {
			found = true
		}
	}
	msg := e.MsgField()
	if msg >= 0 && (sel == nil || sel.msg) {
		// Spaces at the end would be taken for the gap before the fields, or left at the end of the line.
		if text := bytes.TrimRight(r.text(e.Fields[msg].Value), " "); len(text) > 0 {
			dst = AppendInline(append(dst, sep...), text)
			sep = "  "
			found = true
		}
	}

	if sel == nil {
		for i, f := range e.Fields {
			if i == e.TimeField() || i == e.LevelField() || i == msg {
				continue
			}
			dst = append(dst, sep...)
			sep = " "
			found = true
			// A logfmt word without "=" is a part of the text, not a key.
			if f.Value.Kind == entry.KindNone {
				dst = AppendInline(dst, f.Key)
				continue
			}
			dst = append(appendKey(append(dst, th.Key...), f.Key), '=')
			dst = r.appendValue(th.end(dst, th.Key), f.Value)
		}
	} else {
		var shown bool
		dst, shown = r.appendSelected(dst, e, sel, sep, [roleCount]bool{roleMsg: msg >= 0, roleTime: hasTime, roleLevel: hasLevel})
		found = found || shown
	}
	if !found {
		return dst[:start]
	}
	return append(dst, '\n')
}

// appendSelected appends the fields of sel that the entry has, in the order of sel, and reports whether it had
// any. inColumn tells the roles the entry has: such a part is shown in its column, or hidden with it, so its
// name is a field only for an entry without one.
func (r *Renderer) appendSelected(dst []byte, e *entry.Entry, sel *Selection, sep string, inColumn [roleCount]bool) ([]byte, bool) {
	th := &r.Theme
	found := false
	for _, f := range sel.fields {
		if inColumn[f.role] {
			continue
		}
		value, ok := r.finder.Find(e, f.name)
		if !ok {
			continue
		}
		dst = append(appendKey(append(append(dst, sep...), th.Key...), f.name), '=')
		dst = r.appendValue(th.end(dst, th.Key), value)
		sep = " "
		found = true
	}
	return dst, found
}

// AppendTruncated appends a line that was cut at the length limit, as it is, with a note of the bytes dropped.
// dst must not share memory with line.
func (r *Renderer) AppendTruncated(dst, line []byte, dropped int) []byte {
	th := &r.Theme
	dst = appendSafe(dst, trimByteOrderMark(line))
	dst = append(append(dst, th.Marker...), "…[+"...)
	dst = append(appendSize(dst, dropped), ']')
	return append(th.end(dst, th.Marker), '\n')
}

// trimByteOrderMark drops the mark Windows tools start a file with: on a terminal it is noise.
func trimByteOrderMark(line []byte) []byte {
	return bytes.TrimPrefix(line, []byte(byteOrderMark))
}
