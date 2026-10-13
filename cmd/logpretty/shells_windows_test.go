package main

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const (
	cmdExe      = "cmd"
	powershell5 = "powershell" // Windows PowerShell 5.1
	powershell7 = "pwsh"
)

// The README says how to write the arguments in each shell of Windows. These are its commands, run by the
// shells themselves: what reaches the program is decided by them, not by Go.
func TestShellQuoting(t *testing.T) {
	const (
		entries = `{"time":"2026-10-11T12:00:01Z","level":"warn","msg":"slow request","user_id":42,"latency":"1.2s"}` + "\n" +
			`{"time":"2026-10-11T12:00:02Z","level":"info","msg":"ok","user_id":"042","latency":"3ms"}` + "\n"
		slow = "12:00:01.000 WARN  slow request  user_id=42 latency=1.2s\n"
		ok   = "12:00:02.000 INFO  ok  user_id=042 latency=3ms\n"
		both = slow + ok
	)

	tests := []struct {
		name     string
		shells   []string
		command  string
		input    string // the entries above, if empty
		reads    bool   // the command reads app.log by itself
		want     string
		wantCode int
	}{
		{name: "double quotes", shells: []string{cmdExe}, command: `logpretty --utc --where "user_id=42"`, want: both},
		{name: "quote inside double quotes", shells: []string{cmdExe}, command: `logpretty --utc --where "msg=\"slow request\""`, want: slow},
		{name: "quoted number is text", shells: []string{cmdExe}, command: `logpretty --utc --where "user_id=\"042\""`, want: ok},
		{name: "characters of the shell inside double quotes", shells: []string{cmdExe}, command: `logpretty --utc --where "msg~^slow|<&>"`, want: slow},
		// Between a pair of \" the shell reads its own characters, so each one needs a caret there.
		{name: "characters of the shell inside an escaped quote", shells: []string{cmdExe}, command: `logpretty --utc --where "msg~\"^^slow request^|failed\""`, want: slow},
		// Here a caret that was lost would let the second entry through.
		{name: "caret inside an escaped quote", shells: []string{cmdExe}, command: `logpretty --utc --where "msg~\"^^slow request^|^^k\""`, want: slow},
		{name: "single quotes are not quotes", shells: []string{cmdExe}, command: `logpretty --utc --where 'user_id=42'`, wantCode: 2},
		{name: "a file read with < keeps its bytes", shells: []string{cmdExe}, command: `logpretty`, input: "{\"msg\":\"café\"}\n", want: "café\n"},

		{name: "single quotes", shells: []string{powershell5, powershell7}, command: `logpretty --utc --where 'user_id=42'`, want: both},
		// A list written with no space after its comma reaches a program as it is written.
		{
			name: "comma", shells: []string{powershell5, powershell7}, command: `logpretty --utc --fields msg,latency`,
			want: "12:00:01.000 WARN  slow request  latency=1.2s\n12:00:02.000 INFO  ok  latency=3ms\n",
		},
		{
			name: "a file read through cmd keeps its bytes", shells: []string{powershell5, powershell7}, command: `cmd /c "logpretty < app.log"`,
			input: "{\"msg\":\"café\"}\n", reads: true, want: "café\n",
		},

		{name: "quote inside single quotes", shells: []string{powershell7}, command: `logpretty --utc --where 'msg="slow request"'`, want: slow},
		{name: "quoted number is text", shells: []string{powershell7}, command: `logpretty --utc --where 'user_id="042"'`, want: ok},

		// Windows PowerShell passes the quotes inside an argument unescaped, so the program takes them for
		// the quotes of its command line and drops them: this is why the README asks for --% there.
		{name: "bare quotes are dropped", shells: []string{powershell5}, command: `logpretty --utc --where 'user_id="042"'`, want: both},
		{name: "bare quotes are dropped from a value with a space", shells: []string{powershell5}, command: `logpretty --utc --where 'msg="slow request"'`, wantCode: 2},
		// After --% the shell passes the rest of the line as it is written, so it is written as for cmd.exe.
		{name: "stop-parsing token", shells: []string{powershell5}, command: `logpretty --% --utc --where "msg=\"slow request\""`, want: slow},
		{name: "stop-parsing token keeps a number text", shells: []string{powershell5}, command: `logpretty --% --utc --where "user_id=\"042\""`, want: ok},
		// Its pipe to a program is ASCII: each of the two bytes of the letter comes out as a question mark.
		{name: "a pipe loses what is not ASCII", shells: []string{powershell5}, command: `logpretty`, input: "{\"msg\":\"café\"}\n", want: "caf??\n"},
	}
	for _, tt := range tests {
		for _, shell := range tt.shells {
			t.Run(shell+"/"+tt.name, func(t *testing.T) {
				input := tt.input
				if input == "" {
					input = entries
				}
				// A command that fails on its arguments leaves before it reads, so it is given nothing to read.
				feed := tt.wantCode == 0 && !tt.reads
				stdout, stderr, code := runInShell(t, shell, tt.command, input, feed)

				if code != tt.wantCode {
					t.Fatalf("%s\nexit code %d, want %d\nstdout: %q\nstderr: %q", tt.command, code, tt.wantCode, stdout, stderr)
				}
				if stdout != tt.want {
					t.Errorf("%s\nstdout: %q\n  want: %q", tt.command, stdout, tt.want)
				}
				// A shell may wrap what a program writes to stderr, so only its presence is checked.
				if failed := tt.wantCode != 0; failed != strings.Contains(stderr, "logpretty: ") {
					t.Errorf("%s\nstderr: %q, want a message of logpretty: %v", tt.command, stderr, failed)
				}
			})
		}
	}
}

// runInShell gives the shell a script with the command, so that no argument passes through the quoting of
// Go, and returns what came out with the line ends of the shell undone. The input is in app.log beside the
// script; with feed, the shell hands it to the command.
func runInShell(t *testing.T, shell, command, input string, feed bool) (stdout, stderr string, code int) {
	t.Helper()
	path, err := exec.LookPath(shell)
	if err != nil {
		if os.Getenv("CI") != "" {
			t.Fatalf("%s is not on PATH: %v", shell, err)
		}
		t.Skipf("%s is not on PATH: %v", shell, err)
	}

	dir := t.TempDir()
	script, text := `.\run.ps1`, command+"\r\nexit $LASTEXITCODE\r\n"
	args := []string{"-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass", "-File", script}
	if feed {
		text = "Get-Content app.log | " + text
	}
	if shell == cmdExe {
		redirect := ""
		if feed {
			redirect = " < app.log"
		}
		script, text = `.\run.cmd`, "@echo off\r\n"+command+redirect+"\r\nexit /b %ERRORLEVEL%\r\n"
		args = []string{"/d", "/c", script}
	}
	for name, data := range map[string]string{"app.log": input, script: text} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(data), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	// The timeout only turns a hung shell into a failure instead of a stuck test run.
	ctx, cancel := context.WithTimeout(t.Context(), time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, path, args...)
	cmd.Dir = dir
	// The program may outlive a shell that was killed, and would keep the pipes of its output open.
	cmd.WaitDelay = 10 * time.Second
	if os.Getenv("GOCOVERDIR") != "" {
		// Runs of the binary must not share a coverage directory with the scripts running beside them.
		cmd.Env = append(os.Environ(), "GOCOVERDIR="+t.TempDir())
	}
	var out, errOut bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errOut

	err = cmd.Run()
	var exit *exec.ExitError
	if err != nil && !errors.As(err, &exit) {
		t.Fatalf("%s %s: %v", shell, script, err)
	}
	undo := strings.NewReplacer("\r\n", "\n")
	return undo.Replace(out.String()), undo.Replace(errOut.String()), cmd.ProcessState.ExitCode()
}
