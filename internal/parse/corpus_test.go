package parse

import (
	"bytes"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/ai-fn/logpretty/internal/entry"
)

var update = flag.Bool("update", false, "rewrite the golden files of the corpus")

func dumpEntry(b *strings.Builder, n int, line []byte, e *entry.Entry) {
	fmt.Fprintf(b, "line %d: %s\n  in: %q\n", n, e.Format, line)
	if e.Format == entry.FormatPlain {
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

// firstDifference names the log line where two dumps part, so that a failure points at it.
func firstDifference(got, want string) string {
	gotLines, wantLines := strings.Split(got, "\n"), strings.Split(want, "\n")
	header := "the start"
	for i := range max(len(gotLines), len(wantLines)) {
		// A dump that ends sooner differs from one with an empty line there.
		if i >= len(gotLines) || i >= len(wantLines) || gotLines[i] != wantLines[i] {
			side := func(lines []string) string {
				if i >= len(lines) {
					return "(end)"
				}
				return fmt.Sprintf("%q", lines[i])
			}
			return fmt.Sprintf("first difference after %q\n got: %s\nwant: %s", header, side(gotLines), side(wantLines))
		}
		if strings.HasPrefix(gotLines[i], "line ") {
			header = gotLines[i]
		}
	}
	return "no difference"
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
	// A golden file whose log was renamed or deleted would stay unchecked for good.
	goldens, err := filepath.Glob(filepath.Join("testdata", "corpus", "*.golden"))
	if err != nil {
		t.Fatal(err)
	}
	for _, golden := range goldens {
		if !slices.Contains(logs, strings.TrimSuffix(golden, ".golden")+".log") {
			t.Errorf("%s has no log file next to it", golden)
		}
	}

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

			golden := strings.TrimSuffix(path, ".log") + ".golden"
			if *update {
				if err := os.WriteFile(golden, []byte(got.String()), 0o644); err != nil {
					t.Fatal(err)
				}
				return
			}
			want, err := os.ReadFile(golden)
			if err != nil {
				t.Fatalf("%v (create it with -update)", err)
			}
			if got.String() != string(want) {
				t.Errorf("%s differs from the golden file\n%s", name, firstDifference(got.String(), string(want)))
			}
		})
	}
}
