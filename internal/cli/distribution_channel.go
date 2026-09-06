package cli

import (
	"os"
	"path/filepath"
)

// distribution_channel.go answers the question resolveInstalledDistributionRoot
// leaves open. That resolver finds *where* the running distribution lives; this
// one classifies *how* it got there, because replacing a Homebrew keg, a global
// npm package, and a source checkout are three different commands and Loaf can
// only name the right one if it knows which it is.
//
// Provenance is read from the resolved distribution path and its ancestors and
// used for exactly one thing: printing a command for a human to run. Nothing in
// Loaf acts on a channel, so every probe stays a cheap filesystem read and
// anything that does not match a known signature is installChannelUnknown,
// which is silent by construction — a wrong command is worse than none.

type installChannelKind int

const (
	installChannelUnknown installChannelKind = iota
	installChannelHomebrew
	installChannelNpm
	installChannelDev
	installChannelScript
)

// installScriptCommand is the one-liner README documents; re-running it moves
// a script install to the newest release and runs `loaf upgrade`.
const installScriptCommand = `bash -c "$(curl -fsSL https://raw.githubusercontent.com/levifig/loaf/main/install.sh)"`

func (kind installChannelKind) String() string {
	switch kind {
	case installChannelHomebrew:
		return "homebrew"
	case installChannelNpm:
		return "npm"
	case installChannelDev:
		return "dev"
	case installChannelScript:
		return "script"
	default:
		return "unknown"
	}
}

// installChannel pairs a resolved provenance with the one command that replaces
// this binary in place. The command is text to print, never text to execute.
type installChannel struct {
	Kind           installChannelKind
	UpgradeCommand string
}

// resolveInstallChannel classifies an installed-distribution root. The order is
// deliberate but rarely contested: a keg is not inside a node_modules tree, and
// an npm-linked checkout resolves through its symlink to the real checkout,
// where the git signature is the honest answer.
func resolveInstallChannel(distributionRoot string) installChannel {
	if formula, ok := homebrewKegFormula(distributionRoot); ok {
		return installChannel{Kind: installChannelHomebrew, UpgradeCommand: "brew upgrade " + formula}
	}
	if pkg, ok := npmGlobalPackage(distributionRoot); ok {
		return installChannel{Kind: installChannelNpm, UpgradeCommand: "npm update -g " + pkg}
	}
	if insideGitWorktree(distributionRoot) {
		return installChannel{Kind: installChannelDev, UpgradeCommand: "git pull && make build"}
	}
	if installedByScript(distributionRoot) {
		return installChannel{Kind: installChannelScript, UpgradeCommand: installScriptCommand}
	}
	return installChannel{}
}

// installedByScript recognizes install.sh's layout: the distribution sits at
// <LOAF_HOME>/releases/<version> and <LOAF_HOME>/current is a symlink to it.
// Both halves are required so an unpacked archive somewhere else stays unknown.
func installedByScript(distributionRoot string) bool {
	releases := filepath.Dir(distributionRoot)
	if filepath.Base(releases) != "releases" {
		return false
	}
	current, err := os.Readlink(filepath.Join(filepath.Dir(releases), "current"))
	if err != nil {
		return false
	}
	if !filepath.IsAbs(current) {
		current = filepath.Join(filepath.Dir(releases), current)
	}
	return filepath.Clean(current) == filepath.Clean(distributionRoot)
}

// homebrewKegFormula requires both halves of the keg signature: the
// <prefix>/Cellar/<formula>/<version> path pattern and the INSTALL_RECEIPT.json
// Homebrew writes at the keg root. Loaf's payload sits under libexec/, so the
// walk starts at the distribution root and climbs to the keg. The formula name
// comes from the path rather than a constant, so a tapped or versioned formula
// still prints a command that works.
func homebrewKegFormula(distributionRoot string) (string, bool) {
	for _, dir := range ancestorDirs(distributionRoot) {
		if !channelPathExists(filepath.Join(dir, "INSTALL_RECEIPT.json")) {
			continue
		}
		formulaDir := filepath.Dir(dir)
		if filepath.Base(filepath.Dir(formulaDir)) != "Cellar" {
			return "", false
		}
		return filepath.Base(formulaDir), true
	}
	return "", false
}

// npmGlobalPackage recognizes a global npm install and, just as importantly,
// declines a project-local one. `lib/node_modules` is the global layout on
// every Unix Node distribution; a prefix root that carries a bin/ but no
// package.json covers the flatter layout npm uses elsewhere. A project's own
// node_modules fails both tests — its parent is a package — so a locally
// installed copy stays unknown instead of printing `npm update -g`, which would
// not touch it. The package name is read from the distribution's own
// package.json so a scoped or renamed publication stays correct.
func npmGlobalPackage(distributionRoot string) (string, bool) {
	for _, dir := range ancestorDirs(distributionRoot) {
		if filepath.Base(dir) != "node_modules" {
			continue
		}
		if !isGlobalNodeModules(dir) {
			return "", false
		}
		name := packageName(distributionRoot)
		if name == "" {
			return "", false
		}
		return name, true
	}
	return "", false
}

func isGlobalNodeModules(dir string) bool {
	parent := filepath.Dir(dir)
	if filepath.Base(parent) == "lib" {
		return true
	}
	return isDir(filepath.Join(parent, "bin")) && !channelPathExists(filepath.Join(parent, "package.json"))
}

// insideGitWorktree reports whether the distribution is a checkout someone
// pulls and rebuilds. `.git` is a directory in an ordinary clone and a file in
// a linked worktree or submodule, so existence is the signal and type is not.
func insideGitWorktree(distributionRoot string) bool {
	for _, dir := range ancestorDirs(distributionRoot) {
		if channelPathExists(filepath.Join(dir, ".git")) {
			return true
		}
	}
	return false
}

// ancestorDirs lists the path and every parent up to the filesystem root.
func ancestorDirs(path string) []string {
	clean, err := filepath.Abs(path)
	if err != nil {
		return nil
	}
	var dirs []string
	for {
		dirs = append(dirs, clean)
		parent := filepath.Dir(clean)
		if parent == clean {
			return dirs
		}
		clean = parent
	}
}

func channelPathExists(path string) bool {
	_, err := os.Lstat(path)
	return err == nil
}
