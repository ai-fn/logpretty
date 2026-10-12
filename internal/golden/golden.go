// Package golden compares the output of a test with a file kept next to it. It is for tests only: importing it
// registers the -update flag, which the command must not have.
package golden

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

var update = flag.Bool("update", false, "rewrite the golden files with the output of the tests")

// Check fails the test if got is not what the file at path holds. With -update it writes the file instead;
// give the flag to one package at a time, the others do not know it.
func Check(t testing.TB, path string, got []byte) {
	t.Helper()
	if *update {
		if err := os.WriteFile(path, got, 0o644); err != nil {
			t.Fatalf("%v", err)
		}
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("%v (create it with -update)", err)
	}
	if string(got) != string(want) {
		t.Errorf("the output differs from %s\n%s", path, firstDifference(string(got), string(want)))
	}
}

// CheckOrphans fails the test for every file that matches pattern and that used does not know: a golden
// file whose input was renamed or deleted would stay unchecked for good.
func CheckOrphans(t testing.TB, pattern string, used func(path string) bool) {
	t.Helper()
	paths, err := filepath.Glob(pattern)
	if err != nil {
		t.Fatalf("%s: %v", pattern, err)
	}
	for _, path := range paths {
		if !used(path) {
			t.Errorf("%s belongs to no input of the test", path)
		}
	}
}

// firstDifference shows the first line where the two texts part.
func firstDifference(got, want string) string {
	gotLines, wantLines := strings.Split(got, "\n"), strings.Split(want, "\n")
	// A text that ends sooner differs from one with an empty line there.
	side := func(lines []string, i int) string {
		if i >= len(lines) {
			return "(end)"
		}
		return fmt.Sprintf("%q", lines[i])
	}
	i := 0
	for i < len(gotLines) && i < len(wantLines) && gotLines[i] == wantLines[i] {
		i++
	}
	return fmt.Sprintf("first difference at line %d\n got: %s\nwant: %s", i+1, side(gotLines, i), side(wantLines, i))
}
