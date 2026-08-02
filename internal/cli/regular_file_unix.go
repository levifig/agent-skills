//go:build unix

package cli

import (
	"errors"
	"io/fs"
	"os"
	"syscall"
)

// openRegularFile opens a path for reading and returns the descriptor only when
// the descriptor itself is a regular file.
//
// Both halves carry weight. O_NONBLOCK is what stops a FIFO from holding the
// open until somebody writes to it — the hang that would freeze `loaf install`
// and `loaf upgrade` against a directory nobody has vouched for yet. Settling
// the type on the descriptor rather than on the name is what closes the window
// between the two: a path that stats as a regular file can be swapped for a
// FIFO before the open lands, and the question worth answering is what is being
// read, not what was there a moment earlier. Regular files ignore O_NONBLOCK
// for reads, so a read through this descriptor behaves exactly as it would
// without it.
//
// Every read of a path Loaf did not write goes through here: the detector
// probes that decide whether this is a Loaf repo, and the project-file readers
// that run once they have decided. They ask the same question of the same
// untrusted paths, so they get their answer from one implementation.
func openRegularFile(path string) (*os.File, error) {
	return openRegularFileFlags(path, os.O_RDONLY|syscall.O_NONBLOCK)
}

// openRegularFileNoFollow is openRegularFile with O_NOFOLLOW so a symlink at
// the leaf is refused rather than resolved. Used for untrusted authored paths
// (skill sidecars) where following would read outside the repository.
func openRegularFileNoFollow(path string) (*os.File, error) {
	return openRegularFileFlags(path, os.O_RDONLY|syscall.O_NONBLOCK|syscall.O_NOFOLLOW)
}

func openRegularFileFlags(path string, flag int) (*os.File, error) {
	file, err := os.OpenFile(path, flag, 0)
	if err != nil {
		if isSymlinkOpenRefusal(err) {
			return nil, &fs.PathError{Op: "open", Path: path, Err: errNotRegularFile}
		}
		return nil, err
	}
	info, err := file.Stat()
	if err != nil {
		file.Close()
		return nil, err
	}
	if !info.Mode().IsRegular() {
		file.Close()
		return nil, &fs.PathError{Op: "open", Path: path, Err: errNotRegularFile}
	}
	return file, nil
}

// O_NOFOLLOW fails with ELOOP when the leaf is a symlink (Linux and macOS).
func isSymlinkOpenRefusal(err error) bool {
	var pathErr *fs.PathError
	if errors.As(err, &pathErr) {
		err = pathErr.Err
	}
	return errors.Is(err, syscall.ELOOP)
}
