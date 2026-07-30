package cli

import (
	"fmt"
	"os"
	"path/filepath"
)

// releaseCapabilityEvidenceRunners is static remediation copy: all three
// receipt runners share the --client/--expected-version/--receipt shape. The
// list is never parsed out of the loader error.
const releaseCapabilityEvidenceRunners = "cli/scripts/smoke-claude-code-startup.mjs, cli/scripts/smoke-codex-startup.mjs, or cli/scripts/smoke-opencode-request-context.mjs, each with --client <cli> --expected-version <installed> --receipt <path>"

// checkReleaseCapabilityEvidence validates the capability evidence registry
// against the tree at root. Absent evidence exempts the project (present is
// false); any other failure — unreadable, invalid, or stale receipts — must
// refuse the release. There is deliberately no override.
func checkReleaseCapabilityEvidence(root string) (present bool, err error) {
	path := filepath.Join(root, filepath.FromSlash(TargetCapabilityEvidenceRecordPath))
	if _, statErr := os.Stat(path); statErr != nil {
		if os.IsNotExist(statErr) {
			return false, nil
		}
		return true, fmt.Errorf("inspect capability evidence %s: %w", TargetCapabilityEvidenceRecordPath, statErr)
	}
	if _, loadErr := LoadTargetCapabilityEvidence(path); loadErr != nil {
		return true, loadErr
	}
	return true, nil
}

func releaseApplyCapabilityEvidenceRefusal(err error) error {
	return fmt.Errorf("Refusing to commit release artifacts: capability evidence is invalid or stale against the rebuilt tree: %v; re-record with the matching runner (%s) after the artifact rebuild, then rerun the release", err, releaseCapabilityEvidenceRunners)
}

func releasePostMergeCapabilityEvidenceAbortMessage(err error) string {
	return fmt.Sprintf("capability evidence is invalid or stale on the merged tree: %v — re-record with the matching runner (%s) after the artifact rebuild, land the corrected evidence, and rerun", err, releaseCapabilityEvidenceRunners)
}
