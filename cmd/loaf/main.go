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
	identityCommit, identityModified := devBuildIdentity(readBuildInfo(), buildCommit, buildDate)
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

// devBuildIdentity reads the source commit and working-tree state that
// `go build -buildvcs=true` stamps into the binary (build-go.mjs passes the
// flag; go.mod pins a toolchain that stamps linked worktrees). The identity is
// mechanical: it describes the bytes that were compiled, so it cannot drift
// from them the way a commit recorded in a separate file could. Release builds
// carry explicit metadata and never report dev identity. A binary compiled
// without VCS information, or with a malformed stamp, reports no provenance
// rather than inventing one.
func devBuildIdentity(info *debug.BuildInfo, releaseCommit, releaseDate string) (commit string, modified bool) {
	if strings.TrimSpace(releaseCommit) != "" || strings.TrimSpace(releaseDate) != "" || info == nil {
		return "", false
	}
	for _, setting := range info.Settings {
		switch setting.Key {
		case "vcs.revision":
			commit = strings.TrimSpace(setting.Value)
		case "vcs.modified":
			modified = setting.Value == "true"
		}
	}
	if !isCommitHash(commit) {
		return "", false
	}
	return commit, modified
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
