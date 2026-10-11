package main

import (
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/rogpeppe/go-internal/testscript"
)

// TestMain lets the test binary run as the logpretty command, so the tests execute the real main.
func TestMain(m *testing.M) {
	testscript.Main(m, map[string]func(){"logpretty": main})
}

func TestScript(t *testing.T) {
	testscript.Run(t, testscript.Params{
		Dir:                 "../../testdata/script",
		RequireExplicitExec: true,
		Setup:               isolateCoverage(t),
		Cmds:                map[string]func(*testscript.TestScript, bool, []string){"write": writeFile},
	})
}

// writeFile implements `write file text`, where text is the body of a Go string literal: a txtar file
// cannot hold a carriage return or end without a newline.
func writeFile(ts *testscript.TestScript, neg bool, args []string) {
	if neg || len(args) != 2 {
		ts.Fatalf("usage: write file text")
	}
	text, err := strconv.Unquote(`"` + args[1] + `"`)
	ts.Check(err)
	ts.Check(os.WriteFile(ts.MkAbs(args[0]), []byte(text), 0o644))
}

// isolateCoverage gives each script its own GOCOVERDIR and merges the counters into the shared one afterwards:
// parallel processes race on the covmeta temp file, whose name has no pid.
func isolateCoverage(t *testing.T) func(*testscript.Env) error {
	shared := os.Getenv("GOCOVERDIR")
	if shared == "" {
		return nil
	}
	root := t.TempDir()
	t.Cleanup(func() {
		counters, err := filepath.Glob(filepath.Join(root, "*", "covcounters.*"))
		if err != nil {
			t.Error(err)
		}
		// Catches a broken merge: without it the scripts' coverage would vanish with no test failing.
		if scripts, _ := filepath.Glob(filepath.Join(root, "*")); len(scripts) > 0 && len(counters) == 0 {
			t.Error("scripts ran under -cover but left no coverage counters")
		}
		for _, src := range counters {
			data, err := os.ReadFile(src)
			if err == nil {
				err = os.WriteFile(filepath.Join(shared, filepath.Base(src)), data, 0o644)
			}
			if err != nil {
				t.Errorf("merging coverage counters: %v", err)
			}
		}
	})
	return func(env *testscript.Env) error {
		dir, err := os.MkdirTemp(root, "script")
		if err != nil {
			return err
		}
		env.Setenv("GOCOVERDIR", dir)
		return nil
	}
}
