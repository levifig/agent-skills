package main

import (
	"errors"
	"fmt"
	"os"

	"github.com/levifig/loaf/internal/cli"
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
	runner := cli.Runner{
		Stdout: os.Stdout,
		Stderr: os.Stderr,
	}
	return runner.Run(args)
}
