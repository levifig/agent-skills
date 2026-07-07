package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

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
	// argv[0] dispatch: `loaf shim enable gh` symlinks a file named "gh" to
	// this binary. Invoked that way, skip the normal CLI parser entirely and
	// hand off to the per-invocation identity shim (see change.md).
	if filepath.Base(os.Args[0]) == "gh" {
		os.Exit(cli.RunGHShim(os.Args, os.Environ()))
	}
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
		Stdout:      stdout,
		Stderr:      stderr,
		BuildCommit: buildCommit,
		BuildDate:   buildDate,
	}
}
