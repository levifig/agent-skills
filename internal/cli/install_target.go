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
)

const (
	loafInstallMarkerFile = ".loaf-version"
	loafHookMarker        = "loaf-managed"
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
	// Journal-first hook signatures (SPEC-056).
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

var codexHookEvents = map[string]bool{
	"SessionStart":      true,
	"SubagentStart":     true,
	"PreToolUse":        true,
	"PermissionRequest": true,
	"PostToolUse":       true,
	"PreCompact":        true,
	"PostCompact":       true,
	"UserPromptSubmit":  true,
	"SubagentStop":      true,
	"Stop":              true,
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
}

type codexHooksFile struct {
	Version     int                         `json:"version,omitempty"`
	Description string                      `json:"description,omitempty"`
	Hooks       map[string][]map[string]any `json:"hooks"`
}

type codexHooksRawFile struct {
	Description json.RawMessage              `json:"description,omitempty"`
	Version     json.RawMessage              `json:"-"`
	Hooks       map[string][]json.RawMessage `json:"hooks"`
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
	if err := syncManagedSkillsDirIfExists(filepath.Join(options.DistDir, "skills"), skillsDest); err != nil {
		return err
	}
	for _, dir := range []string{"agents", "commands", "plugins", "templates"} {
		if err := syncTargetSubdir(options.DistDir, options.ConfigDir, dir); err != nil {
			return err
		}
	}
	if err := writeInstallMarker(options.ConfigDir, options.Version); err != nil {
		return err
	}
	return writeInstallRecord(options, skillsDest)
}

func installCursorTarget(options targetInstallOptions) error {
	skillsDest := installSkillsDestination(options)
	if err := syncManagedSkillsDirIfExists(filepath.Join(options.DistDir, "skills"), skillsDest); err != nil {
		return err
	}
	if err := os.RemoveAll(filepath.Join(options.ConfigDir, "commands")); err != nil {
		return err
	}
	if err := syncTargetDirIfExists(filepath.Join(options.DistDir, "agents"), filepath.Join(options.ConfigDir, "agents")); err != nil {
		return err
	}
	if err := mergeHookFiles(filepath.Join(options.ConfigDir, "hooks.json"), filepath.Join(options.DistDir, "hooks.json")); err != nil {
		return err
	}
	if options.Upgrade {
		for _, file := range obsoleteCursorHookFiles {
			if err := os.Remove(filepath.Join(options.ConfigDir, "hooks", filepath.FromSlash(file))); err != nil && !os.IsNotExist(err) {
				return err
			}
		}
	}
	if err := mergeTargetDirIfExists(filepath.Join(options.DistDir, "hooks"), filepath.Join(options.ConfigDir, "hooks")); err != nil {
		return err
	}
	if err := syncTargetDirIfExists(filepath.Join(options.DistDir, "templates"), filepath.Join(options.ConfigDir, "templates")); err != nil {
		return err
	}
	if err := writeInstallMarker(options.ConfigDir, options.Version); err != nil {
		return err
	}
	return writeInstallRecord(options, skillsDest)
}

func installCodexTarget(options targetInstallOptions) error {
	homeDir := installHomeDir(options)
	codexHome := options.CodexHome
	if codexHome == "" {
		codexHome = filepath.Join(homeDir, ".codex")
	}
	skillsDest := installSkillsDestination(options)
	if err := syncManagedSkillsDirIfExists(filepath.Join(options.DistDir, "skills"), skillsDest); err != nil {
		return err
	}
	if err := mergeCodexHookFiles(filepath.Join(codexHome, "hooks.json"), filepath.Join(options.DistDir, ".codex", "hooks.json"), options.ProjectRoot, options.CodexRuleOperations); err != nil {
		return err
	}
	if err := installCodexJournalRuleWithOperations(options, codexHome, options.CodexRuleOperations); err != nil {
		return err
	}
	if err := writeInstallMarker(options.ConfigDir, options.Version); err != nil {
		return err
	}
	return writeInstallRecord(options, skillsDest)
}

func installAmpTarget(options targetInstallOptions) error {
	skillsDest := installSkillsDestination(options)
	if err := syncManagedSkillsDirIfExists(filepath.Join(options.DistDir, "skills"), skillsDest); err != nil {
		return err
	}
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
	if options.Target == "amp" && options.AmpSkillsDir != "" {
		return options.AmpSkillsDir
	}
	return filepath.Join(installHomeDir(options), ".agents", "skills")
}

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
	// Complete all ownership checks before mutating the destination.
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
			return fmt.Errorf("managed skill %q was modified; refusing to overwrite or remove", skill)
		}
	}
	for _, skill := range sourceSkills {
		if _, owned := previous.digests[skill]; !owned {
			if _, err := os.Lstat(filepath.Join(dest, skill)); err == nil {
				return fmt.Errorf("skill destination %q already exists and is not managed by Loaf", skill)
			} else if !os.IsNotExist(err) {
				return err
			}
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
		if _, keep := current[skill]; !keep {
			retain, err := retireManagedSkill(filepath.Join(dest, skill), filepath.Join(stageRoot, "backups", skill), previous.digests[skill], previous.legacy)
			if retain {
				retainStageRoot = true
			}
			if err != nil {
				return err
			}
		}
	}
	for _, skill := range sourceSkills {
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
	manifest := managedSkillsManifestV2{Version: 2, Skills: make([]managedSkillDigest, 0, len(sourceSkills))}
	for _, skill := range sourceSkills {
		manifest.Skills = append(manifest.Skills, managedSkillDigest{Name: skill, SHA256: current[skill]})
	}
	return writeManagedSkillsManifest(dest, manifest)
}

func listInstallSkillDirs(path string) ([]string, error) {
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
	body, err := os.ReadFile(path)
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

func hashInstallSkillTree(root string) (string, error) {
	info, err := os.Lstat(root)
	if err != nil {
		return "", err
	}
	if !info.IsDir() || info.Mode()&fs.ModeSymlink != 0 {
		return "", fmt.Errorf("not a directory or is a symlink")
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
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		if rel == "." {
			return os.MkdirAll(dest, 0o755)
		}
		target := filepath.Join(dest, rel)
		if entry.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		info, err := entry.Info()
		if err != nil {
			return err
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

func mergeHookFiles(destPath string, loafPath string) error {
	if !fileExistsForInstall(loafPath) {
		return nil
	}
	existing, err := loadCodexHooksFile(destPath)
	if err != nil {
		return err
	}
	loafHooks, err := loadCodexHooksFile(loafPath)
	if err != nil {
		return err
	}
	merged := codexHooksFile{Version: 1, Hooks: map[string][]map[string]any{}}
	seen := map[string]bool{}
	for hookType := range existing.Hooks {
		seen[hookType] = true
	}
	for hookType := range loafHooks.Hooks {
		seen[hookType] = true
	}
	for hookType := range seen {
		var hooks []map[string]any
		for _, hook := range existing.Hooks[hookType] {
			if !isLoafInstallHook(hook) {
				hooks = append(hooks, hook)
			}
		}
		hooks = append(hooks, loafHooks.Hooks[hookType]...)
		if len(hooks) > 0 {
			merged.Hooks[hookType] = hooks
		}
	}
	if len(merged.Hooks) == 0 {
		merged.Hooks = nil
	}
	body, err := json.MarshalIndent(merged, "", "  ")
	if err != nil {
		return err
	}
	body = append(body, '\n')
	if err := os.MkdirAll(filepath.Dir(destPath), 0o755); err != nil {
		return err
	}
	return os.WriteFile(destPath, body, 0o644)
}

// mergeCodexHookFiles writes the current Codex hooks schema. Existing valid
// user groups survive; recognized legacy Loaf groups are retired, while
// malformed or unowned content is refused. The distributed adapter carries a
// placeholder that is rendered to the trusted absolute executable at install.
func mergeCodexHookFiles(destPath string, loafPath string, projectRoot string, operations *codexRuleInstallOperations) error {
	return mergeCodexHookFilesForOS(destPath, loafPath, projectRoot, operations, runtime.GOOS)
}

func mergeCodexHookFilesForOS(destPath string, loafPath string, projectRoot string, operations *codexRuleInstallOperations, goos string) error {
	return mergeCodexHookFilesForOSWithExecutable(destPath, loafPath, projectRoot, operations, goos, "")
}

func mergeCodexHookFilesForOSWithExecutable(destPath string, loafPath string, projectRoot string, operations *codexRuleInstallOperations, goos string, executableOverride string) error {
	if !fileExistsForInstall(loafPath) {
		return nil
	}
	loafHooks, err := loadCodexHooksRawFileStrict(loafPath)
	if err != nil {
		return err
	}
	loafExecutable := executableOverride
	for hookType, hooks := range loafHooks.Hooks {
		for index, rawHook := range hooks {
			if !bytes.Contains(rawHook, []byte(codexJournalExecutablePlaceholder)) && !bytes.Contains(rawHook, []byte(codexJournalHookCommandTemplate)) {
				continue
			}
			if loafExecutable == "" {
				loafExecutable, err = trustedCodexJournalExecutable(projectRoot, operations)
				if err != nil {
					return err
				}
			}
			rendered, renderErr := renderCodexHookExecutableForOS(rawHook, loafExecutable, goos)
			if renderErr != nil {
				return fmt.Errorf("render Codex Loaf hook %s[%d]: %w", hookType, index, renderErr)
			}
			if bytes.Contains(rendered, []byte(codexJournalExecutablePlaceholder)) || bytes.Contains(rendered, []byte(codexJournalHookCommandTemplate)) {
				return fmt.Errorf("render Codex Loaf hook %s[%d]: executable placeholder remains", hookType, index)
			}
			loafHooks.Hooks[hookType][index] = rendered
		}
	}
	existing, err := loadCodexHooksRawFileStrict(destPath)
	if err != nil {
		return err
	}
	merged := codexHooksRawFile{Description: existing.Description, Hooks: map[string][]json.RawMessage{}}
	retiredLegacy := false
	for hookType, hooks := range existing.Hooks {
		if len(hooks) == 0 {
			merged.Hooks[hookType] = []json.RawMessage{}
			continue
		}
		for _, rawHook := range hooks {
			hook, err := decodeCodexHookObject(rawHook)
			if err != nil {
				return fmt.Errorf("parse Codex hooks matcher group in %s: %w", destPath, err)
			}
			if owned, conflict := codexHookOwnershipForOS(hook, goos); conflict {
				return fmt.Errorf("Codex hooks file %s contains a modified Loaf SessionStart matcher group in %s; refusing to retire or duplicate it", destPath, hookType)
			} else if owned {
				retiredLegacy = true
				continue
			}
			if !isValidCodexMatcherGroup(hook) {
				if isLoafInstallHookForOS(hook, goos) {
					retiredLegacy = true
					continue
				}
				return fmt.Errorf("Codex hooks file %s contains an unsupported matcher entry in %s; preserve it manually or remove it before installing Loaf", destPath, hookType)
			}
			// Preserve each valid user matcher group as a whole. Any modified
			// recognizable Loaf group was rejected above rather than edited.
			merged.Hooks[hookType] = append(merged.Hooks[hookType], rawHook)
		}
	}
	for hookType, hooks := range loafHooks.Hooks {
		for _, rawHook := range hooks {
			hook, err := decodeCodexHookObject(rawHook)
			if err != nil {
				return fmt.Errorf("parse generated Codex hooks matcher group in %s: %w", loafPath, err)
			}
			if !isValidCodexMatcherGroup(hook) {
				return fmt.Errorf("generated Codex hooks file %s contains an unsupported matcher entry in %s", loafPath, hookType)
			}
			merged.Hooks[hookType] = append(merged.Hooks[hookType], rawHook)
		}
	}
	if len(existing.Version) > 0 && !retiredLegacy {
		return fmt.Errorf("Codex hooks file %s contains legacy version metadata without a recognized Loaf hook to retire; refusing to rewrite user/current content", destPath)
	}
	body, err := json.MarshalIndent(merged, "", "  ")
	if err != nil {
		return err
	}
	body = append(body, '\n')
	if err := os.MkdirAll(filepath.Dir(destPath), 0o755); err != nil {
		return err
	}
	return os.WriteFile(destPath, body, 0o644)
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

func loadCodexHooksRawFileStrict(path string) (codexHooksRawFile, error) {
	if !fileExistsForInstall(path) {
		return codexHooksRawFile{Hooks: map[string][]json.RawMessage{}}, nil
	}
	body, err := os.ReadFile(path)
	if err != nil {
		return codexHooksRawFile{}, err
	}
	var topLevel map[string]json.RawMessage
	if err := json.Unmarshal(body, &topLevel); err != nil {
		return codexHooksRawFile{}, fmt.Errorf("parse Codex hooks file %s: %w", path, err)
	}
	if topLevel == nil {
		return codexHooksRawFile{}, fmt.Errorf("parse Codex hooks file %s: top-level value must be an object", path)
	}
	for key := range topLevel {
		if key != "version" && key != "description" && key != "hooks" {
			return codexHooksRawFile{}, fmt.Errorf("Codex hooks file %s contains unsupported top-level field %q", path, key)
		}
	}
	version := topLevel["version"]
	if len(version) > 0 {
		var value float64
		if strings.HasPrefix(strings.TrimSpace(string(version)), "\"") || json.Unmarshal(version, &value) != nil || value != 1 {
			return codexHooksRawFile{}, fmt.Errorf("parse Codex hooks file %s: legacy version must be numeric 1", path)
		}
	}
	var hooks map[string][]json.RawMessage
	if raw, ok := topLevel["hooks"]; ok {
		if strings.TrimSpace(string(raw)) == "null" {
			return codexHooksRawFile{}, fmt.Errorf("parse Codex hooks file %s: hooks must be an object", path)
		}
		var rawHooks map[string]json.RawMessage
		if err := json.Unmarshal(raw, &rawHooks); err != nil {
			return codexHooksRawFile{}, fmt.Errorf("parse Codex hooks file %s: %w", path, err)
		}
		hooks = make(map[string][]json.RawMessage, len(rawHooks))
		for event, rawEvent := range rawHooks {
			if strings.TrimSpace(string(rawEvent)) == "null" {
				return codexHooksRawFile{}, fmt.Errorf("parse Codex hooks file %s: event %q must be an array", path, event)
			}
			var eventHooks []json.RawMessage
			if err := json.Unmarshal(rawEvent, &eventHooks); err != nil {
				return codexHooksRawFile{}, fmt.Errorf("parse Codex hooks file %s: event %q must be an array", path, event)
			}
			if eventHooks == nil {
				eventHooks = []json.RawMessage{}
			}
			hooks[event] = eventHooks
		}
	}
	if hooks == nil {
		hooks = map[string][]json.RawMessage{}
	}
	for event := range hooks {
		if !codexHookEvents[event] {
			return codexHooksRawFile{}, fmt.Errorf("Codex hooks file %s contains unsupported hook event %q", path, event)
		}
	}
	description := topLevel["description"]
	if len(description) > 0 && strings.TrimSpace(string(description)) != "null" {
		var value string
		if err := json.Unmarshal(description, &value); err != nil {
			return codexHooksRawFile{}, fmt.Errorf("parse Codex hooks file %s: description must be a string", path)
		}
	}
	return codexHooksRawFile{Description: description, Version: version, Hooks: hooks}, nil
}

func loadCodexHooksFile(path string) (codexHooksFile, error) {
	if !fileExistsForInstall(path) {
		return codexHooksFile{Hooks: map[string][]map[string]any{}}, nil
	}
	body, err := os.ReadFile(path)
	if err != nil {
		return codexHooksFile{}, err
	}
	var hooks codexHooksFile
	if err := json.Unmarshal(body, &hooks); err != nil {
		return codexHooksFile{Hooks: map[string][]map[string]any{}}, nil
	}
	if hooks.Hooks == nil {
		hooks.Hooks = map[string][]map[string]any{}
	}
	return hooks, nil
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

func isLoafInstallHook(hook map[string]any) bool {
	return isLoafInstallHookForOS(hook, runtime.GOOS)
}

func isLoafInstallHookForOS(hook map[string]any, goos string) bool {
	if marker, ok := hook[loafHookMarker].(bool); ok && marker {
		return true
	}
	if signature := installHookSignature(hook); signature != "" && legacyLoafHookSignatures[signature] {
		return true
	}
	if command, ok := hook["command"].(string); ok && legacyLoafCommands[command] {
		return true
	}
	if prompt, ok := hook["prompt"].(string); ok {
		for _, prefix := range legacyLoafPromptPrefixes {
			if strings.HasPrefix(prompt, prefix) {
				return true
			}
		}
	}
	if isLoafCodexMatcherGroupForOS(hook, goos) {
		return true
	}
	return false
}

func isLoafCodexMatcherGroup(hook map[string]any) bool {
	return isLoafCodexMatcherGroupForOS(hook, runtime.GOOS)
}

func isLoafCodexMatcherGroupForOS(hook map[string]any, goos string) bool {
	owned, conflict := codexHookOwnershipForOS(hook, goos)
	return owned && !conflict
}

// codexHookOwnership recognizes only the exact Loaf one-handler shape. A
// recognizable suffix in a modified group is an ownership conflict, not a
// reason to delete a whole user group.
func codexHookOwnership(hook map[string]any) (owned bool, conflict bool) {
	return codexHookOwnershipForOS(hook, runtime.GOOS)
}

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

func dirExistsForInstall(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

func fileExistsForInstall(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}
