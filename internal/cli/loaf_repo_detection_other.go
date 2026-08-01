//go:build !unix

package cli

import "os"

// openDetectionFile is the portable fallback for platforms with no O_NONBLOCK
// to open with. It settles the type on the path first, which is all Lstat and
// Stat can offer, and then re-checks the open descriptor — narrowing the window
// the unix build closes outright. The file types that make the window matter,
// FIFOs and device nodes, are not things a Windows project directory holds in
// its filesystem namespace, so what is left is the ordinary case.
func openDetectionFile(path string) (*os.File, bool) {
	if !isRegularFileForDetection(path) {
		return nil, false
	}
	file, err := os.Open(path)
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

// isRegularFileForDetection reports whether path is a regular file, resolving a
// symlink but never accepting anything else at either end.
func isRegularFileForDetection(path string) bool {
	link, err := os.Lstat(path)
	if err != nil {
		return false
	}
	if link.Mode()&os.ModeSymlink == 0 {
		return link.Mode().IsRegular()
	}
	info, err := os.Stat(path)
	return err == nil && info.Mode().IsRegular()
}
