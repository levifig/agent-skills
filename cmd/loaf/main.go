package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"runtime/debug"
	"strings"

	"github.com/levifig/loaf/internal/cli"
)

// Build metadata injected at link time via
//
//	-ldflags "-X main.buildCommit=<sha> -X main.buildDate=<iso8601>"
//
// These stay empty for plain `go build`, `go run`, and `go test`, keeping the
// version output clean unless a release build supplies them.
var (
	buildCommit string
	buildDate   string
)

// Dev identity linked by build-go.mjs via
//
//	-ldflags "-X main.devCommit=<sha> -X main.devModified=<true|false>"
//
// from `git rev-parse HEAD` and `git status --porcelain`. It duplicates the
// -buildvcs stamp for toolchains that do not write one inside a linked
// worktree (observed with go1.26.6; go1.27.1 does). The stamp wins when
// present; these fill in when it is absent.
var (
	devCommit   string
	devModified string
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		var silent interface {
			ExitCode() int
			Silent() bool
		}
		if errors.As(err, &silent) && silent.Silent() {
			os.Exit(silent.ExitCode())
		}
		fmt.Fprintf(os.Stderr, "loaf: %v\n", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	return newRunner(os.Stdout, os.Stderr).Run(args)
}

func newRunner(stdout, stderr io.Writer) cli.Runner {
	identityCommit, identityModified := devBuildIdentity(readBuildInfo(), buildCommit, buildDate, devCommit, devModified)
	return cli.Runner{
		Stdout:           stdout,
		Stderr:           stderr,
		BuildCommit:      buildCommit,
		BuildDate:        buildDate,
		DevBuildCommit:   identityCommit,
		DevBuildModified: identityModified,
	}
}

// readBuildInfo returns the build information the Go toolchain embedded in
// this binary, or nil when none is available.
func readBuildInfo() *debug.BuildInfo {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return nil
	}
	return info
}

// devBuildIdentity reads the source commit and working-tree state of a dev
// build. The primary source is the stamp `go build -buildvcs=true` writes
// (build-go.mjs passes the flag); when the toolchain wrote none — a linked
// worktree under go1.26.6, or no Git checkout at all — the values build-go.mjs
// linked from git stand in. Either way the identity is part of the compiled
// bytes, so it cannot drift from them the way a commit recorded in a separate
// file could. Release builds carry explicit metadata and never report dev
// identity. Without any commit, no provenance is invented.
func devBuildIdentity(info *debug.BuildInfo, releaseCommit, releaseDate, linkedCommit, linkedModified string) (commit string, modified bool) {
	if strings.TrimSpace(releaseCommit) != "" || strings.TrimSpace(releaseDate) != "" {
		return "", false
	}
	if info != nil {
		for _, setting := range info.Settings {
			switch setting.Key {
			case "vcs.revision":
				commit = strings.TrimSpace(setting.Value)
			case "vcs.modified":
				modified = setting.Value == "true"
			}
		}
	}
	if isCommitHash(commit) {
		return commit, modified
	}
	// A missing or malformed stamp never shadows the linked identity: both
	// describe the same tree at the same instant, so the valid one is the truth.
	if linked := strings.TrimSpace(linkedCommit); isCommitHash(linked) {
		return linked, strings.TrimSpace(linkedModified) == "true"
	}
	return "", false
}

// isCommitHash accepts an abbreviated or full lowercase Git object name. The
// version renderer needs at least seven hex digits to mint `+g<short-sha>`.
func isCommitHash(value string) bool {
	if len(value) < 7 || len(value) > 40 {
		return false
	}
	for _, char := range value {
		if (char < '0' || char > '9') && (char < 'a' || char > 'f') {
			return false
		}
	}
	return true
}
