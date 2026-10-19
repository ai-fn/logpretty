package parse

import (
	"bytes"

	"github.com/ai-fn/logpretty/internal/entry"
)

// Google Cloud writes "projects/P/traces/ID" or the bare ID.
const gcpTraceKey = "logging.googleapis.com/trace"

// Keys of the trace id at the top level, the most trusted first; "traceid" covers traceId and TraceId of .NET and Jaeger.
var traceKeys = [...]string{"trace_id", "traceid", "trace.id", "dd.trace_id", "@tr", gcpTraceKey}

// Where the id is nested when no top-level key has it: ECS loggers and dd-trace-js write objects.
var nestedTraceKeys = [...]struct{ object, key string }{{"trace", "id"}, {"dd", "trace_id"}}

// Trace returns the trace id of the entry, decoded and for Google Cloud cut after the last slash, and its field or -1
// when nested. The id is valid until the next call of Trace or the next Parse.
func (f *Finder) Trace(e *entry.Entry) (id entry.Value, field int, ok bool) {
	best, bestRank := -1, len(traceKeys)
	nested := false // whether the passes below can find anything: most lines have no such field
	for i, fl := range e.Fields {
		switch len(fl.Key) {
		case len("dd"), len("trace"), len("scopes"):
			nested = nested || fl.Value.Kind == entry.KindObject || fl.Value.Kind == entry.KindArray
			continue
		case len("@tr"), len("traceid"), len("trace_id"), len("dd.trace_id"), len(gcpTraceKey):
		default:
			continue
		}
		if r := rank(traceKeys[:bestRank], fl.Key); r < bestRank {
			if _, ok := f.traceText(fl.Key, fl.Value); ok {
				best, bestRank = i, r
			}
		}
	}
	// Decoded again: a later candidate may have overwritten the text of the best one.
	if best >= 0 {
		id, _ = f.traceText(e.Fields[best].Key, e.Fields[best].Value)
		return id, best, true
	}
	if !nested {
		return entry.Value{}, -1, false
	}

	for _, n := range nestedTraceKeys {
		for _, fl := range e.Fields {
			if fl.Value.Kind == entry.KindObject && equalFold(fl.Key, n.object) {
				if id, ok := f.traceIn(fl.Value.Raw, n.key); ok {
					return id, -1, true
				}
			}
		}
	}
	// .NET AddJsonConsole puts the id into the objects of "Scopes", next to strings.
	for _, fl := range e.Fields {
		if fl.Value.Kind == entry.KindArray && equalFold(fl.Key, "scopes") {
			if id, ok := f.traceInScopes(fl.Value.Raw); ok {
				return id, -1, true
			}
		}
	}
	return entry.Value{}, -1, false
}

// traceIn returns the first usable id under key, which is lower case, among the pairs of a JSON object.
func (f *Finder) traceIn(object []byte, key string) (entry.Value, bool) {
	start := len(f.scratch)
	f.scratch = appendObjectFields(f.scratch, object)
	for _, fl := range f.scratch[start:] {
		if equalFold(fl.Key, key) {
			if id, ok := f.traceText(fl.Key, fl.Value); ok {
				f.scratch = f.scratch[:start]
				return id, true
			}
		}
	}
	f.scratch = f.scratch[:start]
	return entry.Value{}, false
}

// traceInScopes returns the first usable "TraceId" among the objects of a JSON array.
func (f *Finder) traceInScopes(array []byte) (entry.Value, bool) {
	for i := skipJSONSpace(array, 1); array[i] != ']'; {
		switch array[i] {
		case '{':
			end := compositeEnd(array, i)
			if id, ok := f.traceIn(array[i:end], "traceid"); ok {
				return id, true
			}
			i = end
		case '[':
			i = compositeEnd(array, i)
		case '"':
			end, _ := stringEnd(array, i+1)
			i = end + 1
		default:
			for !isSpace(array[i]) && array[i] != ',' && array[i] != ']' {
				i++
			}
		}
		i = skipJSONSpace(array, i)
		if array[i] == ',' {
			i = skipJSONSpace(array, i+1)
		}
	}
	return entry.Value{}, false
}

// traceText returns the id a value holds: a non-empty string, decoded, or a number, which Datadog writes.
func (f *Finder) traceText(key []byte, v entry.Value) (entry.Value, bool) {
	switch {
	case v.Kind != entry.KindString && v.Kind != entry.KindNumber:
		return entry.Value{}, false
	case v.Escaped:
		f.text = v.AppendText(f.text[:0])
		v = entry.Value{Raw: f.text, Kind: v.Kind}
	}
	if equalFold(key, gcpTraceKey) {
		v.Raw = v.Raw[bytes.LastIndexByte(v.Raw, '/')+1:]
	}
	return v, len(v.Raw) > 0
}
