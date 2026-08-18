package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
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
	return cli.Runner{
		Stdout:         stdout,
		Stderr:         stderr,
		BuildCommit:    buildCommit,
		BuildDate:      buildDate,
		DevBuildCommit: devBuildCommit(),
	}
}

// devBuildCommit reads the source commit recorded beside locally built native
// binaries. The ignored provenance file keeps build-varying identity outside
// the reproducible binary and is not present in shipped distributions.
func devBuildCommit() string {
	if buildCommit != "" || buildDate != "" {
		return ""
	}
	path, err := os.Executable()
	if err != nil {
		return ""
	}
	return readDevBuildCommit(path)
}

func readDevBuildCommit(executable string) string {
	targetDir := filepath.Dir(executable)
	nativeDir := filepath.Dir(targetDir)
	if filepath.Base(nativeDir) != "native" {
		return ""
	}
	body, err := os.ReadFile(filepath.Join(filepath.Dir(nativeDir), ".loaf-dev-commit"))
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(body))
}
