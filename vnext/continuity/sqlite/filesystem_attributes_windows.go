//go:build windows

package sqlite

import (
	"fmt"
	"os"
	"syscall"
)

func windowsReparsePoint(info os.FileInfo) (bool, error) {
	attributes, ok := info.Sys().(*syscall.Win32FileAttributeData)
	if !ok {
		return false, fmt.Errorf("Windows file attributes are unavailable")
	}
	return attributes.FileAttributes&syscall.FILE_ATTRIBUTE_REPARSE_POINT != 0, nil
}
