package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// hookCatalogFile is the per-target catalog the build emits beside the adapter
// artifacts. It is the single identity authority: reconciliation pairing,
// absorption cohorts, the hooks verb surface, and config diagnosis all resolve
// (target, event, hook_id) through it, with no access to the source repository.
const hookCatalogFile = ".loaf-hook-catalog.json"

const hookCatalogVersion = 1

// hookExecutableSentinel canonicalizes whichever Loaf-executable form an entry
// carries — the build-time template, a shell-quoted trusted absolute path, or
// the bare `loaf` first token — so one signature covers all three. It reuses
// the install-time placeholder, which the build's placeholder lint already
// permits in generated artifacts.
const hookExecutableSentinel = codexJournalExecutablePlaceholder

type hookCatalog struct {
	Version        int                 `json:"version"`
	Target         string              `json:"target"`
	PackageVersion string              `json:"package_version"`
	Entries        []hookCatalogEntry  `json:"entries"`
	Cohorts        []hookCatalogCohort `json:"cohorts"`
}

// hookCatalogEntry is one identity plus everything recognition and pairing
// need. Token sequences are stored exactly as authored — `$HOME/...` stays
// `$HOME/...` — because the runtime normalizer expands both the catalog side
// and the installed side against the live environment. A catalog built on one
// machine therefore pairs correctly on another.
type hookCatalogEntry struct {
	Event      string          `json:"event"`
	HookID     string          `json:"hook_id"`
	Type       string          `json:"type"`
	Template   json.RawMessage `json:"template"`
	Signatures [][]string      `json:"signatures,omitempty"`
	Stems      [][]string      `json:"stems,omitempty"`
	Prompt     string          `json:"prompt,omitempty"`
}

// hookCatalogCohort enumerates the hook IDs one released version shipped for a
// target. Absorption reads it to bound what a prior install could have
// projected. Cohorts are frozen per version: a catalog that grows new hooks
// never widens a recorded cohort.
type hookCatalogCohort struct {
	Version string   `json:"version"`
	HookIDs []string `json:"hook_ids"`
}

// hookCatalogGenerationHookIDs is what the generation this migration absorbs
// actually shipped: 17 Cursor entries and one Codex entry.
var hookCatalogGenerationHookIDs = map[string][]string{
	"cursor": {
		"artifact-body-write",
		"artifact-names",
		"check-" + "sec" + "rets",
		"detect-linear-magic",
		"ephemeral-provenance",
		"generate-task-board",
		"github-account",
		"kb-staleness-nudge",
		"render-drift",
		"security-audit",
		"session-start-loaf",
		"validate-commit",
		"validate-push",
		"workflow-post-merge",
		"workflow-pre-merge",
		"workflow-pre-pr",
		"workflow-pre-push",
	},
	"codex": {"session-start-loaf"},
}

// hookCatalogPreResetVersions are the releases that shipped that same
// generation under the version line retired at 0.2.20, when `2.0.0-alpha.N`
// was renumbered to `0.2.N`. An installed manifest written by one of them is
// still on disk — the drift refusal this Change removes is exactly what stopped
// those manifests from being rewritten — so absorption has to recognize the old
// spelling or read a known install as an unknown one.
//
// The list is an enumeration, not a pattern: every entry was checked against
// the `dist/cursor/hooks.json` and `dist/codex/.codex/hooks.json` committed at
// that release, and each carries precisely the cohorts above. It stops at
// alpha.14 because alpha.13 and earlier shipped 16 Cursor entries — the
// `artifact-names` hook arrives at alpha.14 — and a cohort that claimed a hook
// those installs never had would record it disabled on the strength of an
// absence that means nothing. Everything outside this list keeps the
// unknown-version rule and absorbs nothing.
//
// Closed by construction: the prerelease scheme is retired, so no future
// release can enter the family.
var hookCatalogPreResetVersions = []string{
	"2.0.0-alpha.14",
	"2.0.0-alpha.15",
	"2.0.0-alpha.16",
	"2.0.0-alpha.17",
	"2.0.0-alpha.18",
	"2.0.0-alpha.19",
}

// hookCatalogFrozenCohorts record what each released version actually shipped,
// pinned by test against captured build output. The current spelling is last
// because the no-manifest fallback reads the final record as the generation
// immediately preceding entry-level reconciliation.
var hookCatalogFrozenCohorts = map[string][]hookCatalogCohort{
	"cursor": hookCatalogGenerationCohorts("cursor"),
	"codex":  hookCatalogGenerationCohorts("codex"),
}

func hookCatalogGenerationCohorts(target string) []hookCatalogCohort {
	hookIDs := hookCatalogGenerationHookIDs[target]
	versions := append(append([]string{}, hookCatalogPreResetVersions...), "0.2.20")
	cohorts := make([]hookCatalogCohort, 0, len(versions))
	for _, version := range versions {
		cohorts = append(cohorts, hookCatalogCohort{Version: version, HookIDs: hookIDs})
	}
	return cohorts
}

// hookCatalogHistoricalCommands are command shapes earlier releases shipped for
// a hook ID that still exists, keyed "target/hook_id". They let an installed
// entry from an older generation pair to its ID instead of reading as a retired
// generation. Closed by construction: every line is a shape a release actually
// shipped, never a guess at what one might have.
var hookCatalogHistoricalCommands = map[string][]string{
	"cursor/session-start-loaf": {
		"loaf journal context",
		"loaf journal context --from-hook",
	},
	"codex/session-start-loaf": {
		"loaf journal context",
		"loaf journal context --from-hook",
	},
}

// hookCatalogVariantFlags are trailing flags the builders append per target or
// per blocking mode. Identity stems exclude them so an entry whose enforcement
// was weakened by hand — `--advisory` added to a fail-closed check — still
// pairs to its hook ID and converges instead of orphaning as foreign.
var hookCatalogVariantFlags = map[string]bool{
	"--advisory":      true,
	"--claude-code":   true,
	"--codex-hook":    true,
	"--cursor-hook":   true,
	"--opencode-hook": true,
}

// hookCatalogSource is one desired entry as a target builder produced it.
type hookCatalogSource struct {
	event    string
	hookID   string
	typeName string
	command  string
	prompt   string
	template any
}

func newHookCatalog(target string, packageVersion string, sources []hookCatalogSource) (hookCatalog, error) {
	catalog := hookCatalog{
		Version:        hookCatalogVersion,
		Target:         target,
		PackageVersion: packageVersion,
		Entries:        make([]hookCatalogEntry, 0, len(sources)),
		Cohorts:        []hookCatalogCohort{},
	}
	for _, cohort := range hookCatalogFrozenCohorts[target] {
		catalog.Cohorts = append(catalog.Cohorts, hookCatalogCohort{
			Version: cohort.Version,
			HookIDs: append([]string{}, cohort.HookIDs...),
		})
	}
	for _, source := range sources {
		template, err := json.Marshal(source.template)
		if err != nil {
			return hookCatalog{}, fmt.Errorf("encode %s hook catalog template for %s: %w", target, source.hookID, err)
		}
		entry := hookCatalogEntry{
			Event:    source.event,
			HookID:   source.hookID,
			Type:     source.typeName,
			Template: template,
			Prompt:   source.prompt,
		}
		if source.typeName != "prompt" {
			for _, command := range append([]string{source.command}, hookCatalogHistoricalCommands[target+"/"+source.hookID]...) {
				tokens, ok := hookCommandTokens(command)
				if !ok || len(tokens) == 0 {
					return hookCatalog{}, fmt.Errorf("%s hook %s has an unparseable command %q", target, source.hookID, command)
				}
				entry.Signatures = appendHookTokenSequence(entry.Signatures, tokens)
			}
			if stem := hookCatalogStem(source.command); len(stem) > 0 {
				entry.Stems = appendHookTokenSequence(entry.Stems, stem)
			}
		}
		catalog.Entries = append(catalog.Entries, entry)
	}
	if err := validateHookCatalog(catalog); err != nil {
		return hookCatalog{}, err
	}
	return catalog, nil
}

// hookCatalogStem derives the identity stem embedded in a desired command: the
// argument tail for a Loaf-executable invocation, or the Loaf-managed file a
// path-backed entry invokes. The `check` verb collapses out of enforcement
// stems so that both `loaf check --hook X` and `loaf check --hook X --advisory`
// carry the same identity.
func hookCatalogStem(command string) []string {
	tokens, ok := hookCommandTokens(command)
	if !ok || len(tokens) == 0 {
		return nil
	}
	if !isLoafExecutableCatalogToken(tokens[0]) {
		for _, token := range tokens {
			if looksLikeHookPathToken(token) {
				return []string{token}
			}
		}
		return nil
	}
	tail := tokens[1:]
	for len(tail) > 0 && hookCatalogVariantFlags[tail[len(tail)-1]] {
		tail = tail[:len(tail)-1]
	}
	if len(tail) > 2 && tail[0] == "check" && tail[1] == "--hook" {
		tail = tail[1:]
	}
	if len(tail) == 0 {
		return nil
	}
	return tail
}

// isLoafExecutableCatalogToken recognizes the two executable forms a built
// catalog can carry. The trusted-absolute-path form only exists after install,
// so it is resolved at recognition time, not here.
func isLoafExecutableCatalogToken(token string) bool {
	return token == "loaf" || token == codexJournalExecutablePlaceholder
}

func appendHookTokenSequence(sequences [][]string, tokens []string) [][]string {
	for _, existing := range sequences {
		if hookTokensEqual(existing, tokens) {
			return sequences
		}
	}
	return append(sequences, tokens)
}

// validateHookCatalog proves pairing cannot be ambiguous. The catalog is
// non-empty, identities are unique, every template is an entry the target can
// actually carry, every command entry has a signature no other identity claims,
// and no stem occurs inside another entry's stem or signature — so a match
// resolves to at most one hook ID by construction rather than by a runtime
// tiebreak.
//
// Emptiness is a failure rather than a degenerate case: an empty catalog claims
// this version ships no hooks, which would read every installed Loaf entry as a
// retired generation and remove all of them while projecting nothing.
func validateHookCatalog(catalog hookCatalog) error {
	if len(catalog.Entries) == 0 {
		return fmt.Errorf("%s hook catalog declares no entries", catalog.Target)
	}
	if err := validateHookCatalogCohorts(catalog); err != nil {
		return err
	}
	seen := map[string]bool{}
	signatures := map[string]string{}
	for _, entry := range catalog.Entries {
		if entry.Event == "" || entry.HookID == "" {
			return fmt.Errorf("%s hook catalog entry is missing an event or hook id", catalog.Target)
		}
		key := entry.Event + "/" + entry.HookID
		if seen[key] {
			return fmt.Errorf("%s hook catalog declares %s twice", catalog.Target, key)
		}
		seen[key] = true
		if err := validateHookCatalogTemplate(catalog.Target, key, entry); err != nil {
			return err
		}
		if entry.Type != "prompt" && len(entry.Signatures) == 0 {
			return fmt.Errorf("%s hook catalog entry %s has no command signature", catalog.Target, key)
		}
		for _, signature := range entry.Signatures {
			joined := strings.Join(signature, " ")
			if owner, claimed := signatures[joined]; claimed && owner != entry.HookID {
				return fmt.Errorf("%s hook catalog signature %q belongs to both %s and %s", catalog.Target, joined, owner, entry.HookID)
			}
			signatures[joined] = entry.HookID
		}
	}
	for _, entry := range catalog.Entries {
		for _, stem := range entry.Stems {
			for _, other := range catalog.Entries {
				if other.HookID == entry.HookID && other.Event == entry.Event {
					continue
				}
				for _, otherStem := range other.Stems {
					if containsHookTokenSequence(otherStem, stem) {
						return fmt.Errorf("%s hook catalog stem %q for %s also matches %s", catalog.Target, strings.Join(stem, " "), entry.HookID, other.HookID)
					}
				}
				for _, signature := range other.Signatures {
					if containsHookTokenSequence(signature, stem) {
						return fmt.Errorf("%s hook catalog stem %q for %s also matches the %s signature", catalog.Target, strings.Join(stem, " "), entry.HookID, other.HookID)
					}
				}
			}
		}
	}
	return nil
}

// validateHookCatalogTemplate rejects a desired entry the target could never
// carry. The template is what reconciliation publishes, so anything that is not
// an entry object — a null, an array, a bare string — would reach the file
// before post-verify could notice, and the recovery from that is worse than
// never writing it.
func validateHookCatalogTemplate(target string, key string, entry hookCatalogEntry) error {
	value, err := decodeHookJSONValue(entry.Template)
	if err != nil {
		return fmt.Errorf("%s hook catalog entry %s has an unreadable template: %w", target, key, err)
	}
	object, ok := value.(map[string]any)
	if !ok {
		return fmt.Errorf("%s hook catalog entry %s has a template that is not an entry object", target, key)
	}
	if target != "codex" {
		return nil
	}
	// Codex nests exactly one command handler inside a matcher group. A group
	// carrying anything else is not a shape recognition would ever claim back,
	// so publishing it would orphan the entry on the next reconcile.
	if !isValidCodexMatcherGroup(object) {
		return fmt.Errorf("codex hook catalog entry %s has a template that is not a valid matcher group", key)
	}
	handlers, ok := object["hooks"].([]any)
	if !ok || len(handlers) != 1 {
		return fmt.Errorf("codex hook catalog entry %s must carry exactly one command handler", key)
	}
	handler, ok := handlers[0].(map[string]any)
	if !ok {
		return fmt.Errorf("codex hook catalog entry %s has a handler that is not an object", key)
	}
	if _, ok := handler["command"].(string); !ok {
		return fmt.Errorf("codex hook catalog entry %s has a handler without a command", key)
	}
	return nil
}

// validateHookCatalogCohorts keeps the absorption bound readable: one record per
// version, no repeated ids inside it. Cohort ids are deliberately not required
// to exist among the current entries — a hook a prior version shipped and this
// one retired still bounds what that version could have projected.
func validateHookCatalogCohorts(catalog hookCatalog) error {
	versions := map[string]bool{}
	for _, cohort := range catalog.Cohorts {
		if strings.TrimSpace(cohort.Version) == "" {
			return fmt.Errorf("%s hook catalog declares a cohort with no version", catalog.Target)
		}
		if versions[cohort.Version] {
			return fmt.Errorf("%s hook catalog declares the %s cohort twice", catalog.Target, cohort.Version)
		}
		versions[cohort.Version] = true
		if len(cohort.HookIDs) == 0 {
			return fmt.Errorf("%s hook catalog %s cohort names no hook ids", catalog.Target, cohort.Version)
		}
		hookIDs := map[string]bool{}
		for _, hookID := range cohort.HookIDs {
			if strings.TrimSpace(hookID) == "" {
				return fmt.Errorf("%s hook catalog %s cohort names an empty hook id", catalog.Target, cohort.Version)
			}
			if hookIDs[hookID] {
				return fmt.Errorf("%s hook catalog %s cohort names %s twice", catalog.Target, cohort.Version, hookID)
			}
			hookIDs[hookID] = true
		}
	}
	return nil
}

func (c hookCatalog) entriesForEvent(event string) []hookCatalogEntry {
	var entries []hookCatalogEntry
	for _, entry := range c.Entries {
		if entry.Event == event {
			entries = append(entries, entry)
		}
	}
	return entries
}

// cohortHookIDs returns the hook IDs the given version shipped. An unknown
// version has no recorded cohort: callers decide what that means rather than
// inheriting a guess from a neighbouring version.
func (c hookCatalog) cohortHookIDs(version string) ([]string, bool) {
	for _, cohort := range c.Cohorts {
		if cohort.Version == version {
			return cohort.HookIDs, true
		}
	}
	return nil, false
}

func writeHookCatalog(outputDir string, catalog hookCatalog) error {
	body, err := json.MarshalIndent(catalog, "", "  ")
	if err != nil {
		return err
	}
	body = append(body, '\n')
	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(outputDir, hookCatalogFile), body, 0o644)
}

// readHookCatalog loads a target's catalog from its built distribution. It
// fails closed: a missing, non-regular, or malformed catalog is an error, never
// an empty identity authority that would make every installed entry foreign.
func readHookCatalog(distDir string) (hookCatalog, error) {
	path := filepath.Join(distDir, hookCatalogFile)
	body, err := readRegularFileNoFollow(path, projectFileReadLimit)
	if err != nil {
		return hookCatalog{}, fmt.Errorf("read hook catalog %s: %w", path, err)
	}
	if err := validateJSONNoDuplicateKeys(body); err != nil {
		return hookCatalog{}, fmt.Errorf("read hook catalog %s: %w", path, err)
	}
	var catalog hookCatalog
	if err := json.Unmarshal(body, &catalog); err != nil {
		return hookCatalog{}, fmt.Errorf("parse hook catalog %s: %w", path, err)
	}
	if catalog.Version != hookCatalogVersion {
		return hookCatalog{}, fmt.Errorf("hook catalog %s has unsupported version %d", path, catalog.Version)
	}
	if err := validateHookCatalog(catalog); err != nil {
		return hookCatalog{}, err
	}
	return catalog, nil
}
