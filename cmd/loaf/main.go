package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"time"

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
	return cli.Runner{
		Stdout:       stdout,
		Stderr:       stderr,
		BuildCommit:  buildCommit,
		BuildDate:    buildDate,
		DevBuildTime: devBuildTime(),
	}
}

// devBuildTime reports when this binary was linked, and only for a build that
// carries no release metadata — the same absence that keeps the version line
// clean is what makes a build a dev build.
//
// The link time is read from the executable's own modification time rather than
// injected at link time, because the committed native binaries must rebuild
// byte-for-byte identical (cli/scripts/verify-go-artifacts.mjs) and a stamp in
// the bytes would differ on every build. The linker writes the file; the
// filesystem records when. A release build returns the zero time, and so does
// any build whose executable cannot be located or stat'd — identity falls back
// to the distribution's release version, never to a guess.
func devBuildTime() time.Time {
	if buildCommit != "" || buildDate != "" {
		return time.Time{}
	}
	path, err := os.Executable()
	if err != nil {
		return time.Time{}
	}
	info, err := os.Stat(path)
	if err != nil {
		return time.Time{}
	}
	return info.ModTime()
}
