package parse

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/ai-fn/logpretty/internal/entry"
	"github.com/ai-fn/logpretty/internal/golden"
)

func dumpEntry(b *strings.Builder, n int, line []byte, e *entry.Entry) {
	fmt.Fprintf(b, "line %d: %s\n  in: %q\n", n, e.Format, line)
	for _, part := range []struct {
		name  string
		value []byte
	}{{"collector", e.Collector}, {"source", e.Source}, {"stamp", e.Stamp}} {
		if len(part.value) > 0 {
			fmt.Fprintf(b, "  %s: %q\n", part.name, part.value)
		}
	}
	if e.Format == entry.FormatPlain {
		// A plain line has a time too: the one a collector wrote.
		if !e.Time.IsZero() {
			fmt.Fprintf(b, "  time: %s\n", e.Time.UTC().Format(time.RFC3339Nano))
		}
		return
	}
	fmt.Fprintf(b, "  level: %s\n", e.Level)
	if e.Time.IsZero() {
		b.WriteString("  time: -\n")
	} else {
		fmt.Fprintf(b, "  time: %s\n", e.Time.UTC().Format(time.RFC3339Nano))
	}
	key := func(i int) string {
		if i < 0 {
			return "-"
		}
		return string(e.Fields[i].Key)
	}
	fmt.Fprintf(b, "  roles: time=%s level=%s msg=%s\n", key(e.TimeField()), key(e.LevelField()), key(e.MsgField()))
	if len(e.Prefix) > 0 {
		fmt.Fprintf(b, "  prefix: %q\n", e.Prefix)
	}
	b.WriteString("  fields:\n")
	for _, f := range e.Fields {
		fmt.Fprintf(b, "    %q=<%s>%q\n", f.Key, f.Value.Kind, f.Value.AppendText(nil))
	}
}

// TestCorpus compares the parsed corpus with its golden files; after a change of the heuristics read the
// diff, then rewrite them with "go test ./internal/parse -run TestCorpus -update".
func TestCorpus(t *testing.T) {
	logs, err := filepath.Glob(filepath.Join("testdata", "corpus", "*.log"))
	if err != nil {
		t.Fatal(err)
	}
	if len(logs) == 0 {
		t.Fatal("no corpus files found")
	}
	golden.CheckOrphans(t, filepath.Join("testdata", "corpus", "*.golden"), func(path string) bool {
		return slices.Contains(logs, strings.TrimSuffix(path, ".golden")+".log")
	})

	for _, path := range logs {
		name := strings.TrimSuffix(filepath.Base(path), ".log")
		t.Run(name, func(t *testing.T) {
			input, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}

			if len(input) == 0 {
				t.Fatal("the log file is empty")
			}

			p := Parser{Loc: time.UTC}
			var got strings.Builder
			for i, line := range bytes.Split(bytes.TrimSuffix(input, []byte("\n")), []byte("\n")) {
				// Clipped, so that a read past the end of the line panics instead of finding spare capacity.
				dumpEntry(&got, i+1, line, p.Parse(slices.Clip(line)))
			}

			golden.Check(t, strings.TrimSuffix(path, ".log")+".golden", []byte(got.String()))
		})
	}
}
