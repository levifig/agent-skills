package cli

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/levifig/loaf/internal/project"
	"github.com/levifig/loaf/internal/state"
)

// loafRepoTier ranks how confidently a directory can be treated as a
// Loaf-powered repo. The constants ascend with strength so callers can compare
// them directly; the ordering is load-bearing.
type loafRepoTier int

const (
	// loafRepoTierNone means no probe matched.
	loafRepoTierNone loafRepoTier = iota
	// loafRepoTierLegacy means only deprecated `.agents/` folders were found,
	// which is enough to ask the user but never enough to proceed alone.
	loafRepoTierLegacy
	// loafRepoTierStrong means a managed marker or the project config was found.
	loafRepoTierStrong
	// loafRepoTierAuthoritative means the state database has a project record
	// for the resolved root.
	loafRepoTierAuthoritative
)

func (tier loafRepoTier) String() string {
	switch tier {
	case loafRepoTierAuthoritative:
		return "authoritative"
	case loafRepoTierStrong:
		return "strong"
	case loafRepoTierLegacy:
		return "legacy"
	default:
		return "none"
	}
}

// loafRepoDetection carries the strongest tier any probe reached plus every
// matched signal, in descending strength order, as text fit to print.
type loafRepoDetection struct {
	Tier  loafRepoTier
	Bases []string
}

func (detection *loafRepoDetection) record(tier loafRepoTier, basis string) {
	if tier > detection.Tier {
		detection.Tier = tier
	}
	detection.Bases = append(detection.Bases, basis)
}

// legacyLoafArtifactDirs are the deprecated `.agents/` subfolders that, on
// their own, only justify asking whether this is a Loaf project. Kept sorted so
// the evidence basis reads the same way on every machine.
var legacyLoafArtifactDirs = []string{"councils", "drafts", "handoffs", "reports", "sessions", "specs"}

// detectLoafRepo answers "is this a Loaf-powered repo?" for the commands that
// branch on it. It is a pure read: it never prompts, never writes, and never
// creates the state database. Every probe degrades to "no signal" on error, so
// a missing or unreadable database only costs the authoritative tier.
func detectLoafRepo(root project.Root, stateHome string) loafRepoDetection {
	detection := loafRepoDetection{}
	if basis, ok := detectLoafProjectRecord(root, stateHome); ok {
		detection.record(loafRepoTierAuthoritative, basis)
	}
	if basis, ok := detectLoafFencedMarker(root.Path()); ok {
		detection.record(loafRepoTierStrong, basis)
	}
	if basis, ok := detectLoafProjectConfig(root.Path()); ok {
		detection.record(loafRepoTierStrong, basis)
	}
	if basis, ok := detectLegacyLoafArtifacts(root.Path()); ok {
		detection.record(loafRepoTierLegacy, basis)
	}
	return detection
}

func detectLoafProjectRecord(root project.Root, stateHome string) (string, bool) {
	databasePath, err := (state.PathResolver{StateHome: stateHome}).DatabasePath(root)
	if err != nil {
		return "", false
	}
	if info, err := os.Stat(databasePath); err != nil || info.IsDir() {
		return "", false
	}
	store, err := state.OpenStoreReadOnly(databasePath)
	if err != nil {
		return "", false
	}
	defer store.Close()
	identity, err := store.LookupProjectIdentityForRoot(context.Background(), root)
	if err != nil {
		return "", false
	}
	return fmt.Sprintf("project record %s in the state database", identity.ID), true
}

func detectLoafFencedMarker(rootPath string) (string, bool) {
	body, err := os.ReadFile(filepath.Join(rootPath, "AGENTS.md"))
	if err != nil || !strings.Contains(string(body), fencedStartMarker) {
		return "", false
	}
	return "managed Loaf section in AGENTS.md", true
}

func detectLoafProjectConfig(rootPath string) (string, bool) {
	info, err := os.Stat(filepath.Join(rootPath, ".agents", "loaf.json"))
	if err != nil || info.IsDir() {
		return "", false
	}
	return "Loaf project config at .agents/loaf.json", true
}

func detectLegacyLoafArtifacts(rootPath string) (string, bool) {
	found := []string{}
	for _, name := range legacyLoafArtifactDirs {
		info, err := os.Stat(filepath.Join(rootPath, ".agents", name))
		if err != nil || !info.IsDir() {
			continue
		}
		found = append(found, ".agents/"+name)
	}
	if len(found) == 0 {
		return "", false
	}
	return "legacy Loaf artifact folders: " + strings.Join(found, ", "), true
}
