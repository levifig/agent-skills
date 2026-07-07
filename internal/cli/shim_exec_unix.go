//go:build unix

package cli

import "syscall"

// execRealGH replaces the current process image with realGH, matching how
// rbenv/pyenv/direnv-style shims hand off: no wrapper process lingers, stdio
// and signal handling become the real gh's, and the exit code is inherited
// verbatim. It only returns when exec itself failed to start.
func execRealGH(realGH string, args []string, env []string) error {
	return syscall.Exec(realGH, args, env)
}
