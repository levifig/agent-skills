//go:build !darwin && !dragonfly && !freebsd && !netbsd && !openbsd && !linux && !windows

package cli

import "os"

func fileIsTerminal(_ *os.File) bool {
	return false
}
