package render

import (
	"reflect"
	"slices"
	"testing"
	"time"

	"github.com/ai-fn/logpretty/internal/entry"
)

// The codes are pinned: they are what a user sees, and a typo in one would still look like a style in a test.
func TestDefaultTheme(t *testing.T) {
	const (
		faint = "\x1b[2m"
		reset = "\x1b[0m"
	)
	want := Theme{
		Time: faint,
		Level: [...]string{
			entry.LevelTrace: faint,
			entry.LevelDebug: "\x1b[36m",
			entry.LevelInfo:  "\x1b[32m",
			entry.LevelWarn:  "\x1b[33m",
			entry.LevelError: "\x1b[31m",
			entry.LevelFatal: "\x1b[1;31m",
		},
		Key:    faint,
		Prefix: faint,
		Marker: "\x1b[33m",
		Reset:  reset,
		Traces: []string{
			"\x1b[38;5;168m", "\x1b[38;5;130m", "\x1b[38;5;31m", "\x1b[38;5;33m", "\x1b[38;5;27m", "\x1b[38;5;104m",
			"\x1b[38;5;99m", "\x1b[38;5;171m", "\x1b[38;5;128m", "\x1b[38;5;133m", "\x1b[38;5;201m", "\x1b[38;5;162m",
		},
	}

	if got := DefaultTheme(); !reflect.DeepEqual(got, want) {
		t.Errorf("DefaultTheme() = %q, want %q", got, want)
	}
	// Blue and magenta: cyan and green are the levels debug and info.
	if got, want := BasicTracePalette(), []string{"\x1b[34m", "\x1b[35m", "\x1b[94m", "\x1b[95m"}; !slices.Equal(got, want) {
		t.Errorf("BasicTracePalette() = %q, want %q", got, want)
	}
}

func TestDefaultThemeInOutput(t *testing.T) {
	r := Renderer{Theme: DefaultTheme(), Loc: time.UTC}

	got := rendered(t, &r, `web-1 | {"time":"2026-10-11T12:00:00Z","level":"warn","msg":"slow request","ms":120}`)
	want := "\x1b[2mweb-1 |\x1b[0m \x1b[2m12:00:00.000\x1b[0m \x1b[33mWARN\x1b[0m  slow request  \x1b[2mms=\x1b[0m120"
	if got != want {
		t.Errorf("entry\n got %q\nwant %q", got, want)
	}

	cut := string(r.AppendTruncated(nil, []byte("abc"), 5))
	if want := "abc\x1b[33m…[+5B]\x1b[0m\n"; cut != want {
		t.Errorf("truncated line\n got %q\nwant %q", cut, want)
	}
}

// A theme may style one part only: each part is closed by its own style, not by that of a neighbor.
func TestThemeWithOnePartStyled(t *testing.T) {
	const line = `web-1 | {"time":"2026-10-11T12:00:00Z","level":"warn","msg":"m","a":1}`
	tests := []struct {
		name    string
		theme   Theme
		want    string
		wantCut string
	}{
		{name: "none", theme: Theme{}, want: "web-1 | 12:00:00.000 WARN  m  a=1", wantCut: "abc…[+5B]"},
		{name: "time", theme: Theme{Time: "<s>"}, want: "web-1 | <s>12:00:00.000</> WARN  m  a=1", wantCut: "abc…[+5B]"},
		{name: "prefix", theme: Theme{Prefix: "<s>"}, want: "<s>web-1 |</> 12:00:00.000 WARN  m  a=1", wantCut: "abc…[+5B]"},
		{
			name:    "level",
			theme:   Theme{Level: [entry.LevelFatal + 1]string{entry.LevelWarn: "<s>"}},
			want:    "web-1 | 12:00:00.000 <s>WARN</>  m  a=1",
			wantCut: "abc…[+5B]",
		},
		{name: "key", theme: Theme{Key: "<s>"}, want: "web-1 | 12:00:00.000 WARN  m  <s>a=</>1", wantCut: "abc…[+5B]"},
		{name: "marker", theme: Theme{Marker: "<s>"}, want: "web-1 | 12:00:00.000 WARN  m  a=1", wantCut: "abc<s>…[+5B]</>"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.theme.Reset = "</>"
			r := Renderer{Theme: tt.theme, Loc: time.UTC}

			if got := rendered(t, &r, line); got != tt.want {
				t.Errorf("entry\n got %q\nwant %q", got, tt.want)
			}
			if got := string(r.AppendTruncated(nil, []byte("abc"), 5)); got != tt.wantCut+"\n" {
				t.Errorf("truncated line\n got %q\nwant %q", got, tt.wantCut+"\n")
			}
		})
	}
}
