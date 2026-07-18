package cli

import (
	"fmt"
	"os"
	"path/filepath"
)

// Loaf resolves three distinct roots that must never be conflated:
//
//   - The project root (project.ResolveRoot / state.Runtime) is where the
//     user's project state lives. It stays working-directory derived.
//   - The installed-distribution root is the Loaf package tree that ships
//     with the running executable. Identity-bearing commands — version,
//     install (including --upgrade), config check maintenance, and doctor's
//     reported CLI version — resolve it from executable provenance only.
//   - The source-checkout root is the checkout a development command operates
//     on. build and setup resolve it from the working directory and project
//     root, exactly as before.

// resolveInstalledDistributionRoot locates the Loaf package tree that ships
// with the running executable. Release archives place the binary at
// <root>/bin/loaf; npm installs and source checkouts place it at
// <root>/bin/native/<target>/loaf; Homebrew exposes it through a bin symlink
// into the Cellar tree. Evaluating symlinks and walking upward from the
// executable reaches the adjacent package.json named "loaf" in all of those
// layouts. The working directory and project root are deliberately never
// consulted: distribution authority comes from the binary being run, not from
// where it is invoked, so an installed executable inside an older checkout can
// never adopt that checkout as its distribution. Local development needs no
// special mode — running a checkout's own binary makes that checkout the
// distribution root by construction.
func (r Runner) resolveInstalledDistributionRoot() (string, error) {
	path, err := r.executablePath()
	if err != nil {
		return "", fmt.Errorf("resolve loaf executable: %w", err)
	}
	if resolved, err := filepath.EvalSymlinks(path); err == nil {
		path = resolved
	}
	if root, ok := findLoafPackageRoot(filepath.Dir(path), map[string]bool{}); ok {
		return root, nil
	}
	return "", fmt.Errorf("no Loaf distribution found alongside %s; reinstall Loaf, or run a source checkout's own bin/loaf for local development", path)
}

func (r Runner) executablePath() (string, error) {
	if r.Executable != nil {
		return r.Executable()
	}
	return os.Executable()
}

// resolveSourceCheckoutRoot locates the Loaf source checkout a development
// command operates on, searching the supplied working/project paths first and
// falling back to the executable's own tree. Only build and setup may use
// this: they act on the checkout the caller is standing in. Identity-bearing
// commands must use resolveInstalledDistributionRoot instead.
func resolveSourceCheckoutRoot(paths ...string) (string, error) {
	seen := map[string]bool{}
	for _, path := range paths {
		if root, ok := findLoafPackageRoot(path, seen); ok {
			return root, nil
		}
	}
	if executable, err := os.Executable(); err == nil {
		if root, ok := findLoafPackageRoot(filepath.Dir(executable), seen); ok {
			return root, nil
		}
	}
	if cwd, err := os.Getwd(); err == nil {
		if root, ok := findLoafPackageRoot(cwd, seen); ok {
			return root, nil
		}
	}
	return "", fmt.Errorf("could not find loaf package root")
}
