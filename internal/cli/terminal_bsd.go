//go:build darwin || dragonfly || freebsd || netbsd || openbsd

package cli

import (
	"os"
	"syscall"
	"unsafe"
)

func fileIsTerminal(file *os.File) bool {
	var termios syscall.Termios
	_, _, errno := syscall.Syscall(syscall.SYS_IOCTL, file.Fd(), uintptr(syscall.TIOCGETA), uintptr(unsafe.Pointer(&termios)))
	return errno == 0
}
