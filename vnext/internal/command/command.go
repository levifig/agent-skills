// Package command exposes the deliberately small vNext bootstrap surface.
package command

import (
	"fmt"
	"io"
	"strings"

	"github.com/levifig/loaf/vnext/internal/kernel"
)

const usage = "usage: loaf <version|ownership>\n"

// Runner writes command results to caller-provided streams.
type Runner struct {
	stdout io.Writer
	stderr io.Writer
}

// New constructs a vNext command runner. A nil stream explicitly discards the
// corresponding output.
func New(stdout, stderr io.Writer) Runner {
	if stdout == nil {
		stdout = io.Discard
	}
	if stderr == nil {
		stderr = io.Discard
	}
	return Runner{stdout: stdout, stderr: stderr}
}

// Run executes one of the kernel's introspection commands and returns a process
// exit code. Stateful workflows are intentionally absent from this bootstrap.
func (runner Runner) Run(args []string) int {
	if len(args) == 0 {
		return write(runner.stdout, usage)
	}

	if len(args) != 1 {
		return runner.unknown(args)
	}

	switch args[0] {
	case "version":
		identity := kernel.CurrentIdentity()
		return write(
			runner.stdout,
			fmt.Sprintf("%s %s schema %s/%d\n", identity.Product, identity.Generation, identity.Schema.Line, identity.Schema.Version),
		)
	case "ownership":
		matrix := kernel.OwnershipMatrix()
		lines := make([]string, 0, len(matrix))
		for _, ownership := range matrix {
			lines = append(lines, fmt.Sprintf("%s: %s", ownership.Authority, strings.Join(ownership.Responsibilities, ", ")))
		}
		return write(runner.stdout, strings.Join(lines, "\n")+"\n")
	default:
		return runner.unknown(args)
	}
}

func (runner Runner) unknown(args []string) int {
	message := fmt.Sprintf("unknown command %q\n%s", strings.Join(args, " "), usage)
	if write(runner.stderr, message) != 0 {
		return 1
	}
	return 2
}

func write(destination io.Writer, value string) int {
	written, err := io.WriteString(destination, value)
	if err != nil || written != len(value) {
		return 1
	}
	return 0
}
