package golden

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// recorder stands in for the test that calls the helper, so that its failures can be looked at.
type recorder struct {
	testing.TB
	errors []string
	fatal  string
}

func (r *recorder) Helper() {}

func (r *recorder) Errorf(format string, args ...any) {
	r.errors = append(r.errors, fmt.Sprintf(format, args...))
}

// Fatalf ends the calling function the way the real one does, by leaving the goroutine.
func (r *recorder) Fatalf(format string, args ...any) {
	r.fatal = fmt.Sprintf(format, args...)
	panic(r)
}

// run calls f and reports what it told the test.
func run(f func(testing.TB)) (r *recorder) {
	r = &recorder{}
	defer func() {
		if p := recover(); p != nil && p != any(r) {
			panic(p)
		}
	}()
	f(r)
	return r
}

func setUpdate(t *testing.T, on bool) {
	t.Helper()
	old := *update
	*update = on
	t.Cleanup(func() { *update = old })
}

func TestCheck(t *testing.T) {
	setUpdate(t, false)
	const content = "one\ntwo\nthree\n"
	path := filepath.Join(t.TempDir(), "out.golden")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name string
		got  string
		want []string // parts of the one error, none if the output matches
	}{
		{name: "same", got: "one\ntwo\nthree\n"},
		{name: "a line differs", got: "one\n2\nthree\n", want: []string{path, "line 2", ` got: "2"`, `want: "two"`}},
		{name: "the first line differs", got: "1\ntwo\nthree\n", want: []string{"line 1", ` got: "1"`, `want: "one"`}},
		{name: "output ends sooner", got: "one\n", want: []string{"line 2", ` got: ""`, `want: "two"`}},
		{name: "output has no final newline", got: "one\ntwo\nthree", want: []string{"line 4", " got: (end)", `want: ""`}},
		{name: "output goes on", got: "one\ntwo\nthree\nfour\n", want: []string{"line 4", ` got: "four"`, `want: ""`}},
		{name: "empty output", got: "", want: []string{"line 1", ` got: ""`, `want: "one"`}},
		{name: "a control character is shown escaped", got: "one\ntwo\x1b\nthree\n", want: []string{"line 2", `"two\x1b"`}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := run(func(tb testing.TB) { Check(tb, path, []byte(tt.got)) })

			if r.fatal != "" {
				t.Fatalf("Check stopped the test: %s", r.fatal)
			}
			// Only -update may write: a mismatch that rewrote the file would pass on the next run.
			if kept, err := os.ReadFile(path); err != nil || string(kept) != content {
				t.Fatalf("after Check the file holds %q (%v), want it untouched", kept, err)
			}
			if len(tt.want) == 0 {
				if len(r.errors) != 0 {
					t.Errorf("Check reported %q for output equal to the file", r.errors)
				}
				return
			}
			if len(r.errors) != 1 {
				t.Fatalf("Check reported %d errors (%q), want one", len(r.errors), r.errors)
			}
			for _, part := range tt.want {
				if !strings.Contains(r.errors[0], part) {
					t.Errorf("the error is\n%s\nwant it to contain %q", r.errors[0], part)
				}
			}
		})
	}
}

func TestCheckWithoutFile(t *testing.T) {
	setUpdate(t, false)
	path := filepath.Join(t.TempDir(), "missing.golden")

	r := run(func(tb testing.TB) { Check(tb, path, []byte("x\n")) })

	if !strings.Contains(r.fatal, "missing.golden") || !strings.Contains(r.fatal, "-update") {
		t.Errorf("Check said %q, want it to name the file and the -update flag", r.fatal)
	}
	if _, err := os.Stat(path); err == nil {
		t.Errorf("Check created %s without -update", path)
	}
}

func TestCheckUpdates(t *testing.T) {
	setUpdate(t, true)
	path := filepath.Join(t.TempDir(), "out.golden")

	for _, content := range []string{"first\n", "second, written over the first\n", ""} {
		r := run(func(tb testing.TB) { Check(tb, path, []byte(content)) })
		if r.fatal != "" || len(r.errors) != 0 {
			t.Fatalf("Check with -update reported %q and %q, want nothing", r.errors, r.fatal)
		}
		if got, err := os.ReadFile(path); err != nil || string(got) != content {
			t.Errorf("the file holds %q (%v), want %q", got, err, content)
		}
	}
}

func TestCheckUpdateFails(t *testing.T) {
	setUpdate(t, true)
	path := filepath.Join(t.TempDir(), "no-such-dir", "out.golden")

	r := run(func(tb testing.TB) { Check(tb, path, []byte("x\n")) })

	if !strings.Contains(r.fatal, "no-such-dir") {
		t.Errorf("Check said %q, want the error of the failed write", r.fatal)
	}
}

func TestCheckOrphans(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"a.golden", "b.golden", "c.golden", "a.log"} {
		if err := os.WriteFile(filepath.Join(dir, name), nil, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	pattern := filepath.Join(dir, "*.golden")

	r := run(func(tb testing.TB) {
		CheckOrphans(tb, pattern, func(path string) bool { return filepath.Base(path) != "b.golden" })
	})
	if r.fatal != "" || len(r.errors) != 1 || !strings.Contains(r.errors[0], "b.golden") {
		t.Errorf("CheckOrphans reported %q and %q, want one error that names b.golden", r.errors, r.fatal)
	}

	r = run(func(tb testing.TB) { CheckOrphans(tb, pattern, func(string) bool { return true }) })
	if r.fatal != "" || len(r.errors) != 0 {
		t.Errorf("CheckOrphans reported %q and %q when every file is in use, want nothing", r.errors, r.fatal)
	}

	r = run(func(tb testing.TB) { CheckOrphans(tb, "[", func(string) bool { return true }) })
	if r.fatal == "" {
		t.Errorf("CheckOrphans accepted a malformed pattern")
	}
}
