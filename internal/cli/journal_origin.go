package cli

import (
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/levifig/loaf/internal/state"
)

// ResolveManualJournalOrigin captures the local Git context that is available
// for a manual journal write. Git is contextual rather than required here:
// journal logging remains useful outside a repository and in repositories
// without a commit, so unavailable fields stay empty instead of being guessed.
func ResolveManualJournalOrigin(rootPath, sourceEvent string) state.JournalOriginInput {
	origin := state.JournalOriginInput{
		EnvelopeVersion:  state.JournalOriginEnvelopeVersion,
		CaptureMechanism: state.JournalOriginMechanismManual,
		SourceEvent:      sourceEvent,
	}
	if strings.TrimSpace(rootPath) == "" {
		rootPath = "."
	}
	worktreeBytes, err := originGitOutputBytes(rootPath, "rev-parse", "--show-toplevel")
	if err != nil {
		return origin
	}
	worktree := strings.TrimSpace(string(worktreeBytes))
	if worktree == "" {
		return origin
	}
	if absolute, absErr := filepath.Abs(worktree); absErr == nil {
		worktree = absolute
	}
	if evaluated, evalErr := filepath.EvalSymlinks(worktree); evalErr == nil {
		worktree = evaluated
	}
	origin.Worktree = worktree

	if headBytes, headErr := originGitOutputBytes(worktree, "rev-parse", "--verify", "HEAD"); headErr == nil {
		origin.Head = strings.TrimSpace(string(headBytes))
	}
	if branchBytes, branchErr := originGitOutputBytes(worktree, "symbolic-ref", "--quiet", "--short", "HEAD"); branchErr == nil {
		origin.Branch = strings.TrimSpace(string(branchBytes))
	}
	return origin
}

func originGitOutputBytes(cwd string, args ...string) ([]byte, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = cwd
	return cmd.Output()
}
