//go:build windows

package cli

import (
	"os"
	"syscall"
)

func fileIsTerminal(file *os.File) bool {
	var mode uint32
	return syscall.GetConsoleMode(syscall.Handle(file.Fd()), &mode) == nil
}
