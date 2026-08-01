package cli

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
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

// loafRepoSignal is one probe's answer: the tier it justifies, and the evidence
// line that says why. A probe carries its own tier because not every probe
// answers at a fixed strength — a managed AGENTS.md section means one thing
// when its header parses and another when it does not.
type loafRepoSignal struct {
	tier  loafRepoTier
	basis string
}

// detectLoafRepo answers "is this a Loaf-powered repo?" for the commands that
// branch on it. It is a pure read: it never prompts, never writes, and never
// creates the state database. Every probe degrades to "no signal" on error, so
// a missing or unreadable database only costs the authoritative tier.
func detectLoafRepo(root project.Root, stateHome string) loafRepoDetection {
	var signals []loafRepoSignal
	if basis, ok := detectLoafProjectRecord(root, stateHome); ok {
		signals = append(signals, loafRepoSignal{tier: loafRepoTierAuthoritative, basis: basis})
	}
	if signal, ok := detectLoafFencedMarker(root.Path()); ok {
		signals = append(signals, signal)
	}
	if basis, ok := detectLoafProjectConfig(root.Path()); ok {
		signals = append(signals, loafRepoSignal{tier: loafRepoTierStrong, basis: basis})
	}
	if basis, ok := detectLegacyLoafArtifacts(root.Path()); ok {
		signals = append(signals, loafRepoSignal{tier: loafRepoTierLegacy, basis: basis})
	}
	// Callers print Bases[0] as the reason they are treating this directory as a
	// Loaf repo, so the slice has to lead with a basis recorded at the tier that
	// was actually reached. Probe order alone stopped guaranteeing that once one
	// probe could answer at either of two tiers.
	sort.SliceStable(signals, func(i, j int) bool { return signals[i].tier > signals[j].tier })
	detection := loafRepoDetection{}
	for _, signal := range signals {
		detection.record(signal.tier, signal.basis)
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

// detectionReadLimit bounds what a probe will read. The detector runs on every
// install and upgrade against a directory nobody has vouched for yet, so the
// file at a probe path is untrusted input: it can be arbitrarily large, and
// reading all of it to answer a yes/no question is the whole exposure. A marker
// beyond the limit contributes no signal, which is the same degradation every
// other probe here already has.
const detectionReadLimit = 256 << 10

// detectLoafFencedMarker requires a complete managed section — both fences —
// before it will say anything at all. A lone start marker is a fragment: an
// interrupted write, a quoted example, or a hand-edited file, and none of those
// mean Loaf manages this AGENTS.md. Promoting a fragment would let `loaf
// upgrade` write project files without asking.
//
// A complete section whose header does not parse is the third answer, and it is
// neither of the other two. Not strong: the header is the one part of the
// section Loaf writes and can recognize, and proceeding on one it cannot read
// would mean writing project files on the strength of a marker that has been
// tampered with, truncated, or left by something else. Not silence either: a
// paired fence is real evidence, and the legacy tier routes it to the explicit
// "is this a Loaf project?" confirmation, which is the question this state
// actually raises.
func detectLoafFencedMarker(rootPath string) (loafRepoSignal, bool) {
	body, ok := readDetectionFilePrefix(filepath.Join(rootPath, "AGENTS.md"), detectionReadLimit)
	if !ok {
		return loafRepoSignal{}, false
	}
	section, complete := findFencedSectionRange(body)
	if !complete {
		return loafRepoSignal{}, false
	}
	if section.malformedHeader {
		return loafRepoSignal{tier: loafRepoTierLegacy, basis: "tampered or malformed managed section in AGENTS.md"}, true
	}
	return loafRepoSignal{tier: loafRepoTierStrong, basis: "managed Loaf section in AGENTS.md"}, true
}

func detectLoafProjectConfig(rootPath string) (string, bool) {
	file, ok := openDetectionFile(filepath.Join(rootPath, ".agents", "loaf.json"))
	if !ok {
		return "", false
	}
	file.Close()
	return "Loaf project config at .agents/loaf.json", true
}

// readDetectionFilePrefix reads at most limit bytes of a probe path, and only
// once openDetectionFile has established that the descriptor it returns is a
// regular file.
func readDetectionFilePrefix(path string, limit int64) (string, bool) {
	file, ok := openDetectionFile(path)
	if !ok {
		return "", false
	}
	defer file.Close()
	body, err := io.ReadAll(io.LimitReader(file, limit))
	if err != nil {
		return "", false
	}
	return string(body), true
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
