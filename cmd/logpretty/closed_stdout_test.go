package main

import (
	"bytes"
	"context"
	"errors"
	"os/exec"
	"runtime"
	"strings"
	"syscall"
	"testing"
	"time"
)

// The reader of stdout leaves early, as in `logpretty | head`: the program must stop without complaining.
func TestClosedStdout(t *testing.T) {
	bin, err := exec.LookPath("logpretty")
	if err != nil {
		t.Fatalf("logpretty is not on PATH (TestMain installs it through testscript.Main): %v", err)
	}
	// The timeout only turns a hung child into a failure instead of a stuck test run.
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, bin)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		cancel()
		_ = cmd.Wait()
	})
	if err := stdout.Close(); err != nil {
		t.Fatal(err)
	}

	// Feed lines until the child is gone; the cap only keeps a broken child from being fed forever.
	chunk := []byte(strings.Repeat("some log line\n", 4096))
	stopped := false
	for range 1024 {
		if _, err := stdin.Write(chunk); err != nil {
			stopped = true
			break
		}
	}
	if !stopped {
		t.Fatalf("the child kept reading %d MiB after its stdout was closed", 1024*len(chunk)>>20)
	}
	_ = stdin.Close()
	err = cmd.Wait()

	if runtime.GOOS == "windows" {
		if err != nil {
			t.Errorf("exit = %v, want success", err)
		}
	} else {
		// On Unix the Go runtime turns a failed write to stdout into death by SIGPIPE, like any filter.
		var exitErr *exec.ExitError
		if !errors.As(err, &exitErr) {
			t.Fatalf("exit = %v, want death by SIGPIPE", err)
		}
		if ws, ok := exitErr.Sys().(syscall.WaitStatus); !ok || !ws.Signaled() || ws.Signal() != syscall.SIGPIPE {
			t.Errorf("exit = %v, want death by SIGPIPE", err)
		}
	}
	if stderr.Len() != 0 {
		t.Errorf("stderr = %q, want empty", stderr.String())
	}
}
