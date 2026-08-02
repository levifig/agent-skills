package cli

import (
	"fmt"
	"io/fs"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/levifig/loaf/internal/project"
)

// Loaf artifacts are named for what they are, never for the work unit that
// produced them. The directory or Change that contains an artifact already
// supplies that provenance, so repeating a work identity in the filename
// inverts the reference and rots the moment that work closes: the unit points
// at its artifacts, artifacts never point back.
//
// The guard is deliberately location-independent. It matches any directory
// whose basename is a known Loaf artifact directory, wherever that directory
// currently lives, plus retained Change research evidence. Relocating those
// directories — including moving them out of .agents/ entirely — therefore
// needs no change here.
//
// Two things that look similar are explicitly legal. A version is identity, not
// reference, so claude-code-2.1.218-plugin-startup-smoke.json is fine. A
// timestamp records when, not which work unit, so 20260512-122202-audit.md is
// fine. Only a numbered work identity is rejected.

type workIdentifierPattern struct {
	label string
	re    *regexp.Regexp
}

// Each pattern is segment-anchored so that incidental matches inside ordinary
// words cannot trip it: "startup" and "u2f" are not implementation units.
var workIdentifierPatterns = []workIdentifierPattern{
	{"implementation unit", regexp.MustCompile(`(?i)(^|[-_])u[0-9]+([-_]|$)`)},
	{"spec record", regexp.MustCompile(`(?i)(^|[-_])spec[-_]?[0-9]+([-_]|$)`)},
	{"task record", regexp.MustCompile(`(?i)(^|[-_])task[-_]?[0-9]+([-_]|$)`)},
	{"decision record", regexp.MustCompile(`(?i)(^|[-_])adr[-_]?[0-9]+([-_]|$)`)},
	{"pull request", regexp.MustCompile(`(?i)(^|[-_])pr[-_]?[0-9]+([-_]|$)`)},
	{"issue", regexp.MustCompile(`(?i)(^|[-_])issue[-_]?[0-9]+([-_]|$)`)},
}

// artifactNameIdentityOwners record the one place each numbered entity may name
// itself. A spec record inside a specs directory *is* SPEC-042; it does not
// refer to it. Everywhere else, that same string is a backward reference.
var artifactNameIdentityOwners = []struct {
	dirBase string
	prefix  string
}{
	{dirBase: "specs", prefix: "spec-"},
	{dirBase: "tasks", prefix: "task-"},
	{dirBase: "decisions", prefix: "adr-"},
}

// artifactNameSkipDirs are never walked: build output, dependencies, and VCS
// internals are not authored artifacts.
var artifactNameSkipDirs = map[string]bool{
	".git":         true,
	"node_modules": true,
	"dist":         true,
	"plugins":      true,
	"bin":          true,
	"vendor":       true,
}

func runNativeArtifactNames(context checkHookContext, runtimeRoot string) checkResult {
	result := checkResult{Passed: true, Warnings: []string{}, Errors: []string{}, Findings: []string{}}
	// Resolve the project root explicitly rather than treating the runtime root as
	// the scan root: the two diverge when loaf runs from an installed location.
	root := firstNonEmpty(strings.TrimSpace(runtimeRoot), ".")
	if resolved, err := project.ResolveRoot(root); err == nil {
		root = resolved.Path()
	}

	violations, err := findWorkIdentifierArtifactNames(root)
	if err != nil {
		result.Passed = false
		result.Blocked = true
		result.Errors = append(result.Errors, fmt.Sprintf("scan artifact names: %v", err))
		return result
	}
	for _, violation := range violations {
		result.Passed = false
		result.Blocked = true
		result.Errors = append(result.Errors, violation.message())
	}
	if result.Passed {
		result.Findings = append(result.Findings, "artifact names carry no work identity")
	} else {
		result.Findings = append(result.Findings,
			"A work identity in a filename inverts the reference: the unit points at its artifacts, never the reverse.",
			"Rename the artifact for what it is. Run with --advisory to report without blocking.")
	}
	return result
}

type artifactNameViolation struct {
	rel   string
	label string
}

func (v artifactNameViolation) message() string {
	return fmt.Sprintf(
		"%s names a %s. The containing directory already records that provenance; name the artifact for what it is.",
		v.rel, v.label)
}

// findWorkIdentifierArtifactNames reports every Loaf-authored artifact whose
// filename carries a work identity.
//
// Tracked paths are the unit of judgement: an untracked scratch file is nobody's
// published artifact, and blocking a commit over one the author never staged
// would be an obstruction rather than a gate. Outside a Git repository the
// filesystem is the only available source of truth, so the walk stands in.
func findWorkIdentifierArtifactNames(root string) ([]artifactNameViolation, error) {
	candidates, err := trackedRepositoryPaths(root)
	if err != nil {
		candidates, err = walkedRepositoryPaths(root)
		if err != nil {
			return nil, err
		}
	}
	var violations []artifactNameViolation
	for _, rel := range candidates {
		if !isLoafAuthoredArtifact(rel) {
			continue
		}
		label, found := workIdentityInArtifactName(rel)
		if !found {
			continue
		}
		// A finalized or archived artifact is a historical record. This principle
		// governs how new artifacts are named, not whether closed ones get rewritten,
		// and a durable render's filename is bound to its state record, which has no
		// rename path today. Enforcing retroactively would block every commit on a
		// name nobody can legally change.
		if isFinalizedArtifact(root, rel) {
			continue
		}
		violations = append(violations, artifactNameViolation{rel: rel, label: label})
	}
	sort.Slice(violations, func(i, j int) bool { return violations[i].rel < violations[j].rel })
	return violations, nil
}

// trackedRepositoryPaths lists repository-relative tracked paths, which includes
// anything already staged for the commit being gated.
func trackedRepositoryPaths(root string) ([]string, error) {
	output, err := exec.Command("git", "-C", root, "ls-files").Output()
	if err != nil {
		return nil, err
	}
	var paths []string
	for _, line := range strings.Split(strings.TrimSpace(string(output)), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		paths = append(paths, filepath.ToSlash(line))
	}
	return paths, nil
}

// walkedRepositoryPaths enumerates candidate paths without Git.
func walkedRepositoryPaths(root string) ([]string, error) {
	var paths []string
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			// An unreadable subtree is not evidence of compliance, but it is also
			// not this check's business to fail on; skip it and keep scanning.
			if entry != nil && entry.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		if entry.IsDir() {
			if artifactNameSkipDirs[entry.Name()] {
				return fs.SkipDir
			}
			return nil
		}
		rel, relErr := filepath.Rel(root, path)
		if relErr != nil {
			return nil
		}
		paths = append(paths, filepath.ToSlash(rel))
		return nil
	})
	if err != nil {
		return nil, err
	}
	return paths, nil
}

// A closed artifact may record its terminal state at any depth in front matter —
// a council nests it under `council:`, a report keeps it top level — so the
// status key is matched with optional leading indentation rather than anchored
// to column zero. The accepted values are Loaf's canonical terminal vocabulary.
var artifactNameFinalizedStatusRE = regexp.MustCompile(`(?mi)^\s*status:\s*["']?(final|archived|done|completed)["']?\s*$`)

// isFinalizedArtifact reports whether rel's front matter closes it. Only the
// leading front-matter block is consulted, so a status word in the body cannot
// silence the guard.
func isFinalizedArtifact(root string, rel string) bool {
	body, err := readRegularFile(filepath.Join(root, filepath.FromSlash(rel)), projectFileReadLimit)
	if err != nil {
		return false
	}
	text := string(body)
	if !strings.HasPrefix(text, "---") {
		return false
	}
	end := strings.Index(text[3:], "\n---")
	if end < 0 {
		return false
	}
	return artifactNameFinalizedStatusRE.MatchString(text[3 : 3+end])
}

// isLoafAuthoredArtifact reports whether rel lives somewhere Loaf itself writes
// artifacts. Matching artifact directories by basename rather than by full path
// keeps the guard correct when those directories move.
func isLoafAuthoredArtifact(rel string) bool {
	segments := strings.Split(rel, "/")
	if len(segments) < 2 {
		return false
	}
	dirs := segments[:len(segments)-1]
	for index, segment := range dirs {
		if _, known := artifactBodyPathDirs[segment]; known {
			return true
		}
		// Retained Change research evidence: docs/changes/<slug>/research/...
		if segment == "research" && index > 0 && dirs[index-1] != "" {
			return true
		}
	}
	return false
}

// taskOwnerPrefixRE matches the TASK-NNN- owner prefix on a numbered task
// record so the slug remainder can be scanned for foreign work identities.
var taskOwnerPrefixRE = regexp.MustCompile(`(?i)^task-[0-9]+-`)

// workIdentityInArtifactName reports the kind of work identity embedded in
// rel's basename, unless the artifact is the entity that identity names.
//
// Task records in a tasks/ directory are identity owners for their own
// TASK-NNN prefix, but the slug remainder after TASK-NNN- is still scanned —
// so TASK-001-spec-053-followup.md is caught while TASK-001-configure-build.md
// is not.
func workIdentityInArtifactName(rel string) (string, bool) {
	segments := strings.Split(rel, "/")
	name := strings.TrimSuffix(segments[len(segments)-1], filepath.Ext(segments[len(segments)-1]))
	lowerName := strings.ToLower(name)
	scanName := name
	for _, owner := range artifactNameIdentityOwners {
		if !strings.HasPrefix(lowerName, owner.prefix) {
			continue
		}
		inOwnerDir := false
		for _, dir := range segments[:len(segments)-1] {
			if dir == owner.dirBase {
				inOwnerDir = true
				break
			}
		}
		if !inOwnerDir {
			continue
		}
		if owner.dirBase == "tasks" && owner.prefix == "task-" {
			remainder := taskOwnerPrefixRE.ReplaceAllString(lowerName, "")
			if remainder == "" || remainder == lowerName {
				return "", false
			}
			scanName = remainder
			break
		}
		return "", false
	}
	for _, pattern := range workIdentifierPatterns {
		if pattern.re.MatchString(scanName) {
			return pattern.label, true
		}
	}
	return "", false
}
