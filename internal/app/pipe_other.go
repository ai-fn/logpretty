//go:build !windows

package app

import "syscall"

// Reached only when stdout is not file descriptor 1: for the real stdout the Go runtime raises SIGPIPE first.
var closedPipeErrs = []error{syscall.EPIPE}
