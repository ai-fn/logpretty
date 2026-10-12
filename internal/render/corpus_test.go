package render

import (
	"bytes"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/ai-fn/logpretty/internal/golden"
	"github.com/ai-fn/logpretty/internal/parse"
)

// corpusInputs returns the log files to render by name: the corpus of the parser and the files of this package.
func corpusInputs(t *testing.T) map[string]string {
	t.Helper()
	inputs := map[string]string{}
	for _, pattern := range []string{
		filepath.Join("..", "parse", "testdata", "corpus", "*.log"),
		filepath.Join("testdata", "*.log"),
	} {
		paths, err := filepath.Glob(pattern)
		if err != nil {
			t.Fatal(err)
		}
		if len(paths) == 0 {
			t.Fatalf("no log files match %s", pattern)
		}
		for _, path := range paths {
			name := strings.TrimSuffix(filepath.Base(path), ".log")
			if other, ok := inputs[name]; ok {
				t.Fatalf("%s and %s would share a golden file", other, path)
			}
			inputs[name] = path
		}
	}
	return inputs
}

// TestCorpus renders real log lines and compares the output with golden files; after a change of the layout
// read the diff, then rewrite them with "go test ./internal/render -run TestCorpus -update".
func TestCorpus(t *testing.T) {
	inputs := corpusInputs(t)
	golden.CheckOrphans(t, filepath.Join("testdata", "golden", "*.golden"), func(path string) bool {
		_, ok := inputs[strings.TrimSuffix(filepath.Base(path), ".golden")]
		return ok
	})

	for name, path := range inputs {
		t.Run(name, func(t *testing.T) {
			input, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}

			p := parse.Parser{Loc: time.UTC}
			r := Renderer{Loc: time.UTC}
			var got []byte
			for _, line := range bytes.Split(bytes.TrimSuffix(input, []byte("\n")), []byte("\n")) {
				// Clipped, so that a read past the end of the line panics instead of finding spare capacity.
				got = r.AppendEntry(got, p.Parse(slices.Clip(line)))
			}

			golden.Check(t, filepath.Join("testdata", "golden", name+".golden"), got)
		})
	}
}
