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
	// NoExpand keeps a value with line breaks in the line, written out, instead of unfolding it under the entry.
	NoExpand bool

	formatter
	finder parse.Finder
	blocks []block // the values to unfold under the entry being rendered
}

// block is a field whose value is unfolded under its entry.
type block struct {
	key   []byte
	value entry.Value
}

// AppendEntry appends the entry as one line and the values with line breaks unfolded under it; a line without fields
// is printed as written, and an entry that Fields and NoTime leave empty adds nothing. dst must not share memory with e.
func (r *Renderer) AppendEntry(dst []byte, e *entry.Entry) []byte {
	if e.Format == entry.FormatPlain || len(e.Fields) == 0 {
		return r.appendPlain(dst, e)
	}
	sel := r.Fields
	start := len(dst)
	clear(r.blocks) // the slices of the last entry are not kept alive
	r.blocks = r.blocks[:0]
	// The separator is written before a part, not after it, so that a part the entry lacks leaves no spaces behind.
	dst, sep := r.appendCollector(dst, e)

	hasTime := !e.Time.IsZero()
	hideTime := hasTime && r.NoTime
	showTime := hasTime && !hideTime
	// found: whether anything asked for is shown. Without a selection only a hidden time can empty an entry,
	// so a blank message still gets its line.
	found := (sel == nil && !hideTime) || (showTime && sel != nil && sel.time)
	// The time a collector wrote stays where it wrote it, before the logger's own text.
	stampTime := showTime && e.TimeField() < 0
	if stampTime {
		dst, sep = r.appendTime(append(dst, sep...), e), " "
	}
	// The logger's own text, like a plain line, keeps its tabs; only the parts built here write them out.
	if len(e.Prefix) > 0 {
		dst = appendSafe(append(dst, sep...), e.Prefix)
		sep = " "
	}
	if showTime && !stampTime {
		dst, sep = r.appendTime(append(dst, sep...), e), " "
	}
	// The level may be known without a field: a Serilog event implies it.
	hasLevel := e.Level > entry.LevelUnknown && int(e.Level) < len(levelNames)
	if hasLevel {
		dst, sep = r.appendLevel(append(dst, sep...), e.Level)
		found = found || sel == nil || sel.level
	}
	msg := e.MsgField()
	if msg >= 0 && (sel == nil || sel.msg) {
		var shown bool
		if dst, sep, shown = r.appendMessage(dst, sep, e.Fields[msg].Value); shown {
			found = true
		}
	}

	var shown bool
	if sel == nil {
		dst, shown = r.appendAll(dst, e, sep, msg)
	} else {
		dst, shown = r.appendSelected(dst, e, sel, sep, [roleCount]bool{roleMsg: msg >= 0, roleTime: hasTime, roleLevel: hasLevel})
	}
	found = found || shown
	if !found {
		return dst[:start]
	}
	dst = append(dst, '\n')
	for _, b := range r.blocks {
		dst = r.appendBlock(dst, b)
	}
	return dst
}

// appendPlain appends a line that is not an entry, or an entry without fields, as it was written but for what a
// log collector wrote: that is faint, and its time goes to the time column.
func (r *Renderer) appendPlain(dst []byte, e *entry.Entry) []byte {
	if e.Collector == nil && e.Stamp == nil {
		return append(appendSafe(dst, trimByteOrderMark(e.Line)), '\n')
	}
	var sep string
	dst, sep = r.appendCollector(dst, e)
	if !e.Time.IsZero() && !r.NoTime {
		dst, sep = r.appendTime(append(dst, sep...), e), " "
	}
	if len(e.Body) > 0 {
		dst = appendSafe(append(dst, sep...), e.Body)
	}
	return append(dst, '\n')
}

// appendMessage appends the message, and returns the separator that the next part needs; shown is false for one
// that is nothing but spaces.
func (r *Renderer) appendMessage(dst []byte, sep string, v entry.Value) (out []byte, next string, shown bool) {
	// Spaces at the end would be taken for the gap before the fields, or left at the end of the line.
	text := bytes.TrimRight(r.text(v), " ")
	if len(text) == 0 {
		return dst, sep, false
	}
	return AppendInline(append(dst, sep...), text), "  ", true
}

// appendLevel appends the level name and returns the separator that the next part needs.
func (r *Renderer) appendLevel(dst []byte, level entry.Level) ([]byte, string) {
	th := &r.Theme
	name, style := levelNames[level], th.Level[level]
	dst = th.end(append(append(dst, style...), name...), style)
	if len(name) < levelWidth {
		return dst, "  " // INFO and WARN are a column short of the other names
	}
	return dst, " "
}

// appendAll appends every field of the entry that no column shows, and reports whether there was any.
func (r *Renderer) appendAll(dst []byte, e *entry.Entry, sep string, msg int) ([]byte, bool) {
	found := false
	for i, f := range e.Fields {
		if i == e.TimeField() || i == e.LevelField() || i == msg {
			continue
		}
		found = true
		// A logfmt word without "=" is a part of the text, not a key.
		if f.Value.Kind == entry.KindNone {
			dst, sep = AppendInline(append(dst, sep...), f.Key), " "
			continue
		}
		var inLine bool
		if dst, inLine = r.appendField(dst, sep, f.Key, f.Value); inLine {
			sep = " "
		}
	}
	return dst, found
}

// appendField appends key=value, or keeps a value with line breaks for a block under the entry and reports
// that nothing was appended.
func (r *Renderer) appendField(dst []byte, sep string, key []byte, v entry.Value) (out []byte, inLine bool) {
	var text []byte
	// A line break can only come from an escape, and a value with escapes is decoded to be shown anyway.
	decoded := v.Kind == entry.KindString && v.Escaped
	if decoded {
		if text = r.text(v); !r.NoExpand && hasInnerBreak(text) {
			r.blocks = append(r.blocks, block{key, v})
			return dst, false
		}
	}
	th := &r.Theme
	dst = append(appendKey(append(append(dst, sep...), th.Key...), key), '=')
	if dst = th.end(dst, th.Key); decoded {
		return appendText(dst, text, " \""), true
	}
	return r.appendValue(dst, v), true
}

// hasInnerBreak reports a line feed before the last byte: one at the very end does not make a second line.
func hasInnerBreak(text []byte) bool {
	i := bytes.IndexByte(text, '\n')
	return i >= 0 && i < len(text)-1
}

// appendBlock appends a value under its entry: the key, then each line indented, so that none of them can pass
// for an entry; a tab stays, as in a stack trace, and "\r\n" is one break.
func (r *Renderer) appendBlock(dst []byte, b block) []byte {
	th := &r.Theme
	dst = append(appendKey(append(append(dst, "  "...), th.Key...), b.key), ':')
	dst = append(th.end(dst, th.Key), '\n')
	// A block has a line feed before the last byte, so the text is not empty. The break at the end is dropped
	// with its "\r"; any other "\r" is written out, unless it starts a "\r\n" in the text.
	text := r.text(b.value)
	if n := len(text); text[n-1] == '\n' {
		text = bytes.TrimSuffix(text[:n-1], []byte("\r"))
	}
	for {
		i := bytes.IndexByte(text, '\n')
		line := text
		if i >= 0 {
			line = bytes.TrimSuffix(text[:i], []byte("\r"))
		}
		if len(line) > 0 {
			dst = appendSafe(append(dst, "    "...), line)
		}
		dst = append(dst, '\n')
		if i < 0 {
			return dst
		}
		text = text[i+1:]
	}
}

// appendCollector appends, faint, what a log collector wrote in front of the line, and returns the separator
// that the next part needs. Its timestamp is the time column instead.
func (r *Renderer) appendCollector(dst []byte, e *entry.Entry) ([]byte, string) {
	if len(e.Collector) == 0 {
		return dst, ""
	}
	th := &r.Theme
	return th.end(appendSafe(append(dst, th.Prefix...), e.Collector), th.Prefix), " "
}

// appendTime appends the time column, in the zone of the output.
func (r *Renderer) appendTime(dst []byte, e *entry.Entry) []byte {
	th, loc := &r.Theme, r.Loc
	if loc == nil {
		loc = time.Local
	}
	return th.end(e.Time.In(loc).AppendFormat(append(dst, th.Time...), timeLayout), th.Time)
}

// appendSelected appends the fields of sel that the entry has, in the order of sel, and reports whether it had
// any. inColumn tells the roles the entry has: such a part is shown in its column, or hidden with it, so its
// name is a field only for an entry without one.
func (r *Renderer) appendSelected(dst []byte, e *entry.Entry, sel *Selection, sep string, inColumn [roleCount]bool) ([]byte, bool) {
	found := false
	for _, f := range sel.fields {
		if inColumn[f.role] {
			continue
		}
		var value entry.Value
		ok := false
		if f.role == roleTrace {
			value, _, ok = r.finder.Trace(e)
		}
		// Without an id, trace is a field like any other.
		if !ok {
			if value, ok = r.finder.Find(e, f.name); !ok {
				continue
			}
		}
		var inLine bool
		if dst, inLine = r.appendField(dst, sep, f.name, value); inLine {
			sep = " "
		}
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
