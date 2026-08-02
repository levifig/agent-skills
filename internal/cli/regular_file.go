package cli

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
)

// projectFileReadLimit bounds a whole-file read of something in the project
// tree. The files behind it are markdown and JSON that Loaf reads in order to
// rewrite them, so the ceiling only has to clear what a person would plausibly
// have written, and 4 MiB clears it by a wide margin. The limit is there for
// the other case — a path that is not the document it claims to be — where
// reading to EOF means reading until memory runs out.
const projectFileReadLimit = 4 << 20

// The two ways a project path can be unfit to read whole. They stay apart
// because they are different statements about the file: one is not a document
// at all, the other is more of one than Loaf will take in.
var (
	errNotRegularFile = errors.New("not a regular file")
	errFileTooLarge   = errors.New("file exceeds the project read limit")
)

// readRegularFile reads a project file whole through the descriptor-hardened
// open, and refuses rather than blocks or truncates.
//
// Refusing on the limit is the part worth stating: returning the first limit
// bytes would hand the caller a prefix that looks like the file, and these
// callers rewrite what they read. A managed section is found by scanning the
// content, so a truncated read could miss a fence that is really there and
// append a second one, or hash a body it only partly saw. Detection can afford
// a prefix because it answers yes or no; a rewrite cannot.
//
// Symlinks to regular files are followed: repo detection and install/doctor
// paths treat "AGENTS.md → another real file" as a normal layout. Untrusted
// authored paths that must not escape the tree (skill sidecars) use
// readRegularFileNoFollow instead.
func readRegularFile(path string, limit int64) ([]byte, error) {
	return readOpenedRegularFile(path, limit, openRegularFile)
}

// readRegularFileNoFollow is readRegularFile without following a symlink at the
// target path. Use it when the path itself is untrusted authored content and
// following would let a checkout point the read outside the repository.
func readRegularFileNoFollow(path string, limit int64) ([]byte, error) {
	return readOpenedRegularFile(path, limit, openRegularFileNoFollow)
}

func readOpenedRegularFile(path string, limit int64, open func(string) (*os.File, error)) ([]byte, error) {
	file, err := open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	body, err := io.ReadAll(io.LimitReader(file, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(body)) > limit {
		return nil, &fs.PathError{Op: "read", Path: path, Err: fmt.Errorf("%w of %d bytes", errFileTooLarge, limit)}
	}
	return body, nil
}

// isProjectFileRefusal reports whether a read failed because of what the path
// is rather than because of what the filesystem did.
func isProjectFileRefusal(err error) bool {
	return errors.Is(err, errNotRegularFile) || errors.Is(err, errFileTooLarge)
}

// refuseProjectFileRead phrases a failed project-file read for a caller that
// was about to write the file. A refusal reads like the other refusals that
// path already reports — the malformed fingerprint, the tampered body — because
// it is the same decision: Loaf will not replace a file it has not read.
func refuseProjectFileRead(err error) error {
	if isProjectFileRefusal(err) {
		return fmt.Errorf("%w; refusing to overwrite", err)
	}
	return err
}
