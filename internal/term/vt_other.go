//go:build !windows

package term

// Outside Windows a terminal needs no setup to interpret escape sequences.
func enableVT(uintptr) bool { return true }
