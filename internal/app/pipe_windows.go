package app

import "syscall"

// 232 is ERROR_NO_DATA, "The pipe is being closed"; the syscall package has no name for it.
var closedPipeErrs = []error{syscall.Errno(232), syscall.ERROR_BROKEN_PIPE}
