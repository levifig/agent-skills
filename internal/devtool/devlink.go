package devtool

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
)

// DevLinkStatus is the outcome of a launcher-pointer refresh.
type DevLinkStatus string

const (
	DevLinkLinked   DevLinkStatus = "linked"
	DevLinkConflict DevLinkStatus = "conflict"
	DevLinkSkipped  DevLinkStatus = "skipped"
	DevLinkFailed   DevLinkStatus = "failed"
)

// DevLinkResult names what happened and where.
type DevLinkResult struct {
	Status  DevLinkStatus
	Link    string
	Pointer string
	Err     error
}

// DevLinkOptions parameterize RefreshDevBuildLink; tests set Home and
// DataHome, production reads them from Env.
type DevLinkOptions struct {
	Env      Env
	Home     string
	DataHome string
	Warn     func(string)
	// BeforeClaimPublic runs between observing the PATH name and creating it;
	// tests use it to inject the race the create-exclusive claim guards against.
	BeforeClaimPublic func()
	// Symlink and MkdirAll default to the os functions; tests inject failures.
	Symlink  func(target, link string) error
	MkdirAll func(path string, perm os.FileMode) error
}

// RefreshDevBuildLink makes the last successful dev build the active CLI
// (ADR-026): it retargets $XDG_DATA_HOME/loaf/current-dev-launcher at the
// checkout's bin/loaf and creates ~/.local/bin/loaf only when that name is
// absent, as a symlink to the pointer. A real file, directory, or any other
// symlink at the PATH name is never replaced, and failures warn rather than
// fail the build that called this.
func RefreshDevBuildLink(launcher string, options DevLinkOptions) DevLinkResult {
	warn := options.Warn
	if warn == nil {
		warn = func(string) {}
	}
	symlink := options.Symlink
	if symlink == nil {
		symlink = os.Symlink
	}
	mkdirAll := options.MkdirAll
	if mkdirAll == nil {
		mkdirAll = os.MkdirAll
	}
	home := options.Home
	if home == "" {
		home = options.Env["HOME"]
	}
	if home == "" {
		home, _ = os.UserHomeDir()
	}
	dataHome := options.DataHome
	if dataHome == "" {
		if xdg := options.Env["XDG_DATA_HOME"]; xdg != "" && filepath.IsAbs(xdg) {
			dataHome = xdg
		} else {
			dataHome = filepath.Join(home, ".local", "share")
		}
	}
	publicLink := filepath.Join(home, ".local", "bin", "loaf")
	pointer := filepath.Join(dataHome, "loaf", "current-dev-launcher")
	result := DevLinkResult{Link: publicLink, Pointer: pointer}

	if runtime.GOOS == "windows" {
		result.Status = DevLinkSkipped
		return result
	}
	fail := func(err error) DevLinkResult {
		warn(fmt.Sprintf("WARN: failed to link latest dev build (%v)", err))
		result.Status = DevLinkFailed
		result.Err = err
		return result
	}

	// Publish the pointer atomically: a temporary symlink renamed over the name.
	if err := mkdirAll(filepath.Dir(pointer), 0o755); err != nil {
		return fail(err)
	}
	if info, err := os.Lstat(pointer); err == nil && info.Mode()&os.ModeSymlink == 0 {
		warn(fmt.Sprintf("WARN: not linking latest dev build; %s is not a symlink", pointer))
		result.Status = DevLinkConflict
		return result
	} else if err != nil && !errors.Is(err, os.ErrNotExist) {
		return fail(err)
	}
	absLauncher, err := filepath.Abs(launcher)
	if err != nil {
		return fail(err)
	}
	temporary := fmt.Sprintf("%s.tmp-%d", pointer, os.Getpid())
	_ = os.Remove(temporary)
	if err := symlink(absLauncher, temporary); err != nil {
		return fail(err)
	}
	if err := os.Rename(temporary, pointer); err != nil {
		_ = os.Remove(temporary)
		return fail(err)
	}

	if err := mkdirAll(filepath.Dir(publicLink), 0o755); err != nil {
		return fail(err)
	}
	if options.BeforeClaimPublic != nil {
		options.BeforeClaimPublic()
	}
	if err := symlink(pointer, publicLink); err != nil {
		if !errors.Is(err, os.ErrExist) {
			return fail(err)
		}
		if !publicPointsAt(publicLink, pointer) {
			warn(publicConflictWarning(publicLink, pointer))
			result.Status = DevLinkConflict
			return result
		}
	}
	result.Status = DevLinkLinked
	return result
}

func publicPointsAt(publicLink, pointer string) bool {
	info, err := os.Lstat(publicLink)
	if err != nil || info.Mode()&os.ModeSymlink == 0 {
		return false
	}
	target, err := os.Readlink(publicLink)
	if err != nil {
		return false
	}
	if !filepath.IsAbs(target) {
		target = filepath.Join(filepath.Dir(publicLink), target)
	}
	return filepath.Clean(target) == filepath.Clean(pointer)
}

func publicConflictWarning(publicLink, pointer string) string {
	info, err := os.Lstat(publicLink)
	if err != nil || info.Mode()&os.ModeSymlink == 0 {
		return fmt.Sprintf("WARN: not linking latest dev build; %s is not a symlink", publicLink)
	}
	if isLoafCheckoutLink(publicLink) {
		return fmt.Sprintf("WARN: not linking latest dev build; %s already points at a Loaf checkout and will not be replaced. Remove it and rebuild to install the last-build pointer at %s", publicLink, pointer)
	}
	return fmt.Sprintf("WARN: not linking latest dev build; %s is not Loaf's launcher pointer", publicLink)
}

// isLoafCheckoutLink recognizes the pre-pointer scheme: a PATH symlink straight
// at some checkout's bin/loaf, identified by the package.json beside bin/.
func isLoafCheckoutLink(link string) bool {
	target, err := os.Readlink(link)
	if err != nil {
		return false
	}
	if !filepath.IsAbs(target) {
		target = filepath.Join(filepath.Dir(link), target)
	}
	packageRoot := filepath.Dir(filepath.Dir(target))
	body, err := os.ReadFile(filepath.Join(packageRoot, "package.json"))
	if err != nil {
		return false
	}
	var manifest struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal(body, &manifest); err != nil {
		return false
	}
	return manifest.Name == "loaf" && filepath.Clean(target) == filepath.Join(packageRoot, "bin", "loaf")
}
