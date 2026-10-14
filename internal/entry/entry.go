// Package entry defines the parsed form of a log line.
package entry

import "time"

// Format is the syntax a line was recognized as.
type Format uint8

// FormatPlain is the fallback and the zero value.
const (
	FormatPlain Format = iota
	FormatJSON
	FormatLogfmt
)

var formatNames = [...]string{"plain", "json", "logfmt"}

func (f Format) String() string {
	if int(f) >= len(formatNames) {
		return "unknown"
	}
	return formatNames[f]
}

// Kind is the type of a field value.
type Kind uint8

// Kinds of values. KindNone is a logfmt key written without "=".
const (
	KindNone Kind = iota
	KindString
	KindNumber
	KindBool
	KindNull
	KindObject
	KindArray
)

var kindNames = [...]string{"none", "string", "number", "bool", "null", "object", "array"}

func (k Kind) String() string {
	if int(k) >= len(kindNames) {
		return "unknown"
	}
	return kindNames[k]
}

// Value is a field value that still points into the line it came from.
type Value struct {
	Raw     []byte // the token as written; for a string, the text between the quotes
	Kind    Kind
	Escaped bool // Raw holds backslash escapes
}

// Field is one key and its value.
type Field struct {
	Key   []byte
	Value Value
}

// Entry is a parsed line. Its slices point into buffers the parser reuses, so it is valid until the next line is parsed.
type Entry struct {
	Line      []byte
	Collector []byte // what a log collector wrote in front of the line, except its timestamp: "web-1  |", "[pod/web-1/app]"
	Source    []byte // the stream that Collector names: "web-1", "pod/web-1/app"
	Stamp     []byte // the timestamp a log collector wrote, as kubectl --timestamps and docker compose -t do
	Body      []byte // the line past what the collector wrote
	Prefix    []byte // the logger's own text before logfmt pairs, as in klog
	Format    Format
	Time      time.Time
	Level     Level   // unknown unless a field names it; the one exception is a Serilog event without "@l", which is info
	Fields    []Field // every pair of the line, in its original order

	// Positions in Fields plus one, so that the zero Entry has no roles.
	timeField, levelField, msgField int
}

// TimeField returns the index in Fields of the pair recognized as the time, or -1.
func (e *Entry) TimeField() int { return e.timeField - 1 }

// LevelField returns the index in Fields of the pair recognized as the level, or -1.
func (e *Entry) LevelField() int { return e.levelField - 1 }

// MsgField returns the index in Fields of the pair recognized as the message, or -1.
func (e *Entry) MsgField() int { return e.msgField - 1 }

// SetTimeField marks Fields[i] as the time; a negative i clears the role.
func (e *Entry) SetTimeField(i int) { e.timeField = max(i, -1) + 1 }

// SetLevelField marks Fields[i] as the level; a negative i clears the role.
func (e *Entry) SetLevelField(i int) { e.levelField = max(i, -1) + 1 }

// SetMsgField marks Fields[i] as the message; a negative i clears the role.
func (e *Entry) SetMsgField(i int) { e.msgField = max(i, -1) + 1 }

// Reset empties the entry for the next line and keeps the capacity of Fields.
func (e *Entry) Reset() {
	*e = Entry{Fields: e.Fields[:0]}
}
