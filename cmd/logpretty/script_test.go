package main

import (
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
	})
}
