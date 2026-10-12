package term

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"testing"
)

func TestParseColorMode(t *testing.T) {
	tests := []struct {
		in      string
		want    ColorMode
		wantErr bool
	}{
		{in: "auto", want: ColorAuto},
		{in: "always", want: ColorAlways},
		{in: "never", want: ColorNever},
		{in: "", wantErr: true},
		{in: "Always", wantErr: true},
		{in: "AUTO", wantErr: true},
		{in: "auto ", wantErr: true},
		{in: " never", wantErr: true},
		{in: "yes", wantErr: true},
		{in: "0", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			got, err := ParseColorMode(tt.in)

			if (err != nil) != tt.wantErr {
				t.Fatalf("ParseColorMode(%q) error = %v, want an error: %v", tt.in, err, tt.wantErr)
			}
			if got != tt.want {
				t.Errorf("ParseColorMode(%q) = %v, want %v", tt.in, got, tt.want)
			}
		})
	}
}

func TestColorModeString(t *testing.T) {
	for mode, want := range map[ColorMode]string{ColorAuto: "auto", ColorAlways: "always", ColorNever: "never", 9: "ColorMode(9)"} {
		if got := mode.String(); got != want {
			t.Errorf("ColorMode(%d).String() = %q, want %q", uint8(mode), got, want)
		}
	}
	// The names are the ones the command line takes.
	for _, mode := range []ColorMode{ColorAuto, ColorAlways, ColorNever} {
		if got, err := ParseColorMode(mode.String()); err != nil || got != mode {
			t.Errorf("ParseColorMode(%q) = %v, %v; want %v", mode.String(), got, err, mode)
		}
	}
}

func TestUseColorDecision(t *testing.T) {
	type env = map[string]string
	tests := []struct {
		name     string
		mode     ColorMode
		terminal bool
		env      env
		vtFails  bool
		want     bool
		wantVT   bool // whether the console was asked to interpret escape sequences
	}{
		{name: "auto on a terminal", mode: ColorAuto, terminal: true, want: true, wantVT: true},
		{name: "auto on a terminal with a usual TERM", mode: ColorAuto, terminal: true, env: env{"TERM": "xterm-256color"}, want: true, wantVT: true},
		{name: "auto into a pipe", mode: ColorAuto},
		{name: "auto into a pipe with a usual TERM", mode: ColorAuto, env: env{"TERM": "xterm-256color"}},
		{name: "auto with NO_COLOR", mode: ColorAuto, terminal: true, env: env{"NO_COLOR": "1"}},
		{name: "auto with any value of NO_COLOR", mode: ColorAuto, terminal: true, env: env{"NO_COLOR": "0"}},
		{name: "auto with an empty NO_COLOR", mode: ColorAuto, terminal: true, env: env{"NO_COLOR": ""}, want: true, wantVT: true},
		{name: "auto on a dumb terminal", mode: ColorAuto, terminal: true, env: env{"TERM": "dumb"}},
		{name: "auto on a console that shows no colors", mode: ColorAuto, terminal: true, vtFails: true, wantVT: true},

		{name: "always on a terminal", mode: ColorAlways, terminal: true, want: true, wantVT: true},
		{name: "always into a pipe", mode: ColorAlways, want: true, wantVT: true},
		{name: "always beats NO_COLOR and TERM", mode: ColorAlways, env: env{"NO_COLOR": "1", "TERM": "dumb"}, want: true, wantVT: true},
		{name: "always on a console that shows no colors", mode: ColorAlways, terminal: true, vtFails: true, want: true, wantVT: true},

		{name: "never on a terminal", mode: ColorNever, terminal: true},
		{name: "never into a pipe", mode: ColorNever},
		{name: "never with NO_COLOR on a dumb terminal", mode: ColorNever, terminal: true, env: env{"NO_COLOR": "1", "TERM": "dumb"}},
		{name: "a mode that does not exist", mode: 9, terminal: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			asked := false
			askConsole := func() bool {
				asked = true
				return !tt.vtFails
			}

			got := useColor(tt.mode, tt.terminal, func(key string) string { return tt.env[key] }, askConsole)

			if got != tt.want {
				t.Errorf("useColor = %v, want %v", got, tt.want)
			}
			if asked != tt.wantVT {
				t.Errorf("console asked to interpret escape sequences: %v, want %v", asked, tt.wantVT)
			}
		})
	}
}

func TestUseColor(t *testing.T) {
	noEnv := func(string) string { return "" }
	file, err := os.Create(filepath.Join(t.TempDir(), "out.log"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = file.Close() })
	pipeReader, pipeWriter, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = pipeReader.Close()
		_ = pipeWriter.Close()
	})

	outputs := map[string]io.Writer{"buffer": &bytes.Buffer{}, "file": file, "pipe": pipeWriter}
	for name, out := range outputs {
		t.Run(name, func(t *testing.T) {
			if UseColor(ColorAuto, out, noEnv) {
				t.Error("UseColor(ColorAuto) = true, want false: the output is not a terminal")
			}
			if !UseColor(ColorAlways, out, noEnv) {
				t.Error("UseColor(ColorAlways) = false, want true")
			}
			if UseColor(ColorNever, out, noEnv) {
				t.Error("UseColor(ColorNever) = true, want false")
			}
		})
	}
}
