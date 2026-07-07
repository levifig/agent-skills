//go:build windows

package cli

import "fmt"

// execRealGH is unreachable in practice: runShimEnable refuses on Windows
// (the shim mechanics are POSIX-only, see change.md's Out-of-scope section),
// so RunGHShim never dispatches here on a real install. This stub exists so
// the cli package still builds for GOOS=windows.
func execRealGH(realGH string, args []string, env []string) error {
	return fmt.Errorf("gh shim exec is not supported on Windows")
}
