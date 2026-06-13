package cli

import "fmt"

// ExitError reports a CLI exit code that the front controller should propagate
// without wrapping or duplicating command-owned stderr.
type ExitError struct {
	Code int
}

func (e ExitError) Error() string {
	return fmt.Sprintf("command exited with code %d", e.Code)
}

// ExitCode returns the process exit code.
func (e ExitError) ExitCode() int {
	return e.Code
}

// Silent tells the top-level CLI that the command already owned stderr.
func (e ExitError) Silent() bool {
	return true
}
