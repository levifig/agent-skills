package cli

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
	"time"
)

const (
	installDeprecationManifestPath = "config/deprecations.json"
	defaultDeprecationWindow       = "one-release"
	migrationReceiptDirName        = "migration-receipts"
)

type installDeprecationManifest struct {
	Version            int                         `json:"version"`
	RetiredTargets     []retiredInstallTarget      `json:"retired_targets"`
	RetiredSkills      []retiredInstallSkill       `json:"retired_skills"`
	RetiredAgents      []retiredInstallAgent       `json:"retired_agents"`
	ExternalizedSkills []externalizedInstallSkill  `json:"externalized_skills"`
	Relocations        []installRelocationManifest `json:"relocations"`
	Aliases            []installAliasManifest      `json:"aliases"`
}

type retiredInstallTarget struct {
	Target  string   `json:"target"`
	Since   string   `json:"since"`
	Window  string   `json:"window"`
	Reason  string   `json:"reason"`
	Signoff string   `json:"signoff"`
	Paths   []string `json:"paths"`
}

type retiredInstallSkill struct {
	Skill      string   `json:"skill"`
	Since      string   `json:"since"`
	Window     string   `json:"window"`
	Reason     string   `json:"reason"`
	Signoff    string   `json:"signoff"`
	SkillHomes []string `json:"skill_homes"`
}

type retiredInstallAgent struct {
	Agent      string   `json:"agent"`
	Since      string   `json:"since"`
	Window     string   `json:"window"`
	Reason     string   `json:"reason"`
	Signoff    string   `json:"signoff"`
	AgentHomes []string `json:"agent_homes"`
}

type externalizedInstallSkill struct {
	Skill          string   `json:"skill"`
	Since          string   `json:"since"`
	Window         string   `json:"window"`
	Reason         string   `json:"reason"`
	Signoff        string   `json:"signoff"`
	Source         string   `json:"source"`
	InstallCommand string   `json:"install_command"`
	SkillHomes     []string `json:"skill_homes"`
}

type installRelocationManifest struct {
	ID          string `json:"id"`
	From        string `json:"from"`
	To          string `json:"to"`
	OwnerMarker string `json:"owner_marker"`
	Since       string `json:"since"`
	Window      string `json:"window"`
	Reason      string `json:"reason"`
	Signoff     string `json:"signoff"`
}

type installAliasManifest struct {
	From    string `json:"from"`
	To      string `json:"to"`
	Since   string `json:"since"`
	Window  string `json:"window"`
	Reason  string `json:"reason"`
	Signoff string `json:"signoff"`
}

type installDeprecationCleanupResult struct {
	Removed      []installDeprecationCleanupAction
	Unmanaged    []installDeprecationCleanupAction
	Externalized []installDeprecationCleanupAction
	Aliases      []installDeprecationCleanupAction
	Skipped      []installDeprecationCleanupAction
	Receipt      installMigrationReceipt
	Mutated      bool
}

type installDeprecationCleanupAction struct {
	Kind    string
	Name    string
	Path    string
	Reason  string
	Since   string
	Window  string
	Signoff string
	Source  string
	Command string
	Action  string
}

// installMigrationReceipt is machine evidence for a destructive migration pass:
// a single before/after tree-hash over every swept home/path.
type installMigrationReceipt struct {
	Before  string   `json:"before"`
	After   string   `json:"after"`
	Homes   []string `json:"homes"`
	Mutated bool     `json:"mutated"`
	When    string   `json:"when,omitempty"`
}

type skillOwnershipClass string

const (
	skillOwnershipAbsent     skillOwnershipClass = "absent"
	skillOwnershipOwned      skillOwnershipClass = "owned"
	skillOwnershipUnmanaged  skillOwnershipClass = "unmanaged"
	skillOwnershipMismatch   skillOwnershipClass = "mismatch"
	skillOwnershipDangling   skillOwnershipClass = "dangling"
	skillOwnershipUnreadable skillOwnershipClass = "unreadable"
)

type skillOwnershipVerdict struct {
	Class    skillOwnershipClass
	Recorded string
	Actual   string
	Claimed  bool
	Legacy   bool // claimed under a pre-v2 name-only manifest; never digest-proven
}

func runInstallDeprecationCleanup(loafRoot string, out io.Writer, allowDestructive bool) error {
	manifest, found, err := loadInstallDeprecationManifest(loafRoot)
	if err != nil {
		return err
	}
	if !found || manifest.isEmpty() {
		return nil
	}
	pathContext := installPathContext()
	result, err := applyInstallDeprecationCleanup(manifest, pathContext, allowDestructive)
	if err != nil {
		return err
	}
	var receiptPersistErr error
	if result.Mutated {
		// Evidence must never abort or obscure the mutations it records.
		if err := persistInstallMigrationReceipt(pathContext["HOME"], result.Receipt); err != nil {
			receiptPersistErr = err
		}
	}
	writeInstallDeprecationCleanup(out, result)
	if receiptPersistErr != nil {
		fmt.Fprintf(out, "  %s migration receipt persistence failed after cleanup mutations: %v\n", ansiYellow("⚠"), receiptPersistErr)
		fmt.Fprintf(out, "    %s destructive work already applied; receipt is missing — inspect skill homes manually\n", ansiYellow("⚠"))
	}
	return nil
}

func loadInstallDeprecationManifest(loafRoot string) (installDeprecationManifest, bool, error) {
	path := filepath.Join(loafRoot, installDeprecationManifestPath)
	file, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return installDeprecationManifest{}, false, nil
		}
		return installDeprecationManifest{}, false, err
	}
	defer file.Close()

	decoder := json.NewDecoder(file)
	decoder.DisallowUnknownFields()
	var manifest installDeprecationManifest
	if err := decoder.Decode(&manifest); err != nil {
		return installDeprecationManifest{}, true, fmt.Errorf("read install deprecation manifest: %w", err)
	}
	if manifest.Version != 1 {
		return installDeprecationManifest{}, true, fmt.Errorf("install deprecation manifest version %d is not supported", manifest.Version)
	}
	return manifest, true, nil
}

func (m installDeprecationManifest) isEmpty() bool {
	return len(m.RetiredTargets) == 0 &&
		len(m.RetiredSkills) == 0 &&
		len(m.RetiredAgents) == 0 &&
		len(m.ExternalizedSkills) == 0 &&
		len(m.Relocations) == 0 &&
		len(m.Aliases) == 0
}

func applyInstallDeprecationCleanup(manifest installDeprecationManifest, pathContext map[string]string, allowDestructive bool) (installDeprecationCleanupResult, error) {
	var result installDeprecationCleanupResult
	sweepHomes, err := skillMigrationSweepHomes(manifest, pathContext)
	if err != nil {
		return result, err
	}
	result.Receipt.Homes = append([]string{}, sweepHomes...)

	orphanHomes, err := quarantineOrphanScanHomes(manifest, pathContext)
	if err != nil {
		return result, err
	}
	if err := reconcileQuarantineOrphans(orphanHomes, &result); err != nil {
		return result, err
	}

	// Hash before only when a destructive pass may mutate — dry runs must not
	// emit a no-op receipt that looks like completed migration evidence.
	if allowDestructive {
		before, err := hashMigrationSkillHomes(sweepHomes)
		if err != nil {
			return result, err
		}
		result.Receipt.Before = before
	}

	for _, target := range manifest.RetiredTargets {
		for _, rawPath := range target.Paths {
			path, err := expandInstallDeprecationPath(rawPath, pathContext)
			if err != nil {
				return result, err
			}
			action := installDeprecationCleanupAction{
				Kind:    "target",
				Name:    target.Target,
				Path:    path,
				Reason:  target.Reason,
				Since:   target.Since,
				Window:  deprecationWindow(target.Window),
				Signoff: target.Signoff,
			}
			if err := requireMigrationSkillHomeRoot(path); err != nil {
				if os.IsNotExist(err) {
					// Absent retirement: no-op, not news — omit from the report.
					continue
				}
				if isDeliberateMigrationRootRefusal(err) {
					// Unowned / refused root: Loaf will never touch it — omit.
					continue
				}
				return result, fmt.Errorf("inspect retired target %s: %w", path, err)
			}
			marker := filepath.Join(path, loafInstallMarkerFile)
			if !isRegularLoafMarkerFile(marker) {
				// Path exists but Loaf has no ownership claim — omit.
				continue
			}
			if !allowDestructive {
				action.Action = "confirmation-required"
				result.Skipped = append(result.Skipped, action)
				continue
			}
			removedSomething, foreignLeft, err := retireMarkedTargetPath(path)
			if err != nil {
				return result, err
			}
			if removedSomething {
				result.Mutated = true
			}
			if foreignLeft {
				action.Action = "unmanaged"
				result.Unmanaged = append(result.Unmanaged, action)
			} else if removedSomething || !pathExistsForDeprecation(path) {
				action.Action = "removed"
				result.Removed = append(result.Removed, action)
			} else {
				action.Action = "unmanaged"
				result.Unmanaged = append(result.Unmanaged, action)
			}
		}
	}
	for _, skill := range manifest.RetiredSkills {
		homes, err := retiredSkillHomes(skill, pathContext)
		if err != nil {
			return result, err
		}
		for _, home := range homes {
			path := filepath.Join(home, skill.Skill)
			action := installDeprecationCleanupAction{
				Kind:    "skill",
				Name:    skill.Skill,
				Path:    path,
				Reason:  skill.Reason,
				Since:   skill.Since,
				Window:  deprecationWindow(skill.Window),
				Signoff: skill.Signoff,
			}
			if err := requireMigrationSkillHomeRoot(home); err != nil {
				if os.IsNotExist(err) {
					// Absent retirement: no-op, not news — omit from the report.
					continue
				}
				if isDeliberateMigrationRootRefusal(err) {
					// Symlinked or non-directory home: never mutate through it.
					if allowDestructive {
						action.Action = "unmanaged"
						result.Unmanaged = append(result.Unmanaged, action)
					} else if pathExistsForDeprecation(path) {
						action.Action = "confirmation-required"
						result.Skipped = append(result.Skipped, action)
					}
					// Absent under a refused root: omit (not news).
					continue
				}
				return result, fmt.Errorf("inspect retired skill home %s: %w", home, err)
			}
			verdict, err := classifyManagedSkillOwnership(home, skill.Skill)
			if err != nil {
				return result, err
			}
			switch verdict.Class {
			case skillOwnershipAbsent:
				if verdict.Claimed {
					if allowDestructive {
						if err := unmanageManagedSkillClaim(home, skill.Skill); err != nil {
							return result, err
						}
						result.Mutated = true
						action.Action = "unmanaged-missing"
						result.Unmanaged = append(result.Unmanaged, action)
					} else {
						action.Action = "confirmation-required"
						result.Skipped = append(result.Skipped, action)
					}
				}
				// Unclaimed absence: omit from the report.
			case skillOwnershipDangling:
				if allowDestructive {
					if verdict.Claimed {
						if err := unmanageManagedSkillClaim(home, skill.Skill); err != nil {
							return result, err
						}
						result.Mutated = true
					}
					action.Action = "dangling"
					result.Unmanaged = append(result.Unmanaged, action)
				} else {
					action.Action = "confirmation-required"
					result.Skipped = append(result.Skipped, action)
				}
			case skillOwnershipMismatch, skillOwnershipUnmanaged, skillOwnershipUnreadable:
				if allowDestructive {
					if verdict.Claimed {
						if err := unmanageManagedSkillClaim(home, skill.Skill); err != nil {
							return result, err
						}
						result.Mutated = true
					}
					if verdict.Legacy {
						action.Action = "legacy-v1"
						action.Reason = legacyV1PreservationReason(action.Reason)
					} else {
						action.Action = "unmanaged"
					}
					result.Unmanaged = append(result.Unmanaged, action)
					continue
				}
				if verdict.Claimed || pathExistsForDeprecation(path) {
					action.Action = "confirmation-required"
					result.Skipped = append(result.Skipped, action)
				}
				// Unclaimed absence under mismatch/unmanaged/unreadable: omit.
			case skillOwnershipOwned:
				if !allowDestructive {
					action.Action = "confirmation-required"
					result.Skipped = append(result.Skipped, action)
					continue
				}
				if err := removeOwnedManagedSkillTree(home, skill.Skill, verdict.Actual); err != nil {
					if err == errMigrationOwnershipLost {
						if claimErr := unmanageManagedSkillClaim(home, skill.Skill); claimErr != nil {
							return result, claimErr
						}
						result.Mutated = true
						action.Action = "unmanaged"
						result.Unmanaged = append(result.Unmanaged, action)
						continue
					}
					return result, err
				}
				if err := unmanageManagedSkillClaim(home, skill.Skill); err != nil {
					return result, err
				}
				result.Mutated = true
				action.Action = "removed"
				result.Removed = append(result.Removed, action)
			default:
				return result, fmt.Errorf("unknown skill ownership class %q for %s", verdict.Class, path)
			}
		}
	}
	for _, agent := range manifest.RetiredAgents {
		for _, rawHome := range agent.AgentHomes {
			home, err := expandInstallDeprecationPath(rawHome, pathContext)
			if err != nil {
				return result, err
			}
			path := filepath.Join(home, agent.Agent+".md")
			action := installDeprecationCleanupAction{
				Kind:    "agent",
				Name:    agent.Agent,
				Path:    path,
				Reason:  agent.Reason,
				Since:   agent.Since,
				Window:  deprecationWindow(agent.Window),
				Signoff: agent.Signoff,
			}
			fileInfo, fileErr := os.Lstat(path)
			if fileErr != nil {
				if os.IsNotExist(fileErr) {
					// Absent retirement: omit.
					continue
				}
				if shouldSurfaceMigrationInspectionError(fileErr) {
					return result, fmt.Errorf("inspect retired agent %s: %w", path, fileErr)
				}
				// Uninspectable without a surfacing I/O error: treat as unowned — omit.
				continue
			}
			if !fileInfo.Mode().IsRegular() || fileInfo.Mode()&os.ModeSymlink != 0 {
				// Not a regular agent file Loaf would retire — omit.
				continue
			}
			if !isLoafOwnedAgentFile(home) {
				// Agent home is not Loaf-owned — omit.
				continue
			}
			if !allowDestructive {
				action.Action = "confirmation-required"
				result.Skipped = append(result.Skipped, action)
				continue
			}
			// Marker proves the harness was once installed by Loaf, not that this
			// agent file is Loaf-authored. Without an agent digest, never delete.
			action.Action = "unmanaged"
			result.Unmanaged = append(result.Unmanaged, action)
		}
	}
	for _, skill := range manifest.ExternalizedSkills {
		for _, rawHome := range skill.SkillHomes {
			home, err := expandInstallDeprecationPath(rawHome, pathContext)
			if err != nil {
				return result, err
			}
			path := filepath.Join(home, skill.Skill)
			dirInfo, dirErr := os.Lstat(path)
			if dirErr != nil {
				if os.IsNotExist(dirErr) {
					continue
				}
				if shouldSurfaceMigrationInspectionError(dirErr) {
					return result, fmt.Errorf("inspect externalized skill %s: %w", path, dirErr)
				}
				continue
			}
			if !dirInfo.IsDir() || dirInfo.Mode()&os.ModeSymlink != 0 {
				continue
			}
			skillMD := filepath.Join(path, "SKILL.md")
			fileInfo, fileErr := os.Lstat(skillMD)
			if fileErr != nil {
				if os.IsNotExist(fileErr) {
					continue
				}
				if shouldSurfaceMigrationInspectionError(fileErr) {
					return result, fmt.Errorf("inspect externalized skill %s: %w", skillMD, fileErr)
				}
				continue
			}
			if !fileInfo.Mode().IsRegular() || fileInfo.Mode()&os.ModeSymlink != 0 {
				continue
			}
			result.Externalized = append(result.Externalized, installDeprecationCleanupAction{
				Kind:    "skill",
				Name:    skill.Skill,
				Path:    path,
				Reason:  skill.Reason,
				Since:   skill.Since,
				Window:  deprecationWindow(skill.Window),
				Signoff: skill.Signoff,
				Source:  skill.Source,
				Command: skill.InstallCommand,
				Action:  "externalized",
			})
		}
	}
	for _, relocation := range manifest.Relocations {
		from, err := expandInstallDeprecationPath(relocation.From, pathContext)
		if err != nil {
			return result, err
		}
		to, err := expandInstallDeprecationPath(relocation.To, pathContext)
		if err != nil {
			return result, err
		}
		ownerMarker := ""
		if relocation.OwnerMarker != "" {
			ownerMarker, err = expandInstallDeprecationPath(relocation.OwnerMarker, pathContext)
			if err != nil {
				return result, err
			}
		}
		action := installDeprecationCleanupAction{
			Kind:    "path",
			Name:    relocation.ID,
			Path:    from + " -> " + to,
			Reason:  relocation.Reason,
			Since:   relocation.Since,
			Window:  deprecationWindow(relocation.Window),
			Signoff: relocation.Signoff,
		}
		if err := requireMigrationSkillHomeRoot(from); err != nil {
			if os.IsNotExist(err) {
				// Absent relocation source: omit.
				continue
			}
			if isDeliberateMigrationRootRefusal(err) {
				// Refused source root: omit.
				continue
			}
			return result, fmt.Errorf("inspect relocation source %s: %w", from, err)
		}
		candidate, candErr := isLoafRelocationCandidate(from, ownerMarker)
		if candErr != nil {
			return result, fmt.Errorf("inspect relocation candidacy for %s: %w", from, candErr)
		}
		if !candidate {
			// Not a Loaf relocation candidate — omit.
			continue
		}
		owned, unmanaged, stranded, err := listRelocationSkillVerdicts(from)
		if err != nil {
			return result, err
		}
		for _, name := range unmanaged {
			actionName := "unmanaged"
			reason := relocation.Reason
			verdict, classErr := classifyManagedSkillOwnership(from, name)
			if classErr == nil && verdict.Legacy {
				actionName = "legacy-v1"
				reason = legacyV1PreservationReason(reason)
			}
			result.Unmanaged = append(result.Unmanaged, installDeprecationCleanupAction{
				Kind:    "skill",
				Name:    name,
				Path:    filepath.Join(from, name),
				Reason:  reason,
				Since:   relocation.Since,
				Window:  deprecationWindow(relocation.Window),
				Signoff: relocation.Signoff,
				Action:  actionName,
			})
		}
		if len(owned) == 0 && len(stranded) == 0 {
			// Candidate home with nothing Loaf would move — omit.
			continue
		}
		if !allowDestructive {
			action.Action = "confirmation-required"
			result.Skipped = append(result.Skipped, action)
			continue
		}
		movedAny := false
		removedStaleAny := false
		for _, name := range owned {
			srcSkill := filepath.Join(from, name)
			verdict, err := classifyManagedSkillOwnership(from, name)
			if err != nil {
				return result, err
			}
			if verdict.Class != skillOwnershipOwned {
				continue
			}
			digest := verdict.Actual
			equivalent, equivErr := destinationHoldsEquivalentOwnedSkill(to, name, digest)
			if equivErr != nil {
				return result, equivErr
			}
			if equivalent {
				// Source may be deleted only when destination demonstrably holds
				// an equivalent Loaf-owned digest match.
				if err := removeOwnedManagedSkillTree(from, name, digest); err != nil {
					if err == errMigrationOwnershipLost {
						if claimErr := unmanageManagedSkillClaim(from, name); claimErr != nil {
							return result, claimErr
						}
						result.Mutated = true
						continue
					}
					return result, err
				}
				if err := unmanageManagedSkillClaim(from, name); err != nil {
					return result, err
				}
				result.Mutated = true
				removedStaleAny = true
				continue
			}
			// Classify the destination root before any child Lstat or MkdirAll —
			// ENOTDIR under a non-directory root is the same refusal class as the
			// root check itself. Absence may still be created below.
			if rootErr := requireMigrationSkillHomeRoot(to); rootErr != nil {
				if isDeliberateMigrationRootRefusal(rootErr) {
					appendRelocationDestinationRefusal(&result, relocation, name, srcSkill,
						"relocation destination refused (symlink or non-directory); source preserved")
					continue
				}
				if !os.IsNotExist(rootErr) {
					return result, fmt.Errorf("relocation destination %s: %w", to, rootErr)
				}
			} else {
				destSkill := filepath.Join(to, name)
				if destInfo, destErr := os.Lstat(destSkill); destErr == nil {
					// Destination path occupied by something that is not an
					// equivalent owned digest — preserve the source copy.
					_ = destInfo
					result.Unmanaged = append(result.Unmanaged, installDeprecationCleanupAction{
						Kind:    "skill",
						Name:    name,
						Path:    srcSkill,
						Reason:  "relocation destination holds non-equivalent content; source preserved",
						Since:   relocation.Since,
						Window:  deprecationWindow(relocation.Window),
						Signoff: relocation.Signoff,
						Action:  "unmanaged",
					})
					continue
				} else if !os.IsNotExist(destErr) {
					if isDeliberateMigrationDestinationPathError(destErr) {
						appendRelocationDestinationRefusal(&result, relocation, name, srcSkill,
							"relocation destination refused (symlink or non-directory); source preserved")
						continue
					}
					return result, destErr
				}
			}
			if err := os.MkdirAll(to, 0o755); err != nil {
				// Create raced with a non-directory appearing, or a parent component
				// is not a directory — same deliberate refusal class.
				if rootErr := requireMigrationSkillHomeRoot(to); isDeliberateMigrationRootRefusal(rootErr) ||
					isDeliberateMigrationDestinationPathError(err) {
					appendRelocationDestinationRefusal(&result, relocation, name, srcSkill,
						"relocation destination refused (symlink or non-directory); source preserved")
					continue
				}
				return result, err
			}
			usable, destErr := migrationDestinationRootUsable(to)
			if destErr != nil {
				return result, fmt.Errorf("relocation destination %s: %w", to, destErr)
			}
			if !usable {
				appendRelocationDestinationRefusal(&result, relocation, name, srcSkill,
					"relocation destination refused (symlink or non-directory); source preserved")
				continue
			}
			destSkill := filepath.Join(to, name)
			// Re-verify immediately before rename (TOCTOU close).
			recheck, err := classifyManagedSkillOwnership(from, name)
			if err != nil {
				return result, err
			}
			if recheck.Class != skillOwnershipOwned || recheck.Actual != digest {
				if recheck.Claimed {
					if err := unmanageManagedSkillClaim(from, name); err != nil {
						return result, err
					}
					result.Mutated = true
				}
				continue
			}
			if err := os.Rename(srcSkill, destSkill); err != nil {
				return result, err
			}
			// Post-rename hash binds the destination claim to the bytes that
			// actually landed. A swap between recheck and rename cannot produce
			// a stale digest claim at the destination.
			landed, hashErr := hashInstallSkillTree(destSkill)
			if hashErr != nil || landed != digest {
				// Leave the tree at the destination unclaimed and drop the
				// source claim so we do not advertise false ownership.
				if claimErr := unmanageManagedSkillClaim(from, name); claimErr != nil {
					return result, fmt.Errorf("post-rename digest mismatch for %s (%v/%s); unmanage source: %w", destSkill, hashErr, landed, claimErr)
				}
				result.Mutated = true
				result.Unmanaged = append(result.Unmanaged, installDeprecationCleanupAction{
					Kind:   "skill",
					Name:   name,
					Path:   destSkill,
					Reason: "post-rename digest verification failed; left unclaimed",
					Action: "unmanaged",
				})
				continue
			}
			if err := claimManagedSkillDigest(to, name, digest); err != nil {
				return result, err
			}
			if err := unmanageManagedSkillClaim(from, name); err != nil {
				return result, err
			}
			result.Mutated = true
			movedAny = true
		}
		for _, name := range stranded {
			completed, err := completeStrandedRelocationClaim(from, to, name)
			if err != nil {
				return result, err
			}
			if completed {
				result.Mutated = true
				continue
			}
			result.Unmanaged = append(result.Unmanaged, installDeprecationCleanupAction{
				Kind:    "skill",
				Name:    name,
				Path:    filepath.Join(from, name),
				Reason:  "relocation destination refused (symlink or non-directory); stranded claim preserved",
				Since:   relocation.Since,
				Window:  deprecationWindow(relocation.Window),
				Signoff: relocation.Signoff,
				Action:  "unmanaged",
			})
		}
		if err := removeEmptySkillHomeIfSafe(from); err != nil {
			return result, err
		}
		if movedAny || removedStaleAny {
			if movedAny {
				action.Action = "relocated"
			} else {
				action.Action = "removed-stale"
			}
			result.Removed = append(result.Removed, action)
		}
	}
	for _, alias := range manifest.Aliases {
		result.Aliases = append(result.Aliases, installDeprecationCleanupAction{
			Kind:    "alias",
			Name:    alias.From,
			Path:    alias.To,
			Reason:  alias.Reason,
			Since:   alias.Since,
			Window:  deprecationWindow(alias.Window),
			Signoff: alias.Signoff,
			Action:  "alias",
		})
	}

	if allowDestructive && result.Mutated {
		after, err := hashMigrationSkillHomes(sweepHomes)
		if err != nil {
			return result, err
		}
		result.Receipt.After = after
		result.Receipt.Mutated = true
		result.Receipt.When = time.Now().UTC().Format(time.RFC3339)
	} else {
		// Clear any partial receipt so dry / no-op passes emit nothing.
		result.Receipt = installMigrationReceipt{}
	}
	return result, nil
}

func deprecationWindow(value string) string {
	if strings.TrimSpace(value) == "" {
		return defaultDeprecationWindow
	}
	return value
}

// isLoafOwnedInstallDir reports directory-level Loaf install markers only.
// SKILL.md presence is never ownership proof — skill ownership requires the
// managed-skills digest manifest (see classifyManagedSkillOwnership).
func isLoafOwnedInstallDir(path string) bool {
	return isRegularLoafMarkerFile(filepath.Join(path, loafInstallMarkerFile)) ||
		isRegularLoafMarkerFile(filepath.Join(filepath.Dir(path), loafInstallMarkerFile))
}

func isLoafOwnedRelocationDir(path string, ownerMarker string) (bool, error) {
	return isLoafRelocationCandidate(path, ownerMarker)
}

func isLoafRelocationCandidate(path string, ownerMarker string) (bool, error) {
	if isLoafOwnedInstallDir(path) {
		return true, nil
	}
	if ownerMarker != "" && isRegularLoafMarkerFile(ownerMarker) {
		return true, nil
	}
	if err := requireMigrationSkillHomeRoot(path); err != nil {
		if os.IsNotExist(err) || isDeliberateMigrationRootRefusal(err) {
			return false, nil
		}
		return false, err
	}
	state, err := readManagedSkillsState(path)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		if shouldSurfaceMigrationInspectionError(err) {
			return false, err
		}
		// Unreadable/invalid claim map is not Loaf candidacy.
		return false, nil
	}
	return len(state.digests) > 0, nil
}

func isLoafOwnedAgentFile(agentHome string) bool {
	return isRegularLoafMarkerFile(filepath.Join(agentHome, loafInstallMarkerFile)) ||
		isRegularLoafMarkerFile(filepath.Join(filepath.Dir(agentHome), loafInstallMarkerFile))
}

func isRegularLoafMarkerFile(path string) bool {
	info, err := os.Lstat(path)
	if err != nil {
		return false
	}
	return info.Mode().IsRegular() && info.Mode()&os.ModeSymlink == 0
}

func installPathContext() map[string]string {
	home := installHome()
	xdgConfig := os.Getenv("XDG_CONFIG_HOME")
	if xdgConfig == "" && home != "" {
		xdgConfig = filepath.Join(home, ".config")
	}
	return map[string]string{
		"HOME":            home,
		"XDG_CONFIG_HOME": xdgConfig,
	}
}

// installHarnessSkillSearchHomes returns every skill home a Loaf-supported
// harness may scan, in migration precedence order. Derived from:
//   - installSkillsDestination (canonical ~/.agents/skills)
//   - ADR-018 / config/deprecations.json relocation sources
//     (~/.config/agents/skills, ${XDG_CONFIG_HOME}/opencode/skills)
//   - documented additional scan paths (Amp higher-precedence home first;
//     Cursor ~/.cursor/skills; OpenCode ~/.claude/skills)
//
// Do not invent further homes here — extend only when the install resolver or
// primary harness docs gain a path.
func installHarnessSkillSearchHomes(pathContext map[string]string) ([]string, error) {
	home := pathContext["HOME"]
	xdgConfig := pathContext["XDG_CONFIG_HOME"]
	if home == "" {
		return nil, fmt.Errorf("cannot derive skill search homes without HOME")
	}
	if xdgConfig == "" {
		xdgConfig = filepath.Join(home, ".config")
	}
	raw := []string{
		filepath.Join(home, ".config", "agents", "skills"), // Amp precedence #1 (shadows canonical)
		filepath.Join(xdgConfig, "opencode", "skills"),     // OpenCode prior home; still scanned
		filepath.Join(home, ".cursor", "skills"),           // Cursor also scans
		filepath.Join(home, ".claude", "skills"),           // OpenCode also scans
		filepath.Join(home, ".agents", "skills"),           // canonical (installSkillsDestination)
	}
	seen := map[string]bool{}
	var homes []string
	for _, path := range raw {
		clean := filepath.Clean(path)
		if seen[clean] {
			continue
		}
		seen[clean] = true
		homes = append(homes, clean)
	}
	return homes, nil
}

func retiredSkillHomes(skill retiredInstallSkill, pathContext map[string]string) ([]string, error) {
	derived, err := installHarnessSkillSearchHomes(pathContext)
	if err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	var homes []string
	add := func(path string) {
		clean := filepath.Clean(path)
		if seen[clean] {
			return
		}
		seen[clean] = true
		homes = append(homes, clean)
	}
	for _, home := range derived {
		add(home)
	}
	for _, raw := range skill.SkillHomes {
		expanded, err := expandInstallDeprecationPath(raw, pathContext)
		if err != nil {
			return nil, err
		}
		add(expanded)
	}
	return homes, nil
}

func skillMigrationSweepHomes(manifest installDeprecationManifest, pathContext map[string]string) ([]string, error) {
	derived, err := installHarnessSkillSearchHomes(pathContext)
	if err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	var homes []string
	add := func(path string) {
		clean := filepath.Clean(path)
		if seen[clean] {
			return
		}
		seen[clean] = true
		homes = append(homes, clean)
	}
	for _, home := range derived {
		add(home)
	}
	for _, skill := range manifest.RetiredSkills {
		for _, raw := range skill.SkillHomes {
			expanded, err := expandInstallDeprecationPath(raw, pathContext)
			if err != nil {
				return nil, err
			}
			add(expanded)
		}
	}
	for _, target := range manifest.RetiredTargets {
		for _, raw := range target.Paths {
			expanded, err := expandInstallDeprecationPath(raw, pathContext)
			if err != nil {
				return nil, err
			}
			add(expanded)
		}
	}
	for _, agent := range manifest.RetiredAgents {
		for _, raw := range agent.AgentHomes {
			expanded, err := expandInstallDeprecationPath(raw, pathContext)
			if err != nil {
				return nil, err
			}
			add(expanded)
			add(filepath.Join(expanded, agent.Agent+".md"))
		}
	}
	for _, relocation := range manifest.Relocations {
		from, err := expandInstallDeprecationPath(relocation.From, pathContext)
		if err != nil {
			return nil, err
		}
		to, err := expandInstallDeprecationPath(relocation.To, pathContext)
		if err != nil {
			return nil, err
		}
		add(from)
		add(to)
	}
	return homes, nil
}

// errMigrationRootRefused is returned when a migration root is a symlink or
// non-directory. Callers must treat this as a deliberate preservation refusal,
// distinct from I/O failures such as EACCES that must surface.
var errMigrationRootRefused = errors.New("migration skill home is not a directory or is a symlink")

func isDeliberateMigrationRootRefusal(err error) bool {
	return errors.Is(err, errMigrationRootRefused)
}

// isDeliberateMigrationDestinationPathError reports ENOTDIR (and wrapped PathError
// forms) from operating under a non-directory destination component. Same refusal
// class as errMigrationRootRefused — preserve and report, do not abort.
func isDeliberateMigrationDestinationPathError(err error) bool {
	if err == nil {
		return false
	}
	if isDeliberateMigrationRootRefusal(err) {
		return true
	}
	return errors.Is(err, syscall.ENOTDIR)
}

// appendRelocationDestinationRefusal records a deliberate destination refusal for
// either an owned source tree or a stranded claim.
func appendRelocationDestinationRefusal(result *installDeprecationCleanupResult, relocation installRelocationManifest, name, path, reason string) {
	result.Unmanaged = append(result.Unmanaged, installDeprecationCleanupAction{
		Kind:    "skill",
		Name:    name,
		Path:    path,
		Reason:  reason,
		Since:   relocation.Since,
		Window:  deprecationWindow(relocation.Window),
		Signoff: relocation.Signoff,
		Action:  "unmanaged",
	})
}

// shouldSurfaceMigrationInspectionError reports OS-level inspection failures
// (permission, path I/O) that must not be flattened into missing/unmanaged/
// unmarked. Parse/content errors stay non-fatal at call sites that already
// treat unreadable claim maps as preservation.
func shouldSurfaceMigrationInspectionError(err error) bool {
	if err == nil || os.IsNotExist(err) || isDeliberateMigrationRootRefusal(err) {
		return false
	}
	if os.IsPermission(err) {
		return true
	}
	var pathErr *os.PathError
	if errors.As(err, &pathErr) {
		return true
	}
	var linkErr *os.LinkError
	if errors.As(err, &linkErr) {
		return true
	}
	return false
}

// requireMigrationSkillHomeRoot mirrors requireInstallSkillTreeRoot: Lstat and
// refuse a symlink or non-directory root. DUPLICATED deliberately —
// install_target.go is owned by a sibling task; consolidate later.
func requireMigrationSkillHomeRoot(root string) error {
	info, err := os.Lstat(root)
	if err != nil {
		return err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("%w: %s", errMigrationRootRefused, root)
	}
	return nil
}

// migrationDirPresent reports a real local directory via Lstat (never follows
// symlinks). Symlinked homes are treated as absent for report-only inspection.
func migrationDirPresent(path string) bool {
	info, err := os.Lstat(path)
	return err == nil && info.IsDir() && info.Mode()&os.ModeSymlink == 0
}

// migrationFilePresent reports a regular local file via Lstat (never follows
// symlinks).
func migrationFilePresent(path string) bool {
	info, err := os.Lstat(path)
	return err == nil && info.Mode().IsRegular() && info.Mode()&os.ModeSymlink == 0
}

func legacyV1PreservationReason(existing string) string {
	note := "pre-v2 name-only claim; left in place because promoting unproven v1 names into digest authority is unsafe"
	if strings.TrimSpace(existing) == "" {
		return note
	}
	return existing + "; " + note
}

// migrationDestinationRootUsable is the destination-side refusal classifier
// shared by every relocation path that would write through `to`. Deliberate
// refusals (symlink / non-directory) and absence are non-fatal; real I/O errors
// surface. Callers must preserve and report on usable=false rather than abort.
func migrationDestinationRootUsable(to string) (usable bool, err error) {
	if err := requireMigrationSkillHomeRoot(to); err != nil {
		if os.IsNotExist(err) || isDeliberateMigrationRootRefusal(err) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

// destinationHoldsEquivalentOwnedSkill is the single rule for deleting a
// relocation source copy: the destination must demonstrably hold a Loaf-owned
// skill whose digest equals expectedDigest. Existence alone is never enough.
func destinationHoldsEquivalentOwnedSkill(to, name, expectedDigest string) (bool, error) {
	if expectedDigest == "" {
		return false, nil
	}
	usable, err := migrationDestinationRootUsable(to)
	if err != nil {
		return false, err
	}
	if !usable {
		return false, nil
	}
	info, err := os.Lstat(filepath.Join(to, name))
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return false, nil
	}
	verdict, err := classifyManagedSkillOwnership(to, name)
	if err != nil {
		return false, err
	}
	return verdict.Class == skillOwnershipOwned && verdict.Actual == expectedDigest, nil
}

func classifyManagedSkillOwnership(skillHome, skill string) (skillOwnershipVerdict, error) {
	if err := requireMigrationSkillHomeRoot(skillHome); err != nil {
		if os.IsNotExist(err) {
			return skillOwnershipVerdict{Class: skillOwnershipAbsent}, nil
		}
		if isDeliberateMigrationRootRefusal(err) {
			// Symlinked/non-dir home: do not follow into an external tree.
			return skillOwnershipVerdict{Class: skillOwnershipUnreadable}, nil
		}
		return skillOwnershipVerdict{}, err
	}
	path := filepath.Join(skillHome, skill)
	info, err := os.Lstat(path)
	claimed := false
	recorded := ""
	state, stateErr := readManagedSkillsState(skillHome)
	if stateErr != nil && !os.IsNotExist(stateErr) {
		if shouldSurfaceMigrationInspectionError(stateErr) {
			return skillOwnershipVerdict{}, stateErr
		}
		if err == nil {
			return skillOwnershipVerdict{Class: skillOwnershipUnreadable, Claimed: false}, nil
		}
	} else if stateErr == nil {
		if digest, ok := state.digests[skill]; ok {
			claimed = true
			recorded = digest
		}
	}
	if err != nil {
		if os.IsNotExist(err) {
			return skillOwnershipVerdict{Class: skillOwnershipAbsent, Claimed: claimed, Recorded: recorded}, nil
		}
		return skillOwnershipVerdict{}, err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		target, readErr := os.Readlink(path)
		if readErr != nil {
			return skillOwnershipVerdict{Class: skillOwnershipDangling, Claimed: claimed, Recorded: recorded}, nil
		}
		resolved := target
		if !filepath.IsAbs(resolved) {
			resolved = filepath.Join(filepath.Dir(path), target)
		}
		if _, statErr := os.Lstat(resolved); os.IsNotExist(statErr) {
			return skillOwnershipVerdict{Class: skillOwnershipDangling, Claimed: claimed, Recorded: recorded}, nil
		}
		return skillOwnershipVerdict{Class: skillOwnershipDangling, Claimed: claimed, Recorded: recorded}, nil
	}
	if state.legacy {
		// Stay at v1 deliberately: name-only claims are not digest authority.
		// Pre-v2 trees are preserved and reported as legacy-v1 rather than
		// promoted into digests Loaf cannot prove.
		return skillOwnershipVerdict{Class: skillOwnershipUnmanaged, Claimed: claimed, Recorded: recorded, Legacy: true}, nil
	}
	if !claimed || recorded == "" {
		return skillOwnershipVerdict{Class: skillOwnershipUnmanaged, Claimed: claimed, Recorded: recorded}, nil
	}
	actual, hashErr := hashInstallSkillTree(path)
	if hashErr != nil {
		return skillOwnershipVerdict{Class: skillOwnershipUnreadable, Claimed: claimed, Recorded: recorded}, nil
	}
	if actual != recorded {
		return skillOwnershipVerdict{Class: skillOwnershipMismatch, Claimed: true, Recorded: recorded, Actual: actual}, nil
	}
	return skillOwnershipVerdict{Class: skillOwnershipOwned, Claimed: true, Recorded: recorded, Actual: actual}, nil
}

var errMigrationOwnershipLost = fmt.Errorf("managed skill ownership lost before mutation")

// quarantinePostRenameHook is a test-only seam invoked after the skill tree is
// renamed into quarantine and before re-hash/revalidation. Production leaves
// it nil.
var quarantinePostRenameHook func(skillHome, skill, quarantine string) error

const loafQuarantineMarker = ".loaf-quarantine-"

// removeOwnedManagedSkillTree re-verifies the digest, renames to an
// unpredictable quarantine path, re-hashes that path, re-hashes again
// immediately before deletion, then RemoveAlls the verified path.
//
// Remaining race windows (stated, not closed):
//
//  1. Swap race: between the final pre-delete re-hash returning success and
//     os.RemoveAll completing, another process can still rename the quarantine
//     away and create a different directory at the same path; RemoveAll would
//     then delete the replacement. Unpredictable quarantine names make a
//     targeted swap impractical, but the path-based RemoveAll API cannot bind
//     deletion to the hashed inode in portable Go.
//  2. Rollback failure: if either revalidation fails and another process has
//     recreated `path`, Rename(quarantine→path) fails and the user's real tree
//     is left under the hidden `.<skill>.loaf-quarantine-<random>` name. That
//     failure must name the quarantine path loudly; subsequent cleanup passes
//     must discover leftover quarantine dirs via reconcileQuarantineOrphans
//     and report them by path. Loaf never renames an orphan back into place —
//     recovery without ownership proof would mutate trees it cannot claim.
func removeOwnedManagedSkillTree(skillHome, skill, expectedDigest string) error {
	if err := requireMigrationSkillHomeRoot(skillHome); err != nil {
		return err
	}
	path := filepath.Join(skillHome, skill)
	recheck, err := classifyManagedSkillOwnership(skillHome, skill)
	if err != nil {
		return err
	}
	if recheck.Class != skillOwnershipOwned || recheck.Actual != expectedDigest {
		return errMigrationOwnershipLost
	}
	suffix, err := randomQuarantineSuffix()
	if err != nil {
		return err
	}
	quarantine := filepath.Join(skillHome, "."+skill+".loaf-quarantine-"+suffix)
	if err := os.Rename(path, quarantine); err != nil {
		return err
	}
	if quarantinePostRenameHook != nil {
		if hookErr := quarantinePostRenameHook(skillHome, skill, quarantine); hookErr != nil {
			// Best-effort rollback so a hook failure does not itself strand the tree.
			_ = os.Rename(quarantine, path)
			return hookErr
		}
	}
	actual, hashErr := hashInstallSkillTree(quarantine)
	if hashErr != nil || actual != expectedDigest {
		if rollbackErr := os.Rename(quarantine, path); rollbackErr != nil {
			return quarantineRollbackFailure(path, quarantine, hashErr, actual, expectedDigest, rollbackErr)
		}
		return errMigrationOwnershipLost
	}
	// Bind removal to the verified tree as tightly as path-based RemoveAll
	// allows: re-hash the same path immediately before delete.
	finalHash, finalErr := hashInstallSkillTree(quarantine)
	if finalErr != nil || finalHash != expectedDigest {
		if rollbackErr := os.Rename(quarantine, path); rollbackErr != nil {
			return quarantineRollbackFailure(path, quarantine, finalErr, finalHash, expectedDigest, rollbackErr)
		}
		return errMigrationOwnershipLost
	}
	if err := os.RemoveAll(quarantine); err != nil {
		return err
	}
	return nil
}

func quarantineRollbackFailure(path, quarantine string, revalidateErr error, got, want string, rollbackErr error) error {
	cause := revalidateErr
	if cause == nil {
		cause = fmt.Errorf("digest %s != expected %s", got, want)
	}
	return fmt.Errorf("quarantine rollback failed: skill tree stranded at %s (could not restore %s): revalidation=%v; rollback=%w",
		quarantine, path, cause, rollbackErr)
}

func randomQuarantineSuffix() (string, error) {
	var buf [16]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf[:]), nil
}

func parseLoafQuarantineDirName(name string) (skill string, ok bool) {
	// Parse from the right so generation (."+skill+".loaf-quarantine-"+suffix)
	// and parsing are exact inverses when skill itself contains the marker.
	if !strings.HasPrefix(name, ".") {
		return "", false
	}
	idx := strings.LastIndex(name, loafQuarantineMarker)
	if idx <= 1 {
		return "", false
	}
	skill = name[1:idx]
	suffix := name[idx+len(loafQuarantineMarker):]
	if skill == "" || suffix == "" {
		return "", false
	}
	for _, r := range suffix {
		if (r < '0' || r > '9') && (r < 'a' || r > 'f') {
			return "", false
		}
	}
	return skill, true
}

// quarantineOrphanScanHomes returns every directory where removeOwnedManagedSkillTree
// can be called — the only places a quarantine dir can be stranded:
//   - harness skill search homes and explicit retired-skill SkillHomes
//     (retired-skill cleanup)
//   - relocation From roots (stale-source deletion after equivalent dest)
//   - retired target paths and their nested skills/ homes
//     (retireMarkedTargetPath → inventory → removeOwnedManagedSkillTree)
//
// Agent homes are excluded: removeOwnedManagedSkillTree is never called there.
func quarantineOrphanScanHomes(manifest installDeprecationManifest, pathContext map[string]string) ([]string, error) {
	derived, err := installHarnessSkillSearchHomes(pathContext)
	if err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	var homes []string
	add := func(path string) {
		clean := filepath.Clean(path)
		if seen[clean] {
			return
		}
		seen[clean] = true
		homes = append(homes, clean)
	}
	for _, home := range derived {
		add(home)
	}
	for _, skill := range manifest.RetiredSkills {
		for _, raw := range skill.SkillHomes {
			expanded, err := expandInstallDeprecationPath(raw, pathContext)
			if err != nil {
				return nil, err
			}
			add(expanded)
		}
	}
	for _, relocation := range manifest.Relocations {
		from, err := expandInstallDeprecationPath(relocation.From, pathContext)
		if err != nil {
			return nil, err
		}
		add(from)
	}
	for _, target := range manifest.RetiredTargets {
		for _, raw := range target.Paths {
			expanded, err := expandInstallDeprecationPath(raw, pathContext)
			if err != nil {
				return nil, err
			}
			add(expanded)
			add(filepath.Join(expanded, "skills"))
		}
	}
	return homes, nil
}

// reconcileQuarantineOrphans finds leftover .<skill>.loaf-quarantine-* trees left
// behind when a deletion rollback could not restore the original path. It reports
// each orphan by path and never renames: a matching directory name is not
// ownership proof, and automatic recovery would mutate trees Loaf cannot claim.
func reconcileQuarantineOrphans(homes []string, result *installDeprecationCleanupResult) error {
	seen := map[string]bool{}
	for _, home := range homes {
		clean := filepath.Clean(home)
		if seen[clean] {
			continue
		}
		seen[clean] = true
		if err := requireMigrationSkillHomeRoot(clean); err != nil {
			if os.IsNotExist(err) || isDeliberateMigrationRootRefusal(err) {
				continue
			}
			return fmt.Errorf("inspect skill home for quarantine orphans %s: %w", clean, err)
		}
		entries, err := os.ReadDir(clean)
		if err != nil {
			if shouldSurfaceMigrationInspectionError(err) {
				return fmt.Errorf("read skill home for quarantine orphans %s: %w", clean, err)
			}
			continue
		}
		for _, entry := range entries {
			skill, ok := parseLoafQuarantineDirName(entry.Name())
			if !ok {
				continue
			}
			quarantine := filepath.Join(clean, entry.Name())
			info, err := os.Lstat(quarantine)
			if err != nil {
				if shouldSurfaceMigrationInspectionError(err) {
					return err
				}
				continue
			}
			if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
				continue
			}
			action := installDeprecationCleanupAction{
				Kind:   "skill",
				Name:   skill,
				Path:   quarantine,
				Reason: "leftover quarantine from a failed ownership-verified deletion rollback; Loaf will not move it",
				Action: "quarantine-orphan",
			}
			result.Unmanaged = append(result.Unmanaged, action)
		}
	}
	return nil
}

func unmanageManagedSkillClaim(skillHome, skill string) error {
	if err := requireMigrationSkillHomeRoot(skillHome); err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	state, err := readManagedSkillsState(skillHome)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	if _, ok := state.digests[skill]; !ok {
		return nil
	}
	delete(state.digests, skill)
	if len(state.digests) == 0 {
		manifestPath := filepath.Join(skillHome, loafSkillManifestFile)
		if info, statErr := os.Lstat(manifestPath); statErr == nil && info.Mode().IsRegular() && info.Mode()&os.ModeSymlink == 0 {
			if removeErr := os.Remove(manifestPath); removeErr != nil && !os.IsNotExist(removeErr) {
				return removeErr
			}
		}
		return nil
	}
	// Stay at v1 when the source was legacy (Decision, round 1 — do not reverse):
	// writing v2 with empty digests produces an unreadable claim map, and
	// computing digests here would promote unproven v1 names into digest
	// authority over possibly-foreign trees. Pre-v2 trees are deliberately
	// left in place; callers must surface them in the cleanup report as
	// legacy-v1 rather than silently skipping forever.
	if state.legacy {
		return writeManagedSkillsManifestV1(skillHome, sortedSkillNames(state.digests))
	}
	return writeManagedSkillsDigestMap(skillHome, state.digests)
}

func claimManagedSkillDigest(skillHome, skill, digest string) error {
	if err := requireMigrationSkillHomeRoot(skillHome); err != nil {
		return err
	}
	if digest == "" || len(digest) != 64 {
		return fmt.Errorf("refusing to claim skill %q with invalid digest", skill)
	}
	state, err := readManagedSkillsState(skillHome)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	if state.digests == nil {
		state.digests = map[string]string{}
	}
	if state.legacy {
		// Destination gains a real digest claim — upgrade survivors that already
		// have digests and convert this write to v2 for the new claim set.
		upgraded := map[string]string{}
		for name, existing := range state.digests {
			if existing != "" {
				upgraded[name] = existing
				continue
			}
			path := filepath.Join(skillHome, name)
			if actual, hashErr := hashInstallSkillTree(path); hashErr == nil {
				upgraded[name] = actual
			}
		}
		upgraded[skill] = digest
		return writeManagedSkillsDigestMap(skillHome, upgraded)
	}
	state.digests[skill] = digest
	return writeManagedSkillsDigestMap(skillHome, state.digests)
}

func writeManagedSkillsDigestMap(skillHome string, digests map[string]string) error {
	names := sortedSkillNames(digests)
	manifest := managedSkillsManifestV2{Version: 2, Skills: make([]managedSkillDigest, 0, len(names))}
	for _, name := range names {
		manifest.Skills = append(manifest.Skills, managedSkillDigest{Name: name, SHA256: digests[name]})
	}
	return writeManagedSkillsManifest(skillHome, manifest)
}

func writeManagedSkillsManifestV1(skillHome string, names []string) error {
	sort.Strings(names)
	body, err := json.MarshalIndent(map[string]any{"version": 1, "skills": names}, "", "  ")
	if err != nil {
		return err
	}
	body = append(body, '\n')
	path := filepath.Join(skillHome, loafSkillManifestFile)
	if info, err := os.Lstat(path); err == nil {
		if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("managed skills manifest %s must be a regular file", path)
		}
	} else if !os.IsNotExist(err) {
		return err
	}
	return writeFileAtomically(path, body, 0o644)
}

func sortedSkillNames(digests map[string]string) []string {
	names := make([]string, 0, len(digests))
	for name := range digests {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func listRelocationSkillVerdicts(from string) (owned []string, unmanaged []string, stranded []string, err error) {
	if err := requireMigrationSkillHomeRoot(from); err != nil {
		return nil, nil, nil, err
	}
	entries, err := os.ReadDir(from)
	if err != nil {
		return nil, nil, nil, err
	}
	seen := map[string]bool{}
	for _, entry := range entries {
		name := entry.Name()
		if name == loafSkillManifestFile || name == loafInstallMarkerFile {
			full := filepath.Join(from, name)
			info, statErr := os.Lstat(full)
			if statErr != nil {
				return nil, nil, nil, statErr
			}
			// Regular Loaf control files are not skills; foreign dirs under the
			// same names are unmanaged content that must survive cleanup.
			if info.Mode().IsRegular() && info.Mode()&os.ModeSymlink == 0 {
				continue
			}
		}
		info, err := os.Lstat(filepath.Join(from, name))
		if err != nil {
			return nil, nil, nil, err
		}
		if !info.IsDir() && info.Mode()&os.ModeSymlink == 0 {
			continue
		}
		seen[name] = true
		verdict, err := classifyManagedSkillOwnership(from, name)
		if err != nil {
			return nil, nil, nil, err
		}
		switch verdict.Class {
		case skillOwnershipOwned:
			owned = append(owned, name)
		case skillOwnershipAbsent:
			// skip
		default:
			unmanaged = append(unmanaged, name)
		}
	}
	state, stateErr := readManagedSkillsState(from)
	if stateErr != nil && !os.IsNotExist(stateErr) {
		if shouldSurfaceMigrationInspectionError(stateErr) {
			return nil, nil, nil, stateErr
		}
		// Invalid claim map: skip stranded detection; directory entries already classified.
	} else if stateErr == nil {
		for name := range state.digests {
			if seen[name] {
				continue
			}
			verdict, err := classifyManagedSkillOwnership(from, name)
			if err != nil {
				return nil, nil, nil, err
			}
			if verdict.Class == skillOwnershipAbsent && verdict.Claimed {
				stranded = append(stranded, name)
			}
		}
	}
	sort.Strings(owned)
	sort.Strings(unmanaged)
	sort.Strings(stranded)
	return owned, unmanaged, stranded, nil
}

// completeStrandedRelocationClaim tries to finish a claim that already landed at
// to. Returns completed=false (and nil error) when the destination root is a
// deliberate refusal — caller must preserve and report, not abort.
func completeStrandedRelocationClaim(from, to, name string) (completed bool, err error) {
	state, err := readManagedSkillsState(from)
	if err != nil {
		return false, err
	}
	recorded := state.digests[name]
	if recorded == "" {
		if err := unmanageManagedSkillClaim(from, name); err != nil {
			return false, err
		}
		return true, nil
	}
	// Usability before any create/write: a non-directory destination must
	// refuse without MkdirAll aborting the upgrade.
	if rootErr := requireMigrationSkillHomeRoot(to); rootErr != nil {
		if isDeliberateMigrationRootRefusal(rootErr) {
			return false, nil
		}
		if !os.IsNotExist(rootErr) {
			return false, rootErr
		}
		if err := os.MkdirAll(to, 0o755); err != nil {
			if checkErr := requireMigrationSkillHomeRoot(to); isDeliberateMigrationRootRefusal(checkErr) ||
				isDeliberateMigrationDestinationPathError(err) {
				return false, nil
			}
			return false, err
		}
	}
	usable, destErr := migrationDestinationRootUsable(to)
	if destErr != nil {
		return false, destErr
	}
	if !usable {
		return false, nil
	}
	destSkill := filepath.Join(to, name)
	actual, hashErr := hashInstallSkillTree(destSkill)
	if hashErr == nil && actual == recorded {
		if err := claimManagedSkillDigest(to, name, recorded); err != nil {
			return false, err
		}
	}
	if err := unmanageManagedSkillClaim(from, name); err != nil {
		return false, err
	}
	return true, nil
}

func removeEmptySkillHomeIfSafe(path string) error {
	entries, err := os.ReadDir(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	hasPreservedClaims := false
	for _, entry := range entries {
		name := entry.Name()
		full := filepath.Join(path, name)
		info, err := os.Lstat(full)
		if err != nil {
			return err
		}
		if name == loafSkillManifestFile && info.Mode().IsRegular() && info.Mode()&os.ModeSymlink == 0 {
			// A remaining digest claim is not empty — e.g. a stranded claim left
			// in place after a deliberate destination refusal.
			state, stateErr := readManagedSkillsState(path)
			if stateErr == nil && len(state.digests) > 0 {
				hasPreservedClaims = true
			}
			continue
		}
		if name == loafInstallMarkerFile && info.Mode().IsRegular() && info.Mode()&os.ModeSymlink == 0 {
			continue
		}
		return nil
	}
	if hasPreservedClaims {
		return nil
	}
	return os.RemoveAll(path)
}

// retireMarkedTargetPath removes only artifacts Loaf can prove it wrote under
// a marked target directory. Foreign content always survives. Returns whether
// anything was removed and whether foreign content remains.
func retireMarkedTargetPath(path string) (removed bool, foreign bool, err error) {
	if err := requireMigrationSkillHomeRoot(path); err != nil {
		if isDeliberateMigrationRootRefusal(err) || os.IsNotExist(err) {
			return false, true, nil
		}
		return false, false, err
	}
	proven, foreign, err := inventoryRetiredTargetArtifacts(path)
	if err != nil {
		return false, false, err
	}
	for _, artifact := range proven {
		info, statErr := os.Lstat(artifact)
		if statErr != nil {
			if os.IsNotExist(statErr) {
				continue
			}
			return removed, foreign, statErr
		}
		if info.IsDir() && info.Mode()&os.ModeSymlink == 0 {
			base := filepath.Base(artifact)
			home := filepath.Dir(artifact)
			verdict, classErr := classifyManagedSkillOwnership(home, base)
			if classErr != nil {
				return removed, foreign, classErr
			}
			if verdict.Class != skillOwnershipOwned {
				foreign = true
				continue
			}
			if remErr := removeOwnedManagedSkillTree(home, base, verdict.Actual); remErr != nil {
				if remErr == errMigrationOwnershipLost {
					foreign = true
					_ = unmanageManagedSkillClaim(home, base)
					removed = true
					continue
				}
				return removed, foreign, remErr
			}
			_ = unmanageManagedSkillClaim(home, base)
			removed = true
			continue
		}
		if info.Mode().IsRegular() && info.Mode()&os.ModeSymlink == 0 {
			if remErr := os.Remove(artifact); remErr != nil && !os.IsNotExist(remErr) {
				return removed, foreign, remErr
			}
			removed = true
		}
	}
	skillsHome := filepath.Join(path, "skills")
	if migrationDirPresent(skillsHome) {
		if remErr := removeEmptySkillHomeIfSafe(skillsHome); remErr != nil {
			return removed, foreign, remErr
		}
	}
	if remErr := removeEmptySkillHomeIfSafe(path); remErr != nil {
		return removed, foreign, remErr
	}
	if !pathExistsForDeprecation(path) {
		return true, false, nil
	}
	_, stillForeign, invErr := inventoryRetiredTargetArtifacts(path)
	if invErr != nil {
		return removed, foreign, invErr
	}
	return removed, foreign || stillForeign, nil
}

func inventoryRetiredTargetArtifacts(path string) (proven []string, foreign bool, err error) {
	entries, err := os.ReadDir(path)
	if err != nil {
		return nil, false, err
	}
	for _, entry := range entries {
		name := entry.Name()
		full := filepath.Join(path, name)
		info, err := os.Lstat(full)
		if err != nil {
			return nil, false, err
		}
		switch {
		case name == loafInstallMarkerFile || name == loafSkillManifestFile:
			// Reserved Loaf control namespace: under a marked retired target,
			// any regular file with these exact basenames is claimed
			// unconditionally — content is not re-validated. Users and vendors
			// must not place files of these names in Loaf-managed target trees.
			// Symlinks and non-regular nodes are treated as foreign.
			if info.Mode().IsRegular() && info.Mode()&os.ModeSymlink == 0 {
				proven = append(proven, full)
			} else {
				foreign = true
			}
		case name == "skills" && info.IsDir() && info.Mode()&os.ModeSymlink == 0:
			nestedProven, nestedForeign, nestedErr := inventorySkillHomeArtifacts(full)
			if nestedErr != nil {
				return nil, false, nestedErr
			}
			proven = append(proven, nestedProven...)
			if nestedForeign {
				foreign = true
			}
		case info.IsDir() && info.Mode()&os.ModeSymlink == 0:
			verdict, classErr := classifyManagedSkillOwnership(path, name)
			if classErr != nil {
				return nil, false, classErr
			}
			if verdict.Class == skillOwnershipOwned {
				proven = append(proven, full)
			} else {
				foreign = true
			}
		default:
			foreign = true
		}
	}
	return proven, foreign, nil
}

func inventorySkillHomeArtifacts(skillHome string) (proven []string, foreign bool, err error) {
	if err := requireMigrationSkillHomeRoot(skillHome); err != nil {
		if isDeliberateMigrationRootRefusal(err) || os.IsNotExist(err) {
			return nil, true, nil
		}
		return nil, false, err
	}
	entries, err := os.ReadDir(skillHome)
	if err != nil {
		return nil, false, err
	}
	for _, entry := range entries {
		name := entry.Name()
		full := filepath.Join(skillHome, name)
		info, err := os.Lstat(full)
		if err != nil {
			return nil, false, err
		}
		if name == loafInstallMarkerFile || name == loafSkillManifestFile {
			// Same reserved-namespace claim as inventoryRetiredTargetArtifacts.
			if info.Mode().IsRegular() && info.Mode()&os.ModeSymlink == 0 {
				proven = append(proven, full)
			} else {
				foreign = true
			}
			continue
		}
		if !info.IsDir() && info.Mode()&os.ModeSymlink == 0 {
			foreign = true
			continue
		}
		verdict, classErr := classifyManagedSkillOwnership(skillHome, name)
		if classErr != nil {
			return nil, false, classErr
		}
		if verdict.Class == skillOwnershipOwned {
			proven = append(proven, full)
		} else {
			foreign = true
		}
	}
	return proven, foreign, nil
}

func pathExistsForDeprecation(path string) bool {
	_, err := os.Lstat(path)
	return err == nil
}

func persistInstallMigrationReceipt(home string, receipt installMigrationReceipt) error {
	if home == "" || !receipt.Mutated {
		return nil
	}
	if err := refuseSymlinkedMigrationReceiptParents(home); err != nil {
		return err
	}
	dir := filepath.Join(home, ".agents", "loaf", migrationReceiptDirName)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	// Re-check after MkdirAll: a race could have swapped in a symlink parent.
	if err := refuseSymlinkedMigrationReceiptParents(home); err != nil {
		return err
	}
	body, err := json.MarshalIndent(receipt, "", "  ")
	if err != nil {
		return err
	}
	body = append(body, '\n')
	stamp := time.Now().UTC().Format("20060102T150405Z")
	if err := writeFileAtomically(filepath.Join(dir, stamp+".json"), body, 0o644); err != nil {
		return err
	}
	return writeFileAtomically(filepath.Join(dir, "latest.json"), body, 0o644)
}

// refuseSymlinkedMigrationReceiptParents refuses to persist evidence through
// any symlink in $HOME/.agents/loaf[/migration-receipts]. A symlinked path
// would let migration write into another vendor's state directory.
func refuseSymlinkedMigrationReceiptParents(home string) error {
	candidates := []string{
		filepath.Join(home, ".agents"),
		filepath.Join(home, ".agents", "loaf"),
		filepath.Join(home, ".agents", "loaf", migrationReceiptDirName),
	}
	for _, path := range candidates {
		info, err := os.Lstat(path)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("migration receipt path %s is a symlink; refusing to persist", path)
		}
		if !info.IsDir() {
			return fmt.Errorf("migration receipt path %s is not a directory; refusing to persist", path)
		}
	}
	return nil
}

func hashMigrationSkillHomes(homes []string) (string, error) {
	hash := sha256.New()
	for _, home := range homes {
		if _, err := hash.Write([]byte(home)); err != nil {
			return "", err
		}
		if _, err := hash.Write([]byte{0}); err != nil {
			return "", err
		}
		digest, err := hashMigrationSkillHome(home)
		if err != nil {
			return "", err
		}
		if _, err := hash.Write([]byte(digest)); err != nil {
			return "", err
		}
		if _, err := hash.Write([]byte{0}); err != nil {
			return "", err
		}
	}
	return fmt.Sprintf("%x", hash.Sum(nil)), nil
}

// hashMigrationSkillHome is symlink-tolerant receipt hashing for a skill home.
// Symlinked homes are followed (including multi-hop chains, capped) for receipt
// content — mutations refuse them via requireMigrationSkillHomeRoot, but the
// receipt must still observe real before/after bytes.
func hashMigrationSkillHome(root string) (string, error) {
	walkRoot, info, err := resolveMigrationReceiptWalkRoot(root)
	if err != nil {
		if os.IsNotExist(err) {
			return "absent", nil
		}
		return "", err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		// resolveMigrationReceiptWalkRoot returns a dangling or non-dir link
		// as a symlink FileInfo with walkRoot still at the unresolved path.
		target, readErr := os.Readlink(walkRoot)
		if readErr != nil {
			return "", readErr
		}
		hash := sha256.New()
		var rootPermissions [4]byte
		binary.BigEndian.PutUint32(rootPermissions[:], uint32(info.Mode().Perm()))
		if err := writeInstallTreeFrame(hash, 's', rootPermissions[:]); err != nil {
			return "", err
		}
		if err := writeInstallTreeFrame(hash, 'l', []byte(target)); err != nil {
			return "", err
		}
		return fmt.Sprintf("%x", hash.Sum(nil)), nil
	}
	if !info.IsDir() {
		hash := sha256.New()
		body, readErr := os.ReadFile(root)
		if readErr != nil {
			if os.IsNotExist(readErr) {
				return "absent", nil
			}
			return "", readErr
		}
		sum := sha256.Sum256(body)
		var permissions [4]byte
		binary.BigEndian.PutUint32(permissions[:], uint32(info.Mode().Perm()))
		if err := writeInstallTreeFrame(hash, 'f', []byte("."), permissions[:], sum[:]); err != nil {
			return "", err
		}
		return fmt.Sprintf("%x", hash.Sum(nil)), nil
	}
	hash := sha256.New()
	var rootPermissions [4]byte
	binary.BigEndian.PutUint32(rootPermissions[:], uint32(info.Mode().Perm()))
	if err := writeInstallTreeFrame(hash, 'r', rootPermissions[:]); err != nil {
		return "", err
	}
	err = filepath.WalkDir(walkRoot, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		info, err := os.Lstat(path)
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(walkRoot, path)
		if err != nil {
			return err
		}
		if rel == "." {
			return nil
		}
		if info.Mode()&os.ModeSymlink != 0 {
			target, readErr := os.Readlink(path)
			if readErr != nil {
				target = ""
			}
			var permissions [4]byte
			binary.BigEndian.PutUint32(permissions[:], uint32(info.Mode().Perm()))
			return writeInstallTreeFrame(hash, 'l', []byte(filepath.ToSlash(rel)), permissions[:], []byte(target))
		}
		if info.IsDir() {
			var permissions [4]byte
			binary.BigEndian.PutUint32(permissions[:], uint32(info.Mode().Perm()))
			return writeInstallTreeFrame(hash, 'd', []byte(filepath.ToSlash(rel)), permissions[:])
		}
		if !info.Mode().IsRegular() {
			return writeInstallTreeFrame(hash, 'x', []byte(filepath.ToSlash(rel)))
		}
		body, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		sum := sha256.Sum256(body)
		var permissions [4]byte
		binary.BigEndian.PutUint32(permissions[:], uint32(info.Mode().Perm()))
		return writeInstallTreeFrame(hash, 'f', []byte(filepath.ToSlash(rel)), permissions[:], sum[:])
	})
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%x", hash.Sum(nil)), nil
}

const migrationReceiptSymlinkHopLimit = 64

// resolveMigrationReceiptWalkRoot follows symlink chains up to
// migrationReceiptSymlinkHopLimit. Returns a symlink FileInfo when the chain
// ends at a dangling or non-directory link so callers can hash link metadata.
func resolveMigrationReceiptWalkRoot(root string) (string, os.FileInfo, error) {
	current := root
	var info os.FileInfo
	var err error
	for hop := 0; hop < migrationReceiptSymlinkHopLimit; hop++ {
		info, err = os.Lstat(current)
		if err != nil {
			return "", nil, err
		}
		if info.Mode()&os.ModeSymlink == 0 {
			return current, info, nil
		}
		target, readErr := os.Readlink(current)
		if readErr != nil {
			return "", nil, readErr
		}
		if !filepath.IsAbs(target) {
			target = filepath.Join(filepath.Dir(current), target)
		}
		nextInfo, nextErr := os.Lstat(target)
		if nextErr != nil {
			if os.IsNotExist(nextErr) {
				return current, info, nil
			}
			return "", nil, nextErr
		}
		if !nextInfo.IsDir() && nextInfo.Mode()&os.ModeSymlink == 0 {
			// Non-directory leaf: hash as link metadata at current.
			return current, info, nil
		}
		current = target
		info = nextInfo
		if info.Mode()&os.ModeSymlink == 0 {
			return current, info, nil
		}
	}
	return "", nil, fmt.Errorf("migration receipt walk of %s exceeded symlink hop limit %d", root, migrationReceiptSymlinkHopLimit)
}

func expandInstallDeprecationPath(path string, context map[string]string) (string, error) {
	if path == "" {
		return "", fmt.Errorf("install deprecation path cannot be empty")
	}
	expanded := path
	if strings.HasPrefix(expanded, "~/") {
		home := context["HOME"]
		if home == "" {
			return "", fmt.Errorf("cannot expand %q without HOME", path)
		}
		expanded = filepath.Join(home, strings.TrimPrefix(expanded, "~/"))
	}
	for key, value := range context {
		if value == "" {
			continue
		}
		expanded = strings.ReplaceAll(expanded, "${"+key+"}", value)
		expanded = strings.ReplaceAll(expanded, "$"+key, value)
	}
	if !filepath.IsAbs(expanded) {
		return "", fmt.Errorf("install deprecation path %q must expand to an absolute path", path)
	}
	return filepath.Clean(expanded), nil
}

func writeInstallDeprecationCleanup(out io.Writer, result installDeprecationCleanupResult) {
	if len(result.Removed) == 0 && len(result.Unmanaged) == 0 && len(result.Externalized) == 0 && len(result.Aliases) == 0 && len(result.Skipped) == 0 {
		return
	}
	fmt.Fprintf(out, "  %s install deprecation cleanup\n", ansiGray("•"))
	if result.Mutated && (result.Receipt.Before != "" || result.Receipt.After != "") {
		fmt.Fprintf(out, "    %s migration receipt before=%s after=%s\n", ansiGray("•"), result.Receipt.Before, result.Receipt.After)
	}
	for _, action := range result.Removed {
		switch action.Action {
		case "relocated":
			fmt.Fprintf(out, "    %s relocated %s %s at %s", ansiGreen("✓"), action.Kind, action.Name, ansiGray(action.Path))
		case "removed-stale":
			fmt.Fprintf(out, "    %s removed stale relocated %s %s at %s", ansiGreen("✓"), action.Kind, action.Name, ansiGray(action.Path))
		default:
			fmt.Fprintf(out, "    %s removed retired %s %s at %s", ansiGreen("✓"), action.Kind, action.Name, ansiGray(action.Path))
		}
		writeInstallDeprecationMetadata(out, action)
		fmt.Fprintln(out)
	}
	for _, action := range result.Unmanaged {
		switch action.Action {
		case "dangling":
			fmt.Fprintf(out, "    %s un-managed dangling %s %s at %s; left in place", ansiYellow("⚠"), action.Kind, action.Name, ansiGray(action.Path))
		case "unmanaged-missing":
			fmt.Fprintf(out, "    %s un-managed missing %s %s at %s; stopped claiming it", ansiYellow("⚠"), action.Kind, action.Name, ansiGray(action.Path))
		case "legacy-v1":
			fmt.Fprintf(out, "    %s un-managed legacy-v1 %s %s at %s; left in place (pre-v2 trees are not promoted to digest authority)", ansiYellow("⚠"), action.Kind, action.Name, ansiGray(action.Path))
		case "quarantine-orphan":
			fmt.Fprintf(out, "    %s quarantine orphan %s %s stranded at %s; Loaf will not move it — inspect and restore or delete it yourself", ansiYellow("⚠"), action.Kind, action.Name, ansiGray(action.Path))
		default:
			fmt.Fprintf(out, "    %s un-managed %s %s at %s; left in place (ownership not proven)", ansiYellow("⚠"), action.Kind, action.Name, ansiGray(action.Path))
		}
		writeInstallDeprecationMetadata(out, action)
		fmt.Fprintln(out)
	}
	for _, action := range result.Externalized {
		fmt.Fprintf(out, "    %s externalized %s %s at %s", ansiGray("-"), action.Kind, action.Name, ansiGray(action.Path))
		writeInstallDeprecationMetadata(out, action)
		if action.Source != "" {
			fmt.Fprintf(out, " source: %s", action.Source)
		}
		if action.Command != "" {
			fmt.Fprintf(out, " command: %s", action.Command)
		}
		fmt.Fprintln(out)
	}
	for _, action := range result.Aliases {
		fmt.Fprintf(out, "    %s alias %s -> %s", ansiGray("-"), action.Name, action.Path)
		writeInstallDeprecationMetadata(out, action)
		fmt.Fprintln(out)
	}
	// "missing" and "unmarked" are deliberately omitted from the action model:
	// absent retirements are no-ops, and unowned paths are never Loaf's to mention.
	for _, action := range result.Skipped {
		switch action.Action {
		case "confirmation-required":
			fmt.Fprintf(out, "    %s skipped %s %s at %s; rerun with --yes to apply destructive deprecation cleanup\n", ansiYellow("⚠"), action.Kind, action.Name, ansiGray(action.Path))
		}
	}
}

func writeInstallDeprecationMetadata(out io.Writer, action installDeprecationCleanupAction) {
	if action.Reason != "" {
		fmt.Fprintf(out, " — %s", action.Reason)
	}
	if action.Since != "" || action.Window != "" {
		fmt.Fprintf(out, " (since %s, window %s)", emptyInstallDeprecationField(action.Since), emptyInstallDeprecationField(action.Window))
	}
	if action.Signoff != "" {
		fmt.Fprintf(out, " [signoff: %s]", action.Signoff)
	}
}

func emptyInstallDeprecationField(value string) string {
	if strings.TrimSpace(value) == "" {
		return "unspecified"
	}
	return value
}
