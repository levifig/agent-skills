package project

import (
	"crypto/sha256"
	"encoding/hex"
	"os/exec"
	"strings"
)

// OriginRemoteURL returns the normalized origin remote when git is available.
func OriginRemoteURL(root Root) (string, error) {
	raw, ok := gitOutput(root.Path(), "remote", "get-url", "origin")
	if !ok {
		return "", nil
	}
	return NormalizeRemoteURL(raw)
}

// RootCommitFingerprint returns a stable fingerprint for the repository root
// commit. It is used for doctor twin-audit only, not steady-state attach.
func RootCommitFingerprint(root Root) (string, error) {
	sha, ok := gitOutput(root.Path(), "rev-list", "--max-parents=0", "HEAD")
	if !ok {
		return "", nil
	}
	sha = strings.TrimSpace(sha)
	if sha == "" {
		return "", nil
	}
	sum := sha256.Sum256([]byte("loaf-root-commit:" + sha))
	return hex.EncodeToString(sum[:8]), nil
}

// ListNormalizedRemotes returns normalized remote URLs for every configured git remote.
func ListNormalizedRemotes(root Root) ([]string, error) {
	cmd := exec.Command("git", "remote")
	cmd.Dir = root.Path()
	out, err := cmd.Output()
	if err != nil {
		return nil, nil
	}
	names := strings.Fields(strings.TrimSpace(string(out)))
	keys := make([]string, 0, len(names))
	seen := map[string]struct{}{}
	for _, name := range names {
		raw, ok := gitOutput(root.Path(), "remote", "get-url", name)
		if !ok {
			continue
		}
		key, err := NormalizeRemoteURL(raw)
		if err != nil || key == "" {
			continue
		}
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		keys = append(keys, key)
	}
	return keys, nil
}
