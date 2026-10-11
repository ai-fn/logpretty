//go:build unix

package main

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os/exec"
	"syscall"
	"testing"
	"time"
)

func TestSignalExitCode(t *testing.T) {
	tests := []struct {
		name string
		sig  syscall.Signal
		want int
	}{
		{name: "SIGINT", sig: syscall.SIGINT, want: 130},
		{name: "SIGTERM", sig: syscall.SIGTERM, want: 143},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
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
			// Reaps the child on every exit path; after a normal Wait this second call is a harmless error.
			t.Cleanup(func() {
				cancel()
				_ = cmd.Wait()
			})

			if _, err := io.WriteString(stdin, "a\nb"); err != nil {
				t.Fatal(err)
			}
			// Seeing the first line means the child handles signals and waits for the rest of "b".
			first := make([]byte, len("a\n"))
			if _, err := io.ReadFull(stdout, first); err != nil {
				t.Fatalf("reading the first line: %v", err)
			}
			if err := cmd.Process.Signal(tt.sig); err != nil {
				t.Fatal(err)
			}
			rest, err := io.ReadAll(stdout)
			if err != nil {
				t.Fatal(err)
			}

			var exitErr *exec.ExitError
			if err := cmd.Wait(); !errors.As(err, &exitErr) || exitErr.ExitCode() != tt.want {
				t.Errorf("exit = %v, want exit status %d", err, tt.want)
			}
			if got := string(first) + string(rest); got != "a\n" {
				t.Errorf("stdout = %q, want only the complete line %q", got, "a\n")
			}
			if stderr.Len() != 0 {
				t.Errorf("stderr = %q, want empty", stderr.String())
			}
		})
	}
}
