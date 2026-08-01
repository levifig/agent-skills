//go:build !unix

package cli

import (
	"io/fs"
	"os"
)

// openRegularFile is the portable fallback for platforms with no O_NONBLOCK to
// open with. It settles the type on the path first, which is all Lstat and Stat
// can offer, and then re-checks the open descriptor — narrowing the window the
// unix build closes outright. The file types that make the window matter, FIFOs
// and device nodes, are not things a Windows project directory holds in its
// filesystem namespace, so what is left is the ordinary case.
func openRegularFile(path string) (*os.File, error) {
	if err := checkRegularFilePath(path); err != nil {
		return nil, err
	}
	file, err := os.Open(path)
	if err != nil {
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

// checkRegularFilePath settles the file type on the path, resolving a symlink
// but never accepting anything but a regular file at either end.
func checkRegularFilePath(path string) error {
	link, err := os.Lstat(path)
	if err != nil {
		return err
	}
	info := link
	if link.Mode()&os.ModeSymlink != 0 {
		if info, err = os.Stat(path); err != nil {
			return err
		}
	}
	if !info.Mode().IsRegular() {
		return &fs.PathError{Op: "open", Path: path, Err: errNotRegularFile}
	}
	return nil
}
