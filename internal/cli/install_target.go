package cli

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync/atomic"
)

const (
	loafInstallMarkerFile = ".loaf-version"
	loafSkillManifestFile = ".loaf-managed-skills.json"
)

var legacyLoafHookSignatures = map[string]bool{
	"command:loaf check --hook check-" + "se" + "crets|matcher:Edit|Write|Bash|if:":           true,
	"command:loaf check --hook security-audit|matcher:Bash|if:":                               true,
	"command:loaf check --hook validate-push|matcher:Bash|if:":                                true,
	"command:loaf check --hook workflow-pre-pr|matcher:Bash|if:":                              true,
	"command:loaf check --hook validate-commit|matcher:Bash|if:":                              true,
	"command:loaf task refresh|matcher:Edit|Write|if:":                                        true,
	"command:bash $HOME/.cursor/hooks/post-tool/kb-staleness-nudge.sh|matcher:Edit|Write|if:": true,
	// Journal-first hook signatures.
	"command:loaf journal log --detect-linear|matcher:Bash|if:":                 true,
	"command:loaf journal log --from-hook|matcher:Bash|if:Bash(git commit:*)":   true,
	"command:loaf journal log --from-hook|matcher:Bash|if:Bash(gh pr create:*)": true,
	"command:loaf journal log --from-hook|matcher:Bash|if:Bash(gh pr merge:*)":  true,
	"command:loaf journal context|matcher:|if:":                                 true,
	// Legacy session-entity hook signatures — retained so `loaf install` cleans
	// them from existing installs during the journal-first migration.
	"command:loaf session log --detect-linear|matcher:Bash|if:":                 true,
	"command:loaf session log --from-hook|matcher:Bash|if:Bash(git commit:*)":   true,
	"command:loaf session log --from-hook|matcher:Bash|if:Bash(gh pr create:*)": true,
	"command:loaf session log --from-hook|matcher:Bash|if:Bash(gh pr merge:*)":  true,
	"command:loaf session start|matcher:|if:":                                   true,
	"command:loaf session end|matcher:|if:":                                     true,
	"command:bash $HOME/.cursor/hooks/session/compact.sh|matcher:|if:":          true,
}

var legacyLoafCommands = map[string]bool{
	"bash $HOME/.cursor/hooks/session/session-start-soul.sh":  true,
	"bash $HOME/.cursor/hooks/session/session-start.sh":       true,
	"bash $HOME/.cursor/hooks/session/kb-session-start.sh":    true,
	"bash $HOME/.cursor/hooks/session/session-end.sh":         true,
	"bash $HOME/.cursor/hooks/session/kb-session-end.sh":      true,
	"bash $HOME/.cursor/hooks/session/pre-compact-archive.sh": true,
}

var legacyLoafPromptPrefixes = []string{
	"STOP. Before running gh pr merge",
	"ADVISORY: You are about to run `git push`",
	"KNOWLEDGE BASE:",
	"POST-MERGE HOUSEKEEPING:",
	"CONTEXT COMPACTION IMMINENT:",
	"SESSION JOURNAL NUDGE:",
}

var obsoleteCursorHookFiles = []string{
	"session/check-sessions.sh",
	"session/kb-session-end.sh",
	"session/kb-session-start.sh",
	"session/pre-compact-archive.sh",
	"session/session-end-simple.sh",
	"session/session-end.sh",
	"session/session-start-soul.sh",
	"session/session-start.sh",
}

type targetInstallOptions struct {
	Target              string
	DistDir             string
	ConfigDir           string
	Upgrade             bool
	CodexBasicCommands  bool
	Version             string
	HomeDir             string
	CodexHome           string
	CodexRuleOperations *codexRuleInstallOperations
	ProjectRoot         string
	AmpSkillsDir        string
	AmpPluginsDir       string
	TargetAdapterOps    *targetAdapterInstallOperations
	// HookState resolves the user-scoped state hook reconciliation reads and
	// writes. Only Cursor and Codex reach it, and only when they actually
	// reconcile, so a target that keeps no shared hooks file never opens it.
	HookState hookStateResolver
	// HookActions receives the per-entry actions a reconcile took, so the
	// command that ran it can say what changed. Nil discards them.
	HookActions func([]hookAction)
	// HookOps injects failures at the reconciler's two ordering windows from
	// outside the target installer, which is the only vantage point that can
	// exercise the window after the adapter manifest is replaced and before
	// the file is projected. Nil in production.
	HookOps *hookReconcileOperations
	// SkipSkillsSync leaves the shared skills store alone. Multi-target
	// install/upgrade sets it after syncCanonicalManagedSkills has already
	// performed the one write per destination for the run.
	SkipSkillsSync bool
}

type installTargetRecord struct {
	Version   string `json:"version"`
	Target    string `json:"target"`
	ConfigDir string `json:"config_dir"`
	SkillsDir string `json:"skills_dir,omitempty"`
}

type managedSkillDigest struct {
	Name   string `json:"name"`
	SHA256 string `json:"sha256"`
}

type managedSkillsManifestV2 struct {
	Version int                  `json:"version"`
	Skills  []managedSkillDigest `json:"skills"`
}

type managedSkillsState struct {
	legacy  bool
	digests map[string]string
}

func installTargetDistribution(options targetInstallOptions) error {
	if options.Target == "" {
		return fmt.Errorf("install target is required")
	}
	if options.DistDir == "" {
		return fmt.Errorf("install dist dir is required")
	}
	if options.ConfigDir == "" {
		return fmt.Errorf("install config dir is required")
	}
	if options.Version == "" {
		options.Version = "0.0.0"
	}

	switch options.Target {
	case "opencode":
		return installOpencodeTarget(options)
	case "cursor":
		return installCursorTarget(options)
	case "codex":
		return installCodexTarget(options)
	case "amp":
		return installAmpTarget(options)
	default:
		return fmt.Errorf("no installer available for target %q", options.Target)
	}
}

func installOpencodeTarget(options targetInstallOptions) error {
	skillsDest := installSkillsDestination(options)
	if !options.SkipSkillsSync {
		if err := syncManagedSkillsDirIfExists(filepath.Join(options.DistDir, "skills"), skillsDest); err != nil {
			return err
		}
	}
	hasAdapterManifest := fileExistsForInstall(filepath.Join(options.DistDir, targetBuildManifestFile))
	if hasAdapterManifest {
		if err := syncTargetAdapterManifest(options); err != nil {
			return err
		}
	}
	dirs := []string{"agents", "templates"}
	if !hasAdapterManifest {
		dirs = append(dirs, "plugins")
	}
	for _, dir := range dirs {
		if err := syncTargetSubdir(options.DistDir, options.ConfigDir, dir); err != nil {
			return err
		}
	}
	if err := syncOpenCodeCommandsDir(options.DistDir, options.ConfigDir, skillsDest); err != nil {
		return err
	}
	if err := writeInstallMarker(options.ConfigDir, options.Version); err != nil {
		return err
	}
	return writeInstallRecord(options, skillsDest)
}

// syncOpenCodeCommandsDir installs OpenCode command files and rewrites
// skill-local template/reference links to paths that resolve from the
// installed commands directory to the canonical skills store.
//
// The distribution commands tree is fully validated before any destination
// mutation. A symlinked commands directory or any symlink inside it is a
// refusal — distinct from a legitimate symlinked config destination, which
// resolveInstallPath handles for link rewriting.
//
// Failure mode: if a user copies an installed command file to a different
// directory, Rel-based links break. That is acceptable; the install location
// is the contract. When Rel cannot be computed (different volumes), links fall
// back to an absolute path.
func syncOpenCodeCommandsDir(distDir string, configDir string, skillsStore string) error {
	src := filepath.Join(distDir, "commands")
	info, err := os.Lstat(src)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	if info.Mode()&fs.ModeSymlink != 0 {
		return fmt.Errorf("opencode commands source is a symlink")
	}
	if !info.IsDir() {
		return fmt.Errorf("opencode commands source is not a directory")
	}
	// Fail closed: command bodies link into the canonical skills store. A
	// partial distribution (commands without skills) must not stamp success
	// for links that cannot resolve.
	if !dirExistsForInstall(skillsStore) {
		return fmt.Errorf("cannot install OpenCode commands: canonical skills store %s does not exist", skillsStore)
	}
	// Validate the entire source tree before touching the destination. A
	// refusal here must leave existing installed commands intact.
	if err := validateOpenCodeCommandsSource(src); err != nil {
		return err
	}

	// Source fully accepted. First destination mutation:
	dest := filepath.Join(configDir, "commands")
	if err := os.MkdirAll(dest, 0o755); err != nil {
		return err
	}
	entries, err := os.ReadDir(dest)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if err := os.RemoveAll(filepath.Join(dest, entry.Name())); err != nil {
			return err
		}
	}
	srcEntries, err := os.ReadDir(src)
	if err != nil {
		return err
	}
	for _, entry := range srcEntries {
		name := entry.Name()
		from := filepath.Join(src, name)
		to := filepath.Join(dest, name)
		info, err := os.Lstat(from)
		if err != nil {
			return err
		}
		if info.Mode()&fs.ModeSymlink != 0 {
			return fmt.Errorf("opencode command %q is a symlink", name)
		}
		if info.IsDir() {
			if err := copyDirContentsForInstall(from, to); err != nil {
				return err
			}
			continue
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("opencode command %q is not a regular file", name)
		}
		body, err := os.ReadFile(from)
		if err != nil {
			return err
		}
		if strings.HasSuffix(name, ".md") {
			skill := strings.TrimSuffix(name, ".md")
			body = []byte(rewriteOpenCodeCommandSkillLinks(string(body), skill, dest, skillsStore))
		}
		if err := os.WriteFile(to, body, info.Mode().Perm()); err != nil {
			return err
		}
	}
	return nil
}

// validateOpenCodeCommandsSource walks the distribution commands tree with
// Lstat and refuses any symlink or non-regular leaf. The root must already be
// a real directory (caller Lstats it); this walk covers nested paths that a
// top-level entry check would miss.
func validateOpenCodeCommandsSource(src string) error {
	return filepath.WalkDir(src, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		info, err := os.Lstat(path)
		if err != nil {
			return err
		}
		if info.Mode()&fs.ModeSymlink != 0 {
			rel, relErr := filepath.Rel(src, path)
			if relErr != nil {
				rel = path
			}
			if rel == "." {
				return fmt.Errorf("opencode commands source is a symlink")
			}
			return fmt.Errorf("opencode command %q is a symlink", filepath.ToSlash(rel))
		}
		if info.IsDir() {
			return nil
		}
		if !info.Mode().IsRegular() {
			rel, relErr := filepath.Rel(src, path)
			if relErr != nil {
				rel = path
			}
			return fmt.Errorf("opencode command %q is not a regular file", filepath.ToSlash(rel))
		}
		return nil
	})
}

// rewriteOpenCodeCommandSkillLinks rewrites skill-local and legacy build-time
// template/reference markdown links so they resolve from commandsDir to
// skillsStore/<skill>. Paths are symlink-resolved first so Rel matches what
// the filesystem resolver sees when config directories are legitimate symlinks.
// Prefer filepath.Rel; fall back to absolute paths when Rel fails (cross-volume).
// Copied-elsewhere breakage is an accepted failure mode.
func rewriteOpenCodeCommandSkillLinks(content string, skill string, commandsDir string, skillsStore string) string {
	commandsResolved := resolveInstallPath(commandsDir)
	storeResolved := resolveInstallPath(skillsStore)
	skillRoot := filepath.Join(storeResolved, skill)
	rel, err := filepath.Rel(commandsResolved, skillRoot)
	linkBase := filepath.ToSlash(skillRoot)
	if err == nil {
		linkBase = filepath.ToSlash(rel)
	}
	legacyTemplates := "](../skills/" + skill + "/templates/"
	legacyReferences := "](../skills/" + skill + "/references/"
	content = strings.ReplaceAll(content, legacyTemplates, "]("+linkBase+"/templates/")
	content = strings.ReplaceAll(content, legacyReferences, "]("+linkBase+"/references/")
	content = strings.ReplaceAll(content, "](templates/", "]("+linkBase+"/templates/")
	content = strings.ReplaceAll(content, "](references/", "]("+linkBase+"/references/")
	return content
}

func installCursorTarget(options targetInstallOptions) error {
	// Building the reconciler is what proves the distribution is current, so it
	// happens before anything on this machine changes. A stale build output
	// that was refused only after the commands directory had been removed would
	// have already taken something away from the operator.
	reconciler, err := newHookReconciler(options)
	if err != nil {
		return err
	}
	skillsDest := installSkillsDestination(options)
	if !options.SkipSkillsSync {
		if err := syncManagedSkillsDirIfExists(filepath.Join(options.DistDir, "skills"), skillsDest); err != nil {
			return err
		}
	}
	if err := os.RemoveAll(filepath.Join(options.ConfigDir, "commands")); err != nil {
		return err
	}
	if err := syncTargetDirIfExists(filepath.Join(options.DistDir, "agents"), filepath.Join(options.ConfigDir, "agents")); err != nil {
		return err
	}
	if err := beginHookReconcile(reconciler); err != nil {
		return err
	}
	defer releaseHookReconcile(reconciler)
	if err := syncTargetAdapterManifest(options); err != nil {
		return err
	}
	if options.Upgrade {
		for _, file := range obsoleteCursorHookFiles {
			if err := os.Remove(filepath.Join(options.ConfigDir, "hooks", filepath.FromSlash(file))); err != nil && !os.IsNotExist(err) {
				return err
			}
		}
	}
	if err := syncTargetDirIfExists(filepath.Join(options.DistDir, "templates"), filepath.Join(options.ConfigDir, "templates")); err != nil {
		return err
	}
	if err := completeHookReconcile(options, reconciler); err != nil {
		return err
	}
	if err := writeInstallMarker(options.ConfigDir, options.Version); err != nil {
		return err
	}
	return writeInstallRecord(options, skillsDest)
}

func installCodexTarget(options targetInstallOptions) error {
	// Same ordering as Cursor: prove the distribution is current before the
	// shared skills store — the first surface this touches — is written.
	reconciler, err := newHookReconciler(options)
	if err != nil {
		return err
	}
	homeDir := installHomeDir(options)
	codexHome := options.CodexHome
	if codexHome == "" {
		codexHome = filepath.Join(homeDir, ".codex")
	}
	skillsDest := installSkillsDestination(options)
	if !options.SkipSkillsSync {
		if err := syncManagedSkillsDirIfExists(filepath.Join(options.DistDir, "skills"), skillsDest); err != nil {
			return err
		}
	}
	if err := beginHookReconcile(reconciler); err != nil {
		return err
	}
	defer releaseHookReconcile(reconciler)
	if err := syncTargetAdapterManifest(options); err != nil {
		return err
	}
	if err := installCodexJournalRuleWithOperations(options, codexHome, options.CodexRuleOperations); err != nil {
		return err
	}
	if err := completeHookReconcile(options, reconciler); err != nil {
		return err
	}
	if err := writeInstallMarker(options.ConfigDir, options.Version); err != nil {
		return err
	}
	return writeInstallRecord(options, skillsDest)
}

func installAmpTarget(options targetInstallOptions) error {
	skillsDest := installSkillsDestination(options)
	if !options.SkipSkillsSync {
		if err := syncManagedSkillsDirIfExists(filepath.Join(options.DistDir, "skills"), skillsDest); err != nil {
			return err
		}
	}
	if fileExistsForInstall(filepath.Join(options.DistDir, targetBuildManifestFile)) {
		if err := syncTargetAdapterManifest(options); err != nil {
			return err
		}
	} else {
		pluginSrc := filepath.Join(options.DistDir, ".amp", "plugins", "loaf.ts")
		if fileExistsForInstall(pluginSrc) {
			pluginsDest := options.AmpPluginsDir
			if pluginsDest == "" {
				pluginsDest = filepath.Join(options.ConfigDir, "plugins")
			}
			if err := os.MkdirAll(pluginsDest, 0o755); err != nil {
				return err
			}
			if err := copyFileForInstall(pluginSrc, filepath.Join(pluginsDest, "loaf.ts")); err != nil {
				return err
			}
		}
	}
	if err := writeInstallMarker(options.ConfigDir, options.Version); err != nil {
		return err
	}
	return writeInstallRecord(options, skillsDest)
}

func installHomeDir(options targetInstallOptions) string {
	if options.HomeDir != "" {
		return options.HomeDir
	}
	if home := os.Getenv("HOME"); home != "" {
		return home
	}
	return os.Getenv("USERPROFILE")
}

func installSkillsDestination(options targetInstallOptions) string {
	// AmpSkillsDir is a test-only override (see TestInstallTargetAmpUsesSharedAndCustomHomes).
	// Production install/upgrade never set it; collapse still groups by the
	// resolved destination so a genuine per-target override would stay correct.
	if options.Target == "amp" && options.AmpSkillsDir != "" {
		return options.AmpSkillsDir
	}
	return filepath.Join(installHomeDir(options), ".agents", "skills")
}

// skillsDestinationGroup is one resolved skills destination and the selected
// targets that would write into it.
type skillsDestinationGroup struct {
	Destination string
	Options     []targetInstallOptions
}

// groupSkillsInstallByDestination collapses selected targets onto the distinct
// destinations their skills would land in. Targets without a skills tree are
// omitted — they contribute no write.
//
// Install assumes dist/ is stable for the duration of the run: source trees are
// verified once before planning/sync, then read again to write. Concurrent
// mutation of dist/ between those steps is not detected. That TOCTOU property
// predates the single-canonical-write collapse — each per-target installer
// already read its DistDir without a re-hash lock.
func groupSkillsInstallByDestination(options []targetInstallOptions) ([]skillsDestinationGroup, error) {
	byDest := map[string][]targetInstallOptions{}
	for _, opt := range options {
		src := filepath.Join(opt.DistDir, "skills")
		if !dirExistsForInstall(src) {
			continue
		}
		dest := installSkillsDestination(opt)
		byDest[dest] = append(byDest[dest], opt)
	}
	dests := make([]string, 0, len(byDest))
	for dest := range byDest {
		dests = append(dests, dest)
	}
	sort.Strings(dests)
	groups := make([]skillsDestinationGroup, 0, len(dests))
	for _, dest := range dests {
		opts := byDest[dest]
		sort.Slice(opts, func(i, j int) bool { return opts[i].Target < opts[j].Target })
		if err := verifyIdenticalSkillsSources(opts); err != nil {
			return nil, err
		}
		groups = append(groups, skillsDestinationGroup{Destination: dest, Options: opts})
	}
	return groups, nil
}

// selectCanonicalSkillsSource picks which selected target's skills tree is
// copied into the shared store. A single store copy can carry only one
// frontmatter shape, so the source is the unique store-sharing target that
// declares owned sidecar keys in nativeBuildSidecarOwnedFrontmatterKeysByTarget.
// Claude Code is never among options here (plugin channel). If none declare
// owned keys, any tree is equivalent and the alphabetical first wins. If more
// than one declares owned keys, fail — that is a design conflict a future
// sidecar could introduce and must not resolve silently.
func selectCanonicalSkillsSource(options []targetInstallOptions) (targetInstallOptions, error) {
	return selectCanonicalSkillsSourceWithOwnedKeys(options, nativeBuildSidecarOwnedFrontmatterKeysByTarget)
}

func selectCanonicalSkillsSourceWithOwnedKeys(options []targetInstallOptions, ownedByTarget map[string]map[string]bool) (targetInstallOptions, error) {
	if len(options) == 0 {
		return targetInstallOptions{}, fmt.Errorf("no skills sources to select")
	}
	var owners []targetInstallOptions
	for _, opt := range options {
		if len(ownedByTarget[opt.Target]) > 0 {
			owners = append(owners, opt)
		}
	}
	if len(owners) > 1 {
		parts := make([]string, 0, len(owners))
		for _, opt := range owners {
			keys := make([]string, 0, len(ownedByTarget[opt.Target]))
			for key := range ownedByTarget[opt.Target] {
				keys = append(keys, key)
			}
			sort.Strings(keys)
			parts = append(parts, fmt.Sprintf("%s (%s)", opt.Target, strings.Join(keys, ", ")))
		}
		return targetInstallOptions{}, fmt.Errorf("multiple store-sharing targets declare owned sidecar frontmatter keys; a single canonical skills copy cannot satisfy all of them: %s", strings.Join(parts, "; "))
	}
	if len(owners) == 1 {
		return owners[0], nil
	}
	return options[0], nil
}

// verifyIdenticalSkillsSources requires every selected target's skills tree to
// match (after stripping known sidecar-owned frontmatter keys and the stamped
// version) before a shared destination accepts a single write.
//
// Guarantee: detects accidental body divergence between built target trees in
// dist/. Non-guarantee: does not and cannot detect tampering of owned-key
// values or of dist/ itself — dist/ is trusted input (as it was when each
// per-target installer copied dist/<target>/skills with no check). Source
// sidecars (content/skills/*/SKILL.<target>.yaml) are not shipped in the
// distribution, so value-level authorization is impossible at install time.
// Silent first-wins would install one target's tree while claiming to serve
// the rest.
//
// Comparison is scoped to the skill directories listInstallSkillDirs returns —
// the same set syncManagedSkillsDirIfExists installs — so stray regular files
// under the skills root are neither hashed nor admitted.
func verifyIdenticalSkillsSources(options []targetInstallOptions) error {
	if len(options) == 0 {
		return nil
	}
	// Single-target installs still validate the source path (dist → DistDir →
	// skills) even though there is nothing to compare against. Skipping here
	// left a symlinked DistDir uncaught when only one store-sharing target was
	// selected.
	if len(options) == 1 {
		_, err := listInstallSkillDirs(filepath.Join(options[0].DistDir, "skills"))
		return err
	}
	if _, err := selectCanonicalSkillsSource(options); err != nil {
		return err
	}
	baseline := options[0]
	baselineFiles, baselineVersion, err := loadNormalizedInstallSkillFiles(baseline)
	if err != nil {
		return fmt.Errorf("normalize skills tree for target %q: %w", baseline.Target, err)
	}
	for _, opt := range options[1:] {
		files, version, err := loadNormalizedInstallSkillFiles(opt)
		if err != nil {
			return fmt.Errorf("normalize skills tree for target %q: %w", opt.Target, err)
		}
		if version != baselineVersion {
			return fmt.Errorf("skills trees diverge between targets %q and %q; stamped versions %q vs %q; refusing shared install into %s", baseline.Target, opt.Target, baselineVersion, version, installSkillsDestination(opt))
		}
		for rel, body := range files {
			want, ok := baselineFiles[rel]
			if !ok {
				return fmt.Errorf("skills trees diverge between targets %q and %q; refusing shared install into %s", baseline.Target, opt.Target, installSkillsDestination(opt))
			}
			if body != want {
				return fmt.Errorf("skills trees diverge between targets %q and %q; refusing shared install into %s", baseline.Target, opt.Target, installSkillsDestination(opt))
			}
		}
		for rel := range baselineFiles {
			if _, ok := files[rel]; !ok {
				return fmt.Errorf("skills trees diverge between targets %q and %q; refusing shared install into %s", baseline.Target, opt.Target, installSkillsDestination(opt))
			}
		}
	}
	return nil
}

// loadNormalizedInstallSkillFiles returns the comparable form of every regular
// file under each skill directory listInstallSkillDirs admits. SKILL.md strips
// known sidecar-owned keys by membership only (see
// normalizeInstallSkillFileForDivergence); other files compare as raw bytes.
//
// The skills source path and walk use the same symlink/non-directory guards as
// sync (via requireInstallSkillsSourcePath → requireInstallSkillTreeRoot in
// listInstallSkillDirs, and Lstat rejection in the walk). Per-file reads are
// bounded by projectFileReadLimit.
func loadNormalizedInstallSkillFiles(opt targetInstallOptions) (map[string]string, string, error) {
	src := filepath.Join(opt.DistDir, "skills")
	skills, err := listInstallSkillDirs(src)
	if err != nil {
		return nil, "", err
	}
	out := map[string]string{}
	var stampedVersion string
	for _, skill := range skills {
		skillDir := filepath.Join(src, skill)
		if _, err := requireInstallSkillTreeRoot(skillDir); err != nil {
			return nil, "", err
		}
		err := filepath.WalkDir(skillDir, func(path string, entry fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			info, err := os.Lstat(path)
			if err != nil {
				return err
			}
			if info.Mode()&fs.ModeSymlink != 0 {
				return fmt.Errorf("contains symlink %q", path)
			}
			if info.IsDir() {
				return nil
			}
			if !info.Mode().IsRegular() {
				return fmt.Errorf("contains non-regular file %q", path)
			}
			rel, err := filepath.Rel(src, path)
			if err != nil {
				return err
			}
			rel = filepath.ToSlash(rel)
			body, err := readRegularFileNoFollow(path, projectFileReadLimit)
			if err != nil {
				return err
			}
			if filepath.Base(rel) != "SKILL.md" {
				out[rel] = string(body)
				return nil
			}
			normalized, version, err := normalizeInstallSkillFileForDivergence(rel, string(body))
			if err != nil {
				return err
			}
			if stampedVersion == "" {
				stampedVersion = version
			} else if version != stampedVersion {
				return fmt.Errorf("inconsistent stamped versions %q and %q", stampedVersion, version)
			}
			out[rel] = normalized
			return nil
		})
		if err != nil {
			return nil, "", err
		}
	}
	return out, stampedVersion, nil
}

// normalizeInstallSkillFileForDivergence returns the comparable form of a skill
// file for cross-target divergence detection at install time.
//
// Build-time invariance (normalizeNativeBuildSkillFileForInvariance) authorizes
// owned-key values against content/skills sidecars. Install cannot: those
// sidecars are not present in dist/. Owned keys in
// nativeBuildAnySidecarOwnedFrontmatterKey are therefore stripped by key
// membership only — value-independently — along with the stamped version.
// Detects accidental body divergence between target trees; does not detect
// tampering of owned-key values in dist/.
func normalizeInstallSkillFileForDivergence(rel string, body string) (normalized string, version string, err error) {
	if filepath.Base(rel) != "SKILL.md" {
		return body, "", nil
	}
	frontmatter, content := splitNativeBuildFrontmatter(body)
	if frontmatter == "" {
		return body, "", nil
	}
	kept, version, err := stripOwnedFrontmatterKeysByMembership(frontmatter, nativeBuildAnySidecarOwnedFrontmatterKey)
	if err != nil {
		return "", "", fmt.Errorf("%s: %w", rel, err)
	}
	if strings.TrimSpace(kept) == "" {
		return content, version, nil
	}
	return "---\n" + kept + "---\n" + content, version, nil
}

// stripOwnedFrontmatterKeysByMembership walks raw frontmatter lines, removing
// top-level keys present in ownedKeys (and the stamped version) without
// comparing values. Nested YAML under kept keys is preserved.
func stripOwnedFrontmatterKeysByMembership(frontmatter string, ownedKeys map[string]bool) (kept string, version string, err error) {
	lines := strings.Split(frontmatter, "\n")
	var out []string
	for i := 0; i < len(lines); {
		line := lines[i]
		if line == "" || strings.HasPrefix(strings.TrimSpace(line), "#") || strings.HasPrefix(line, " ") || strings.HasPrefix(line, "\t") || !strings.Contains(line, ":") {
			out = append(out, line)
			i++
			continue
		}
		key, rawValue, _ := strings.Cut(strings.TrimSpace(line), ":")
		key = strings.TrimSpace(key)
		block, next := collectNativeBuildFrontmatterKeyBlock(lines, i)
		i = next

		if key == "version" {
			version = unquoteNativeBuildYAML(strings.TrimSpace(rawValue))
			if version == ">-" || version == ">" || version == "|" || version == "|-" {
				version = strings.TrimSpace(strings.Join(block[1:], "\n"))
			}
			continue
		}
		if ownedKeys[key] {
			continue
		}
		out = append(out, block...)
	}
	return strings.Join(out, "\n"), version, nil
}

// skillSyncConflict names one skill the apply path refused to mutate.
type skillSyncConflict struct {
	Skill  string
	Reason string
}

// skillSyncConflictsError is returned after a partial managed-skills sync when
// one or more skills were skipped for ownership conflicts. Non-conflicted
// skills were installed; callers must still run target adapters and then
// propagate this error so scripts observe non-zero.
type skillSyncConflictsError struct {
	Conflicts []skillSyncConflict
}

func (e *skillSyncConflictsError) Error() string {
	if e == nil || len(e.Conflicts) == 0 {
		return "managed skills sync conflicts"
	}
	parts := make([]string, 0, len(e.Conflicts))
	for _, c := range e.Conflicts {
		parts = append(parts, fmt.Sprintf("%s: %s", c.Skill, c.Reason))
	}
	return "managed skills sync conflicts: " + strings.Join(parts, "; ")
}

// syncCanonicalManagedSkills performs exactly one syncManagedSkillsDirIfExists
// per resolved destination across the selected targets. The source tree is the
// canonical store-sharing target (see selectCanonicalSkillsSource), not the
// alphabetical first option.
func syncCanonicalManagedSkills(options []targetInstallOptions) error {
	groups, err := groupSkillsInstallByDestination(options)
	if err != nil {
		return err
	}
	var conflicts *skillSyncConflictsError
	for _, group := range groups {
		source, err := selectCanonicalSkillsSource(group.Options)
		if err != nil {
			return err
		}
		src := filepath.Join(source.DistDir, "skills")
		if err := syncManagedSkillsDirIfExists(src, group.Destination); err != nil {
			var groupConflicts *skillSyncConflictsError
			if errors.As(err, &groupConflicts) {
				if conflicts == nil {
					conflicts = &skillSyncConflictsError{}
				}
				conflicts.Conflicts = append(conflicts.Conflicts, groupConflicts.Conflicts...)
				continue
			}
			return err
		}
	}
	if conflicts != nil && len(conflicts.Conflicts) > 0 {
		return conflicts
	}
	return nil
}

// managedSkillsSyncCalls counts syncManagedSkillsDirIfExists invocations that
// pass the source-exists gate. Tests reset and read it to prove multi-target
// installs collapse to one write per destination.
var managedSkillsSyncCalls atomic.Int64

func syncTargetSubdir(distDir string, configDir string, name string) error {
	return syncTargetDirIfExists(filepath.Join(distDir, name), filepath.Join(configDir, name))
}

func syncTargetDirIfExists(src string, dest string) error {
	if !dirExistsForInstall(src) {
		return nil
	}
	if err := os.MkdirAll(dest, 0o755); err != nil {
		return err
	}
	entries, err := os.ReadDir(dest)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if err := os.RemoveAll(filepath.Join(dest, entry.Name())); err != nil {
			return err
		}
	}
	return copyDirContentsForInstall(src, dest)
}

func syncManagedSkillsDirIfExists(src string, dest string) (returnErr error) {
	if !dirExistsForInstall(src) {
		return nil
	}
	managedSkillsSyncCalls.Add(1)
	if err := os.MkdirAll(dest, 0o755); err != nil {
		return err
	}
	sourceSkills, err := listInstallSkillDirs(src)
	if err != nil {
		return err
	}
	previous, err := readManagedSkillsState(dest)
	if err != nil {
		return err
	}
	current := map[string]string{}
	for _, skill := range sourceSkills {
		digest, err := hashInstallSkillTree(filepath.Join(src, skill))
		if err != nil {
			return fmt.Errorf("hash source skill %q: %w", skill, err)
		}
		current[skill] = digest
	}

	// Collect ownership conflicts for the whole source set before mutating.
	// Blast radius is per-skill: non-conflicted skills still install.
	conflicts := map[string]string{}
	for skill, recordedDigest := range previous.digests {
		path := filepath.Join(dest, skill)
		if previous.legacy {
			continue
		}
		actual, err := hashInstallSkillTree(path)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return fmt.Errorf("managed skill %q cannot be verified: %w", skill, err)
		}
		if actual != recordedDigest && actual != current[skill] {
			conflicts[skill] = fmt.Sprintf("managed skill %q was modified; refusing to overwrite or remove", skill)
		}
	}
	for _, skill := range sourceSkills {
		if _, owned := previous.digests[skill]; owned {
			continue
		}
		if _, err := os.Lstat(filepath.Join(dest, skill)); err == nil {
			conflicts[skill] = fmt.Sprintf("skill destination %q already exists and is not managed by Loaf", skill)
		} else if !os.IsNotExist(err) {
			return err
		}
	}

	stageRoot, err := os.MkdirTemp(filepath.Dir(dest), "."+filepath.Base(dest)+".loaf-skills-stage-")
	if err != nil {
		return err
	}
	retainStageRoot := false
	defer func() {
		if !retainStageRoot {
			if cleanupErr := os.RemoveAll(stageRoot); cleanupErr != nil && returnErr == nil {
				returnErr = cleanupErr
			}
		}
	}()
	for _, skill := range sourceSkills {
		if _, bad := conflicts[skill]; bad {
			continue
		}
		staged := filepath.Join(stageRoot, "desired", skill)
		if err := copyInstallSkillTree(filepath.Join(src, skill), staged); err != nil {
			return fmt.Errorf("stage skill %q: %w", skill, err)
		}
		stagedDigest, err := hashInstallSkillTree(staged)
		if err != nil || stagedDigest != current[skill] {
			if err != nil {
				return fmt.Errorf("verify staged skill %q: %w", skill, err)
			}
			return fmt.Errorf("verify staged skill %q: source changed during install", skill)
		}
	}
	for skill := range previous.digests {
		if _, keep := current[skill]; keep {
			continue
		}
		if _, bad := conflicts[skill]; bad {
			// Ownership refusal: never delete a directory Loaf cannot prove it owns.
			continue
		}
		retain, err := retireManagedSkill(filepath.Join(dest, skill), filepath.Join(stageRoot, "backups", skill), previous.digests[skill], previous.legacy)
		if retain {
			retainStageRoot = true
		}
		if err != nil {
			return err
		}
	}
	for _, skill := range sourceSkills {
		if _, bad := conflicts[skill]; bad {
			continue
		}
		installed := filepath.Join(dest, skill)
		if actual, err := hashInstallSkillTree(installed); err == nil && actual == current[skill] {
			continue
		} else if err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("verify managed skill %q before publish: %w", skill, err)
		}
		_, owned := previous.digests[skill]
		retain, err := publishStagedSkill(filepath.Join(stageRoot, "desired", skill), installed, filepath.Join(stageRoot, "backups", skill), previous.digests[skill], current[skill], previous.legacy, owned)
		if retain {
			retainStageRoot = true
		}
		if err != nil {
			return err
		}
	}

	// Manifest retention for conflicted skills:
	// - Modified-managed: keep the previous digest entry even though the skill
	//   was not updated. Dropping it would make the next run treat the
	//   destination as never-managed and silently change the ownership story.
	//   Retaining keeps Loaf's claim visible; the conflict report makes the
	//   "claims but does not control" state explicit.
	// - Unowned collision (never in manifest): do not add a new entry.
	manifestSkills := map[string]string{}
	for _, skill := range sourceSkills {
		if _, bad := conflicts[skill]; bad {
			if _, owned := previous.digests[skill]; owned {
				manifestSkills[skill] = previous.digests[skill]
			}
			continue
		}
		manifestSkills[skill] = current[skill]
	}
	for skill, digest := range previous.digests {
		if _, keep := current[skill]; keep {
			continue
		}
		if _, bad := conflicts[skill]; bad {
			manifestSkills[skill] = digest
		}
	}
	names := make([]string, 0, len(manifestSkills))
	for name := range manifestSkills {
		names = append(names, name)
	}
	sort.Strings(names)
	manifest := managedSkillsManifestV2{Version: 2, Skills: make([]managedSkillDigest, 0, len(names))}
	for _, name := range names {
		manifest.Skills = append(manifest.Skills, managedSkillDigest{Name: name, SHA256: manifestSkills[name]})
	}
	if err := writeManagedSkillsManifest(dest, manifest); err != nil {
		return err
	}
	if len(conflicts) == 0 {
		return nil
	}
	names = make([]string, 0, len(conflicts))
	for skill := range conflicts {
		names = append(names, skill)
	}
	sort.Strings(names)
	out := &skillSyncConflictsError{Conflicts: make([]skillSyncConflict, 0, len(names))}
	for _, skill := range names {
		out.Conflicts = append(out.Conflicts, skillSyncConflict{Skill: skill, Reason: conflicts[skill]})
	}
	return out
}

func listInstallSkillDirs(path string) ([]string, error) {
	if err := requireInstallSkillsSourcePath(path); err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(path)
	if err != nil {
		return nil, err
	}
	var skills []string
	for _, entry := range entries {
		info, err := os.Lstat(filepath.Join(path, entry.Name()))
		if err != nil {
			return nil, err
		}
		if info.Mode()&fs.ModeSymlink != 0 {
			return nil, fmt.Errorf("skill source %q contains a symlink", entry.Name())
		}
		if info.IsDir() {
			if !validSourceSkillName(entry.Name()) {
				return nil, fmt.Errorf("invalid skill source name %q", entry.Name())
			}
			skills = append(skills, entry.Name())
		}
	}
	sort.Strings(skills)
	return skills, nil
}

func readManagedSkillsState(dest string) (managedSkillsState, error) {
	path := filepath.Join(dest, loafSkillManifestFile)
	if info, err := os.Lstat(path); err == nil {
		if !info.Mode().IsRegular() || info.Mode()&fs.ModeSymlink != 0 {
			return managedSkillsState{}, fmt.Errorf("managed skills manifest %s must be a regular file", path)
		}
	} else if !os.IsNotExist(err) {
		return managedSkillsState{}, err
	}
	body, err := readRegularFile(path, projectFileReadLimit)
	if err != nil {
		if os.IsNotExist(err) {
			return managedSkillsState{legacy: true, digests: map[string]string{}}, nil
		}
		return managedSkillsState{}, err
	}
	if err := validateJSONNoDuplicateKeys(body); err != nil {
		return managedSkillsState{}, fmt.Errorf("read managed skills manifest: %w", err)
	}
	var raw map[string]json.RawMessage
	decoder := json.NewDecoder(bytes.NewReader(body))
	if err := decoder.Decode(&raw); err != nil {
		return managedSkillsState{}, fmt.Errorf("read managed skills manifest: %w", err)
	}
	if raw == nil {
		return managedSkillsState{}, fmt.Errorf("read managed skills manifest: top-level value must be an object")
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return managedSkillsState{}, fmt.Errorf("read managed skills manifest: trailing JSON values")
	}
	if len(raw) != 2 || raw["version"] == nil || raw["skills"] == nil {
		return managedSkillsState{}, fmt.Errorf("read managed skills manifest: expected only version and skills")
	}
	var version int
	if err := json.Unmarshal(raw["version"], &version); err != nil {
		return managedSkillsState{}, fmt.Errorf("read managed skills manifest: version must be an integer")
	}
	if version == 1 {
		var skills []string
		if bytes.Equal(bytes.TrimSpace(raw["skills"]), []byte("null")) || json.Unmarshal(raw["skills"], &skills) != nil {
			return managedSkillsState{}, fmt.Errorf("read managed skills manifest: v1 skills must be an array of names")
		}
		if err := validateManagedSkillNames(skills); err != nil {
			return managedSkillsState{}, err
		}
		digests := make(map[string]string, len(skills))
		for _, skill := range skills {
			digests[skill] = ""
		}
		return managedSkillsState{legacy: true, digests: digests}, nil
	}
	if version != 2 {
		return managedSkillsState{}, fmt.Errorf("read managed skills manifest: unsupported version %d", version)
	}
	if bytes.Equal(bytes.TrimSpace(raw["skills"]), []byte("null")) {
		return managedSkillsState{}, fmt.Errorf("read managed skills manifest: v2 skills must be an array")
	}
	var entries []json.RawMessage
	if err := json.Unmarshal(raw["skills"], &entries); err != nil || entries == nil {
		return managedSkillsState{}, fmt.Errorf("read managed skills manifest: v2 skills must be an array")
	}
	digests := make(map[string]string, len(entries))
	last := ""
	for _, entry := range entries {
		var fields map[string]json.RawMessage
		if err := json.Unmarshal(entry, &fields); err != nil || len(fields) != 2 || fields["name"] == nil || fields["sha256"] == nil {
			return managedSkillsState{}, fmt.Errorf("read managed skills manifest: invalid v2 skill entry")
		}
		var skill managedSkillDigest
		if err := json.Unmarshal(fields["name"], &skill.Name); err != nil || json.Unmarshal(fields["sha256"], &skill.SHA256) != nil {
			return managedSkillsState{}, fmt.Errorf("read managed skills manifest: invalid v2 skill entry")
		}
		if !validManagedSkillName(skill.Name) || skill.Name <= last || len(skill.SHA256) != 64 || strings.ToLower(skill.SHA256) != skill.SHA256 || !isHexString(skill.SHA256) {
			return managedSkillsState{}, fmt.Errorf("read managed skills manifest: invalid v2 skill entry %q", skill.Name)
		}
		last = skill.Name
		digests[skill.Name] = skill.SHA256
	}
	return managedSkillsState{digests: digests}, nil
}

func validateJSONNoDuplicateKeys(body []byte) error {
	decoder := json.NewDecoder(bytes.NewReader(body))
	token, err := decoder.Token()
	if err != nil {
		return err
	}
	if err := walkJSONValue(decoder, token); err != nil {
		return err
	}
	if _, err := decoder.Token(); err != io.EOF {
		return fmt.Errorf("trailing JSON values")
	}
	return nil
}

func walkJSONValue(decoder *json.Decoder, token json.Token) error {
	delimiter, ok := token.(json.Delim)
	if !ok {
		return nil
	}
	switch delimiter {
	case '{':
		seen := map[string]bool{}
		for decoder.More() {
			key, err := decoder.Token()
			if err != nil {
				return err
			}
			name, ok := key.(string)
			if !ok {
				return fmt.Errorf("object key is not a string")
			}
			if seen[name] {
				return fmt.Errorf("duplicate object key %q", name)
			}
			seen[name] = true
			value, err := decoder.Token()
			if err != nil {
				return err
			}
			if err := walkJSONValue(decoder, value); err != nil {
				return err
			}
		}
		_, err := decoder.Token()
		return err
	case '[':
		for decoder.More() {
			value, err := decoder.Token()
			if err != nil {
				return err
			}
			if err := walkJSONValue(decoder, value); err != nil {
				return err
			}
		}
		_, err := decoder.Token()
		return err
	default:
		return fmt.Errorf("unexpected delimiter %q", delimiter)
	}
}

func writeManagedSkillsManifest(dest string, manifest managedSkillsManifestV2) error {
	body, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return err
	}
	body = append(body, '\n')
	path := filepath.Join(dest, loafSkillManifestFile)
	if info, err := os.Lstat(path); err == nil {
		if !info.Mode().IsRegular() || info.Mode()&fs.ModeSymlink != 0 {
			return fmt.Errorf("managed skills manifest %s must be a regular file", path)
		}
	} else if !os.IsNotExist(err) {
		return err
	}
	return writeFileAtomically(path, body, 0o644)
}

func validManagedSkillName(name string) bool {
	if name == "" || filepath.Base(name) != name || name == "." || name == ".." || strings.ContainsAny(name, `/\\`) {
		return false
	}
	for _, char := range name {
		if !(char >= 'a' && char <= 'z' || char >= 'A' && char <= 'Z' || char >= '0' && char <= '9' || char == '.' || char == '_' || char == '-') {
			return false
		}
	}
	return true
}

func validSourceSkillName(name string) bool {
	if len(name) == 0 || len(name) > 64 || name == "anthropic" || name == "claude" {
		return false
	}
	for _, char := range name {
		if !(char >= 'a' && char <= 'z' || char >= '0' && char <= '9' || char == '-') {
			return false
		}
	}
	return true
}

func validateManagedSkillNames(names []string) error {
	seen := map[string]bool{}
	for _, name := range names {
		if !validManagedSkillName(name) || seen[name] {
			return fmt.Errorf("read managed skills manifest: invalid or duplicate v1 skill name %q", name)
		}
		seen[name] = true
	}
	return nil
}

func isHexString(value string) bool {
	for _, char := range value {
		if !(char >= '0' && char <= '9' || char >= 'a' && char <= 'f') {
			return false
		}
	}
	return true
}

// requireInstallSkillTreeRoot Lstats path and refuses a symlink or non-directory.
// Shared by hashInstallSkillTree, requireInstallSkillsSourcePath, and the
// install divergence verifier so the guards cannot drift apart.
func requireInstallSkillTreeRoot(root string) (os.FileInfo, error) {
	info, err := os.Lstat(root)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() || info.Mode()&fs.ModeSymlink != 0 {
		return nil, fmt.Errorf("not a directory or is a symlink")
	}
	return info, nil
}

// requireInstallSkillsSourcePath Lstats every directory from the distribution
// root through DistDir and DistDir/skills. skillsSrc is Join(DistDir, "skills");
// DistDir is derived as its parent, and the distribution root as DistDir's parent
// (typically <prefix>/dist). The leaf must be named "skills" and both parents
// must be distinct establishable paths — if the boundary cannot be established,
// refuse rather than silently checking fewer components.
//
// A symlink at dist/ or at DistDir is the exfiltration class this closes:
// leaf-only Lstat of .../skills misses it because os.Stat/ReadDir follow
// ancestors. Ancestors *above* the distribution root are not inspected, so a
// checkout or install prefix reached through a legitimate symlink (macOS
// /tmp → /private/tmp) remains valid. EvalSymlinks containment is deliberately
// not used: resolving a component that *is* the symlink would make the external
// tree look "inside" the resolved root and false-pass.
func requireInstallSkillsSourcePath(skillsSrc string) error {
	skillsSrc = filepath.Clean(skillsSrc)
	if skillsSrc == "" || skillsSrc == "." {
		return fmt.Errorf("skills source path is required")
	}
	if filepath.Base(skillsSrc) != "skills" {
		return fmt.Errorf("skills source %q must end with a skills directory component", skillsSrc)
	}
	distDir := filepath.Dir(skillsSrc)
	if distDir == "." || distDir == skillsSrc {
		return fmt.Errorf("cannot establish distribution directory for skills source %q", skillsSrc)
	}
	distRoot := filepath.Dir(distDir)
	if distRoot == "." || distRoot == distDir {
		return fmt.Errorf("cannot establish distribution root above %q", distDir)
	}
	for _, path := range []string{distRoot, distDir, skillsSrc} {
		if _, err := requireInstallSkillTreeRoot(path); err != nil {
			return err
		}
	}
	return nil
}

func hashInstallSkillTree(root string) (string, error) {
	info, err := requireInstallSkillTreeRoot(root)
	if err != nil {
		return "", err
	}
	hash := sha256.New()
	var rootPermissions [4]byte
	binary.BigEndian.PutUint32(rootPermissions[:], uint32(info.Mode().Perm()))
	if err := writeInstallTreeFrame(hash, 'r', rootPermissions[:]); err != nil {
		return "", err
	}
	err = filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		info, err := os.Lstat(path)
		if err != nil {
			return err
		}
		if info.Mode()&fs.ModeSymlink != 0 {
			return fmt.Errorf("contains symlink %q", path)
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		if rel == "." {
			return nil
		}
		if info.IsDir() {
			var permissions [4]byte
			binary.BigEndian.PutUint32(permissions[:], uint32(info.Mode().Perm()))
			return writeInstallTreeFrame(hash, 'd', []byte(filepath.ToSlash(rel)), permissions[:])
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("contains non-regular file %q", path)
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

func writeInstallTreeFrame(writer io.Writer, kind byte, fields ...[]byte) error {
	if _, err := writer.Write([]byte{kind}); err != nil {
		return err
	}
	for _, field := range fields {
		var length [8]byte
		binary.BigEndian.PutUint64(length[:], uint64(len(field)))
		if _, err := writer.Write(length[:]); err != nil {
			return err
		}
		if _, err := writer.Write(field); err != nil {
			return err
		}
	}
	return nil
}

func copyInstallSkillTree(src string, dest string) error {
	return filepath.WalkDir(src, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		info, err := os.Lstat(path)
		if err != nil {
			return err
		}
		if info.Mode()&fs.ModeSymlink != 0 {
			return fmt.Errorf("contains symlink %q", path)
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := dest
		if rel != "." {
			target = filepath.Join(dest, rel)
		}
		if info.IsDir() {
			if err := os.MkdirAll(target, info.Mode().Perm()); err != nil {
				return err
			}
			return os.Chmod(target, info.Mode().Perm())
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("contains non-regular file %q", path)
		}
		return copyFileWithModeForInstall(path, target, info.Mode().Perm())
	})
}

func publishStagedSkill(staged string, dest string, backup string, recorded string, desired string, legacy bool, owned bool) (bool, error) {
	if err := os.MkdirAll(filepath.Dir(backup), 0o755); err != nil {
		return false, err
	}
	if _, err := os.Lstat(backup); err == nil {
		return false, fmt.Errorf("staged skill backup path %s already exists", backup)
	} else if !os.IsNotExist(err) {
		return false, err
	}
	hadDestination := false
	if _, err := os.Lstat(dest); err == nil {
		if !owned {
			return false, fmt.Errorf("skill destination %s appeared after preflight and is not managed by Loaf", dest)
		}
		hadDestination = true
		if err := os.Rename(dest, backup); err != nil {
			return false, err
		}
		if !legacy {
			actual, hashErr := hashInstallSkillTree(backup)
			if hashErr != nil || (actual != recorded && actual != desired) {
				if rollbackErr := os.Rename(backup, dest); rollbackErr != nil {
					return true, fmt.Errorf("verify managed skill %s after preflight: %v; rollback failed: %v; recovery backup retained at %s", dest, hashErr, rollbackErr, backup)
				}
				return false, fmt.Errorf("managed skill %s changed after preflight; existing destination restored", dest)
			}
		}
	} else if !os.IsNotExist(err) {
		return false, err
	}
	if err := os.Rename(staged, dest); err != nil {
		if !hadDestination {
			return false, fmt.Errorf("publish staged skill %s to %s: %w", staged, dest, err)
		}
		if rollbackErr := os.Rename(backup, dest); rollbackErr != nil {
			return true, fmt.Errorf("publish staged skill %s to %s: %w; rollback failed: %v; recovery backup retained at %s", staged, dest, err, rollbackErr, backup)
		}
		return false, fmt.Errorf("publish staged skill %s to %s: %w; existing destination restored", staged, dest, err)
	}
	return false, nil
}

func retireManagedSkill(dest string, backup string, recorded string, legacy bool) (bool, error) {
	if _, err := os.Lstat(dest); os.IsNotExist(err) {
		return false, nil
	} else if err != nil {
		return false, err
	}
	if err := os.MkdirAll(filepath.Dir(backup), 0o755); err != nil {
		return false, err
	}
	if err := os.Rename(dest, backup); err != nil {
		return false, err
	}
	if !legacy {
		actual, err := hashInstallSkillTree(backup)
		if err != nil || actual != recorded {
			if rollbackErr := os.Rename(backup, dest); rollbackErr != nil {
				return true, fmt.Errorf("verify stale managed skill %s: %v; rollback failed: %v; recovery backup retained at %s", dest, err, rollbackErr, backup)
			}
			return false, fmt.Errorf("stale managed skill %s changed after preflight; existing destination restored", dest)
		}
	}
	return false, nil
}

func mergeTargetDirIfExists(src string, dest string) error {
	if !dirExistsForInstall(src) {
		return nil
	}
	return copyDirContentsForInstall(src, dest)
}

func copyDirContentsForInstall(src string, dest string) error {
	return filepath.WalkDir(src, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		info, err := os.Lstat(path)
		if err != nil {
			return err
		}
		if info.Mode()&fs.ModeSymlink != 0 {
			return fmt.Errorf("contains symlink %q", path)
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		if rel == "." {
			return os.MkdirAll(dest, 0o755)
		}
		target := filepath.Join(dest, rel)
		if info.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("contains non-regular file %q", path)
		}
		return copyFileWithModeForInstall(path, target, info.Mode().Perm())
	})
}

func copyFileForInstall(src string, dest string) error {
	info, err := os.Stat(src)
	if err != nil {
		return err
	}
	return copyFileWithModeForInstall(src, dest, info.Mode().Perm())
}

func copyFileWithModeForInstall(src string, dest string, mode fs.FileMode) error {
	body, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return err
	}
	return os.WriteFile(dest, body, mode)
}

func writeInstallMarker(configDir string, version string) error {
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(configDir, loafInstallMarkerFile), []byte(version+"\n"), 0o644)
}

func writeInstallRecord(options targetInstallOptions, skillsDir string) error {
	homeDir := installHomeDir(options)
	if homeDir == "" {
		return nil
	}
	record := installTargetRecord{
		Version:   options.Version,
		Target:    options.Target,
		ConfigDir: options.ConfigDir,
		SkillsDir: skillsDir,
	}
	body, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		return err
	}
	body = append(body, '\n')
	path := installRecordPath(homeDir, options.Target)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, body, 0o644)
}

func installRecordPath(homeDir string, target string) string {
	return filepath.Join(homeDir, ".agents", "loaf", "install-targets", target+".json")
}

func renderCodexHookExecutable(rawHook json.RawMessage, executable string) (json.RawMessage, error) {
	return renderCodexHookExecutableForOS(rawHook, executable, runtime.GOOS)
}

func renderCodexHookExecutableForOS(rawHook json.RawMessage, executable string, goos string) (json.RawMessage, error) {
	hook, err := decodeCodexHookObject(rawHook)
	if err != nil {
		return nil, err
	}
	handlers, ok := hook["hooks"].([]any)
	if !ok {
		return nil, errors.New("matcher group hooks must be an array")
	}
	if len(handlers) != 1 {
		return nil, errors.New("Loaf Codex matcher group must contain exactly one command handler")
	}
	for _, rawHandler := range handlers {
		handler, ok := rawHandler.(map[string]any)
		if !ok {
			return nil, errors.New("matcher handler must be an object")
		}
		command, ok := handler["command"].(string)
		if !ok {
			return nil, errors.New("Loaf Codex matcher group command must be a string")
		}
		if command != codexJournalExecutablePlaceholder+codexJournalHookCommandSuffix && command != codexJournalHookCommandTemplate {
			return nil, errors.New("Loaf Codex matcher group contains an unexpected command")
		}
		rawWindowsCommand, hasWindowsCommand := handler["commandWindows"]
		if hasWindowsCommand {
			windowsCommand, ok := rawWindowsCommand.(string)
			if !ok || (windowsCommand != codexJournalExecutablePlaceholder+codexJournalHookCommandSuffix && windowsCommand != codexJournalHookCommandTemplate) {
				return nil, errors.New("Loaf Codex matcher group contains an unexpected Windows command")
			}
		}
		if goos == "windows" {
			renderedWindowsCommand, err := codexWindowsJournalContextCommand(executable)
			if err != nil {
				return nil, err
			}
			handler["command"] = renderedWindowsCommand
			handler["commandWindows"] = renderedWindowsCommand
		} else {
			handler["command"] = journalContextShellQuote(executable) + codexJournalHookCommandSuffix
		}
		if hasWindowsCommand && goos != "windows" {
			delete(handler, "commandWindows")
		}
	}
	if matcher, _ := hook["matcher"].(string); matcher != codexJournalHookMatcher {
		return nil, errors.New("Loaf Codex matcher group contains an unexpected matcher")
	}
	return json.Marshal(hook)
}

func isValidCodexMatcherGroup(hook map[string]any) bool {
	if hook == nil {
		return false
	}
	for key, value := range hook {
		switch key {
		case "matcher":
			if value != nil {
				if _, ok := value.(string); !ok {
					return false
				}
			}
		case "hooks":
		default:
			return false
		}
	}
	handlers := []any{}
	if rawHandlers, ok := hook["hooks"]; ok {
		var valid bool
		handlers, valid = rawHandlers.([]any)
		if !valid {
			return false
		}
	}
	for _, handler := range handlers {
		handlerMap, ok := handler.(map[string]any)
		if !ok || len(handlerMap) == 0 {
			return false
		}
		handlerType, ok := handlerMap["type"].(string)
		if !ok {
			return false
		}
		switch handlerType {
		case "prompt", "agent":
			if len(handlerMap) != 1 {
				return false
			}
		case "command":
			if _, canonical := handlerMap["commandWindows"]; canonical {
				if _, alias := handlerMap["command_windows"]; alias {
					return false
				}
			}
			for key, value := range handlerMap {
				switch key {
				case "type":
				case "command":
					if _, ok := value.(string); !ok {
						return false
					}
				case "statusMessage", "commandWindows", "command_windows":
					if value != nil {
						if _, ok := value.(string); !ok {
							return false
						}
					}
				case "timeout":
					if value != nil {
						if _, ok := codexHookUint64(value); !ok {
							return false
						}
					}
				case "async":
					if _, ok := value.(bool); !ok {
						return false
					}
				default:
					return false
				}
			}
			if _, ok := handlerMap["command"]; !ok {
				return false
			}
		default:
			return false
		}
	}
	return true
}

func decodeCodexHookObject(rawHook json.RawMessage) (map[string]any, error) {
	var hook map[string]any
	decoder := json.NewDecoder(bytes.NewReader(rawHook))
	decoder.UseNumber()
	if err := decoder.Decode(&hook); err != nil {
		return nil, err
	}
	if hook == nil {
		return nil, errors.New("matcher group must be an object")
	}
	return hook, nil
}

func codexHookUint64(value any) (uint64, bool) {
	switch value := value.(type) {
	case json.Number:
		parsed, err := strconv.ParseUint(string(value), 10, 64)
		if err != nil {
			return 0, false
		}
		return parsed, true
	case float64:
		// A float64 can represent every integer exactly only through 2^53.
		// Reject larger values instead of accepting a rounded value that may
		// differ from the JSON integer or exceed uint64's range.
		const maxSafeInteger = float64(1 << 53)
		if math.IsNaN(value) || math.IsInf(value, 0) || value < 0 || value > maxSafeInteger || math.Trunc(value) != value {
			return 0, false
		}
		return uint64(value), true
	default:
		return 0, false
	}
}

func codexWindowsJournalContextCommand(executable string) (string, error) {
	if !isCanonicalWindowsExecutablePath(executable) {
		return "", errors.New("Codex Windows command requires a canonical absolute Windows executable path")
	}
	// The outer quote makes cmd.exe /C pass the complete command through as a
	// single command line; the inner quotes protect spaces and cmd metacharacters
	// in the executable path.
	return `""` + executable + `"` + codexJournalHookCommandSuffix + `"`, nil
}

func isCanonicalWindowsExecutablePath(path string) bool {
	if path == "" || strings.Contains(path, "/") || strings.ContainsAny(path, `%!"`) {
		return false
	}
	for _, b := range []byte(path) {
		if b < 0x20 || b == 0x7f {
			return false
		}
	}
	if strings.HasPrefix(path, `\\.\`) || strings.HasPrefix(path, `\\?\`) {
		return false
	}
	if strings.HasPrefix(path, `\\`) {
		parts := strings.Split(path[2:], `\`)
		return len(parts) >= 3 && parts[0] != "" && parts[1] != "" && windowsPathPartsCanonical(parts[2:])
	}
	if len(path) < 4 || !isASCIIWindowsDriveLetter(path[0]) || path[1] != ':' || path[2] != '\\' {
		return false
	}
	return windowsPathPartsCanonical(strings.Split(path[3:], `\`))
}

func windowsPathPartsCanonical(parts []string) bool {
	if len(parts) == 0 || parts[len(parts)-1] == "" {
		return false
	}
	for _, part := range parts {
		if part == "" || part == "." || part == ".." {
			return false
		}
	}
	return true
}

func isASCIIWindowsDriveLetter(value byte) bool {
	return (value >= 'A' && value <= 'Z') || (value >= 'a' && value <= 'z')
}

// codexHookOwnershipForOS recognizes only the exact Loaf one-handler shape. A
// recognizable command inside a group carrying anything else reports a conflict
// rather than ownership, which is what keeps recognition from claiming — and a
// reconcile from rewriting — a user group Loaf's command was pasted into.
func codexHookOwnershipForOS(hook map[string]any, goos string) (owned bool, conflict bool) {
	matcher, _ := hook["matcher"].(string)
	handlers, ok := hook["hooks"].([]any)
	if !ok {
		return false, false
	}
	containsLoafCommand := false
	for _, rawHandler := range handlers {
		handler, ok := rawHandler.(map[string]any)
		if !ok {
			continue
		}
		command, ok := handler["command"].(string)
		if ok && strings.Contains(command, codexJournalHookCommandSuffix) {
			containsLoafCommand = true
		}
		windowsCommand, ok := handler["commandWindows"].(string)
		if ok && strings.Contains(windowsCommand, codexJournalHookCommandSuffix) {
			containsLoafCommand = true
		}
	}
	if !containsLoafCommand {
		return false, false
	}
	if matcher != codexJournalHookMatcher || len(hook) != 2 || len(handlers) != 1 {
		return false, true
	}
	handler, ok := handlers[0].(map[string]any)
	if !ok || handler["type"] != "command" {
		return false, true
	}
	command, ok := handler["command"].(string)
	if !ok {
		return false, true
	}
	if goos == "windows" {
		windowsCommand, windowsOK := handler["commandWindows"].(string)
		if len(handler) != 3 || !windowsOK || command != windowsCommand || !isExactCodexJournalHookCommandWindows(command) {
			return false, true
		}
	} else if len(handler) != 2 || !isExactCodexJournalHookCommand(command) {
		return false, true
	}
	return true, false
}

func isExactCodexJournalHookCommand(command string) bool {
	if command == codexJournalExecutablePlaceholder+codexJournalHookCommandSuffix || command == codexJournalHookCommandTemplate {
		return true
	}
	if !strings.HasPrefix(command, "'") || !strings.HasSuffix(command, codexJournalHookCommandSuffix) {
		return false
	}
	quotedEnd := len(command) - len(codexJournalHookCommandSuffix)
	if quotedEnd < 2 || command[quotedEnd-1] != '\'' {
		return false
	}
	quoted := command[:quotedEnd]
	path := strings.TrimSuffix(strings.TrimPrefix(quoted, "'"), "'")
	path = strings.ReplaceAll(path, "'\\''", "'")
	return filepath.IsAbs(path) && journalContextShellQuote(path) == quoted
}

func isExactCodexJournalHookCommandWindows(command string) bool {
	if command == codexJournalExecutablePlaceholder+codexJournalHookCommandSuffix || command == codexJournalHookCommandTemplate {
		return true
	}
	if !strings.HasPrefix(command, `""`) || !strings.HasSuffix(command, `"`) {
		return false
	}
	body := command[2 : len(command)-1]
	path, ok := strings.CutSuffix(body, `"`+codexJournalHookCommandSuffix)
	if !ok {
		return false
	}
	want, err := codexWindowsJournalContextCommand(path)
	return err == nil && want == command
}

func installHookSignature(hook map[string]any) string {
	command, hasCommand := hook["command"].(string)
	prompt, hasPrompt := hook["prompt"].(string)
	matcher, _ := hook["matcher"].(string)
	condition, _ := hook["if"].(string)
	switch {
	case hasCommand:
		return fmt.Sprintf("command:%s|matcher:%s|if:%s", command, matcher, condition)
	case hasPrompt:
		return fmt.Sprintf("prompt:%s|matcher:%s|if:%s", prompt, matcher, condition)
	default:
		return ""
	}
}

// resolveInstallPath returns an absolute, symlink-resolved path when possible.
// Config directories may legitimately be symlinks; resolved form is the goal so
// relative links match the filesystem resolver. Missing paths fall back to Abs.
func resolveInstallPath(path string) string {
	abs, err := filepath.Abs(path)
	if err != nil {
		return path
	}
	resolved, err := filepath.EvalSymlinks(abs)
	if err != nil {
		return abs
	}
	return resolved
}

func dirExistsForInstall(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

func fileExistsForInstall(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}
