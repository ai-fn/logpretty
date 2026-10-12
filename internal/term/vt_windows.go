package term

import "golang.org/x/sys/windows"

// enableVT makes the console interpret escape sequences: the old console host prints them as text
// until it is told to. It reports false for a handle that is not a console, or one that refuses.
func enableVT(fd uintptr) bool {
	handle := windows.Handle(fd)
	var mode uint32
	if err := windows.GetConsoleMode(handle, &mode); err != nil {
		return false
	}
	if mode&windows.ENABLE_VIRTUAL_TERMINAL_PROCESSING != 0 {
		return true
	}
	return windows.SetConsoleMode(handle, mode|windows.ENABLE_VIRTUAL_TERMINAL_PROCESSING) == nil
}
