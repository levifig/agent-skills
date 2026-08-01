//go:build unix

package cli

import (
	"os"
	"syscall"
)

// openDetectionFile opens a probe path for reading and returns the descriptor
// only when the descriptor itself is a regular file.
//
// Both halves carry weight. O_NONBLOCK is what stops a FIFO from holding the
// open until somebody writes to it — the hang that would freeze `loaf install`
// and `loaf upgrade` against a directory nobody has vouched for yet. Settling
// the type on the descriptor rather than on the name is what closes the window
// between the two: a path that stats as a regular file can be swapped for a
// FIFO before the open lands, and the question worth answering is what is being
// read, not what was there a moment earlier. Regular files ignore O_NONBLOCK
// for reads, so the capped read behaves exactly as it would without it.
func openDetectionFile(path string) (*os.File, bool) {
	file, err := os.OpenFile(path, os.O_RDONLY|syscall.O_NONBLOCK, 0)
	if err != nil {
		return nil, false
	}
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() {
		file.Close()
		return nil, false
	}
	return file, true
}
