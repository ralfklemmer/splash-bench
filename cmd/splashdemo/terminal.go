//go:build darwin || linux

package main

import (
	"os"
	"syscall"
	"unsafe"
)

type winsize struct {
	rows, cols, xpixel, ypixel uint16
}

// terminalSize asks the terminal for its size; ok is false when f is not a
// terminal (pipe, file).
func terminalSize(f *os.File) (rows, cols int, ok bool) {
	var ws winsize
	_, _, errno := syscall.Syscall(syscall.SYS_IOCTL, f.Fd(), uintptr(syscall.TIOCGWINSZ), uintptr(unsafe.Pointer(&ws)))
	if errno != 0 || ws.rows == 0 {
		return 0, 0, false
	}
	return int(ws.rows), int(ws.cols), true
}
