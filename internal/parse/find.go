package parse

import (
	"bytes"

	"github.com/ai-fn/logpretty/internal/entry"
)

// Finder looks a field up by the path a user writes. It keeps scratch space between calls, so it is not
// safe for concurrent use.
type Finder struct {
	scratch []entry.Field // the pairs of the nested objects on the way down
}

// Find returns the value that path names in the entry: a key as it is written, dots and all, or failing
// that the way down nested JSON objects, as in "http.status". Of several pairs that fit, the first wins.
func (f *Finder) Find(e *entry.Entry, path []byte) (entry.Value, bool) {
	return f.find(e.Fields, path)
}

func (f *Finder) find(fields []entry.Field, path []byte) (entry.Value, bool) {
	// A logfmt word without "=" is a part of the text, not a key.
	for _, field := range fields {
		if field.Value.Kind != entry.KindNone && bytes.Equal(field.Key, path) {
			return field.Value, true
		}
	}
	for _, field := range fields {
		key := field.Key
		if field.Value.Kind != entry.KindObject || len(path) <= len(key) || path[len(key)] != '.' || !bytes.HasPrefix(path, key) {
			continue
		}
		// The pairs go after the ones still being walked: a search further down must not overwrite them.
		start := len(f.scratch)
		f.scratch = appendObjectFields(f.scratch, field.Value.Raw)
		value, ok := f.find(f.scratch[start:], path[len(key)+1:])
		f.scratch = f.scratch[:start]
		if ok {
			return value, true
		}
	}
	return entry.Value{}, false
}
