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
			if strings.HasSuffix(name, ".color") {
				t.Fatalf("%s would share a golden file with the colored output of another input", path)
			}
			if other, ok := inputs[name]; ok {
				t.Fatalf("%s and %s would share a golden file", other, path)
			}
			inputs[name] = path
		}
	}
	return inputs
}

// TestCorpus renders real log lines, with color and without, and compares the output with golden files;
// after a change of the layout read the diff, then rewrite them with "go test ./internal/render -run TestCorpus -update".
func TestCorpus(t *testing.T) {
	inputs := corpusInputs(t)
	golden.CheckOrphans(t, filepath.Join("testdata", "golden", "*.golden"), func(path string) bool {
		name := strings.TrimSuffix(strings.TrimSuffix(filepath.Base(path), ".golden"), ".color")
		_, ok := inputs[name]
		return ok
	})

	themes := []struct {
		suffix string
		theme  Theme
	}{
		{suffix: ".golden"},
		{suffix: ".color.golden", theme: DefaultTheme()},
	}
	for name, path := range inputs {
		t.Run(name, func(t *testing.T) {
			input, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}

			for _, tt := range themes {
				p := parse.Parser{Loc: time.UTC}
				r := Renderer{Theme: tt.theme, Loc: time.UTC}
				var got []byte
				for _, line := range bytes.Split(bytes.TrimSuffix(input, []byte("\n")), []byte("\n")) {
					// Clipped, so that a read past the end of the line panics instead of finding spare capacity.
					got = r.AppendEntry(got, p.Parse(slices.Clip(line)))
				}
				if tt.theme == (Theme{}) {
					if bytes.IndexByte(got, 0x1b) >= 0 {
						t.Fatalf("the output without color has an escape character: %q", got)
					}
				} else {
					// The escape character is spelled out, so that the file can be read in a diff and in an editor.
					got = bytes.ReplaceAll(got, []byte{0x1b}, []byte(`\e`))
				}

				golden.Check(t, filepath.Join("testdata", "golden", name+tt.suffix), got)
			}
		})
	}
}
