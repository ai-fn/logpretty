// Package term decides whether the output is to be colored.
package term

import (
	"fmt"
	"io"

	xterm "golang.org/x/term"
)

// ColorMode is the user's choice of when to color the output.
type ColorMode uint8

// Color modes. ColorAuto colors only what a terminal will show.
const (
	ColorAuto ColorMode = iota
	ColorAlways
	ColorNever
)

func (m ColorMode) String() string {
	switch m {
	case ColorAuto:
		return "auto"
	case ColorAlways:
		return "always"
	case ColorNever:
		return "never"
	}
	return fmt.Sprintf("ColorMode(%d)", uint8(m))
}

// ParseColorMode reads the mode as it is written on the command line.
func ParseColorMode(s string) (ColorMode, error) {
	switch s {
	case "auto":
		return ColorAuto, nil
	case "always":
		return ColorAlways, nil
	case "never":
		return ColorNever, nil
	}
	return ColorAuto, fmt.Errorf("invalid color mode %q: want auto, always or never", s)
}

// UseColor reports whether to color what is written to out, which must be the file itself: a wrapper
// around it counts as not a terminal. On Windows it also turns on the console's support for escape sequences.
func UseColor(mode ColorMode, out io.Writer, getenv func(string) string) bool {
	f, ok := out.(interface{ Fd() uintptr })
	if !ok {
		return useColor(mode, false, getenv, func() bool { return false })
	}
	fd := f.Fd()
	return useColor(mode, xterm.IsTerminal(int(fd)), getenv, func() bool { return enableVT(fd) })
}

func useColor(mode ColorMode, terminal bool, getenv func(string) string, askConsole func() bool) bool {
	switch mode {
	case ColorAlways:
		// The user's word stands even if the console refuses; the output may well be going to a pager.
		askConsole()
		return true
	case ColorAuto:
		// An empty NO_COLOR counts as unset. The console is asked last, so that nothing changes it without need.
		return terminal && getenv("NO_COLOR") == "" && getenv("TERM") != "dumb" && askConsole()
	}
	return false
}
