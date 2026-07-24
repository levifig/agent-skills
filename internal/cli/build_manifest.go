package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const (
	targetBuildManifestFile   = ".loaf-target-manifest.json"
	targetInstallManifestFile = ".loaf-managed-target.json"
)

type targetAdapterManifest struct {
	Version                   int                     `json:"version"`
	Target                    string                  `json:"target"`
	PackageVersion            string                  `json:"package_version"`
	CapabilityContractVersion int                     `json:"capability_contract_version"`
	Adapters                  []string                `json:"adapters"`
	Artifacts                 []targetAdapterArtifact `json:"artifacts"`
}

type targetAdapterArtifact struct {
	ID          string  `json:"id"`
	Kind        string  `json:"kind"`
	SourcePath  string  `json:"source_path,omitempty"`
	Destination string  `json:"destination"`
	SHA256      string  `json:"sha256"`
	Mode        *uint32 `json:"mode,omitempty"`
}

type targetAdapterSnapshot struct {
	path   string
	exists bool
	body   []byte
	mode   fs.FileMode
}

type targetAdapterInstallOperations struct {
	beforePublish   func() error
	beforeArtifact  func(string) error
	afterArtifact   func(string) error
	restoreSnapshot func(targetAdapterSnapshot) error
}

func writeNativeBuildTargetManifest(root string, target string) error {
	version, err := nativeBuildPackageVersion(root)
	if err != nil {
		return err
	}
	contractBody, err := os.ReadFile(filepath.Join(root, TargetCapabilityEvidenceRecordPath))
	if err != nil {
		return fmt.Errorf("read target capability evidence for build manifest: %w", err)
	}
	contract, err := DecodeTargetCapabilityEvidence(contractBody)
	if err != nil {
		return fmt.Errorf("validate target capability evidence for build manifest: %w", err)
	}
	adapters := targetCapabilityAdapters(contract, target)
	if len(adapters) == 0 {
		return fmt.Errorf("target capability evidence has no adapter for build target %q", target)
	}
	outputDir := nativeBuildTargetOutputDir(root, target)
	artifacts, err := collectTargetAdapterArtifacts(target, outputDir)
	if err != nil {
		return err
	}
	artifacts = append([]targetAdapterArtifact{{
		ID:          "managed-instructions",
		Kind:        "instruction",
		Destination: "project-instructions",
		SHA256:      fencedContentFingerprint(generateFencedContent()),
	}}, artifacts...)
	manifest := targetAdapterManifest{
		Version:                   1,
		Target:                    target,
		PackageVersion:            version,
		CapabilityContractVersion: contract.ContractVersion,
		Adapters:                  adapters,
		Artifacts:                 artifacts,
	}
	return writeTargetAdapterManifest(filepath.Join(outputDir, targetBuildManifestFile), manifest)
}

func targetCapabilityAdapters(contract TargetCapabilityEvidenceContract, target string) []string {
	seen := map[string]bool{}
	for _, record := range contract.Records {
		if record.Target == target && record.Context.Adapter != "" {
			seen[record.Context.Adapter] = true
		}
	}
	adapters := make([]string, 0, len(seen))
	for adapter := range seen {
		adapters = append(adapters, adapter)
	}
	sort.Strings(adapters)
	return adapters
}

func collectTargetAdapterArtifacts(target string, outputDir string) ([]targetAdapterArtifact, error) {
	var paths []string
	switch target {
	case "claude-code":
		paths = []string{"hooks"}
	case "opencode":
		paths = []string{"plugins"}
	case "cursor":
		paths = []string{"hooks.json", "hooks"}
	case "codex":
		paths = []string{".codex/hooks.json"}
	case "amp":
		paths = []string{".amp/plugins/loaf.ts"}
	default:
		return nil, fmt.Errorf("unsupported target manifest target %q", target)
	}
	var artifacts []targetAdapterArtifact
	seen := map[string]bool{}
	for _, relative := range paths {
		fullPath := filepath.Join(outputDir, filepath.FromSlash(relative))
		info, err := os.Lstat(fullPath)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return nil, err
		}
		if info.Mode()&fs.ModeSymlink != 0 {
			return nil, fmt.Errorf("target adapter artifact %q is a symlink", relative)
		}
		if info.IsDir() {
			err = filepath.WalkDir(fullPath, func(path string, entry fs.DirEntry, walkErr error) error {
				if walkErr != nil {
					return walkErr
				}
				if path == fullPath {
					return nil
				}
				if entry.Type()&fs.ModeSymlink != 0 {
					return fmt.Errorf("target adapter artifact %q is a symlink", nativeBuildRelativePath(outputDir, path))
				}
				if entry.IsDir() {
					return nil
				}
				entryInfo, err := entry.Info()
				if err != nil {
					return err
				}
				if !entryInfo.Mode().IsRegular() {
					return fmt.Errorf("target adapter artifact %q is not a regular file", nativeBuildRelativePath(outputDir, path))
				}
				rel, err := filepath.Rel(outputDir, path)
				if err != nil {
					return err
				}
				return appendTargetAdapterArtifact(&artifacts, seen, target, outputDir, filepath.ToSlash(rel))
			})
			if err != nil {
				return nil, err
			}
			continue
		}
		if !info.Mode().IsRegular() {
			return nil, fmt.Errorf("target adapter artifact %q is not a regular file", relative)
		}
		if err := appendTargetAdapterArtifact(&artifacts, seen, target, outputDir, filepath.ToSlash(relative)); err != nil {
			return nil, err
		}
	}
	sort.Slice(artifacts, func(i, j int) bool { return artifacts[i].ID < artifacts[j].ID })
	return artifacts, nil
}

func appendTargetAdapterArtifact(artifacts *[]targetAdapterArtifact, seen map[string]bool, target string, outputDir string, sourcePath string) error {
	if seen[sourcePath] {
		return nil
	}
	seen[sourcePath] = true
	body, err := os.ReadFile(filepath.Join(outputDir, filepath.FromSlash(sourcePath)))
	if err != nil {
		return err
	}
	kind := "hook-file"
	destination := sourcePath
	switch target {
	case "amp":
		kind = "plugin"
		destination = "plugins/loaf.ts"
	case "opencode":
		if sourcePath == "plugins/hooks.ts" {
			kind = "plugin"
		}
	case "cursor":
		if sourcePath == "hooks.json" {
			kind = "hook-projection"
		}
	case "codex":
		kind = "hook-projection"
		destination = "hooks.json"
	case "claude-code":
		if sourcePath == "hooks/hooks.json" {
			kind = "hook-projection"
		}
	}
	digest := sha256Bytes(body)
	var mode *uint32
	if kind == "hook-projection" {
		digest, err = targetHookProjectionDigest(target, body, false)
		if err != nil {
			return fmt.Errorf("hash %s hook projection: %w", target, err)
		}
	} else {
		info, err := os.Lstat(filepath.Join(outputDir, filepath.FromSlash(sourcePath)))
		if err != nil {
			return err
		}
		value := uint32(info.Mode().Perm())
		mode = &value
	}
	*artifacts = append(*artifacts, targetAdapterArtifact{
		ID:          kind + ":" + sourcePath,
		Kind:        kind,
		SourcePath:  sourcePath,
		Destination: destination,
		SHA256:      digest,
		Mode:        mode,
	})
	return nil
}

func targetHookProjectionDigest(target string, body []byte, installed bool) (string, error) {
	switch target {
	case "cursor":
		var hooks codexHooksFile
		if err := json.Unmarshal(body, &hooks); err != nil {
			return "", err
		}
		projection := codexHooksFile{Version: 1, Hooks: map[string][]map[string]any{}}
		for event, entries := range hooks.Hooks {
			for _, entry := range entries {
				if isLoafInstallHook(entry) {
					projection.Hooks[event] = append(projection.Hooks[event], entry)
				}
			}
		}
		canonical, err := json.Marshal(projection)
		if err != nil {
			return "", err
		}
		return sha256Bytes(canonical), nil
	case "codex":
		hooks, err := decodeCodexHooksBodyStrict(body)
		if err != nil {
			return "", err
		}
		projection := codexHooksRawFile{Hooks: map[string][]json.RawMessage{}}
		for event, entries := range hooks.Hooks {
			for _, rawEntry := range entries {
				entry, err := decodeCodexHookObject(rawEntry)
				if err != nil {
					return "", err
				}
				if installed {
					owned, conflict := codexHookOwnership(entry)
					if conflict {
						return "", fmt.Errorf("modified Loaf matcher group")
					}
					if !owned {
						continue
					}
					handlers := entry["hooks"].([]any)
					handler := handlers[0].(map[string]any)
					handler["command"] = codexJournalExecutablePlaceholder + codexJournalHookCommandSuffix
					handler["commandWindows"] = codexJournalExecutablePlaceholder + codexJournalHookCommandSuffix
				} else if !bytes.Contains(rawEntry, []byte(codexJournalExecutablePlaceholder)) {
					continue
				}
				canonical, err := json.Marshal(entry)
				if err != nil {
					return "", err
				}
				projection.Hooks[event] = append(projection.Hooks[event], canonical)
			}
		}
		canonical, err := json.Marshal(projection)
		if err != nil {
			return "", err
		}
		return sha256Bytes(canonical), nil
	default:
		return sha256Bytes(body), nil
	}
}

func decodeCodexHooksBodyStrict(body []byte) (codexHooksRawFile, error) {
	tempDir, err := os.MkdirTemp("", "loaf-hooks-decode-*")
	if err != nil {
		return codexHooksRawFile{}, err
	}
	path := filepath.Join(tempDir, "hooks.json")
	if err := os.WriteFile(path, body, 0o600); err != nil {
		if cleanupErr := os.RemoveAll(tempDir); cleanupErr != nil {
			return codexHooksRawFile{}, fmt.Errorf("%w; clean up Codex hooks decode directory %s: %v", err, tempDir, cleanupErr)
		}
		return codexHooksRawFile{}, err
	}
	hooks, decodeErr := loadCodexHooksRawFileStrict(path)
	cleanupErr := os.RemoveAll(tempDir)
	if decodeErr != nil {
		if cleanupErr != nil {
			return codexHooksRawFile{}, fmt.Errorf("%w; clean up Codex hooks decode directory %s: %v", decodeErr, tempDir, cleanupErr)
		}
		return codexHooksRawFile{}, decodeErr
	}
	if cleanupErr != nil {
		return codexHooksRawFile{}, fmt.Errorf("clean up Codex hooks decode directory %s: %w", tempDir, cleanupErr)
	}
	return hooks, nil
}

func readTargetAdapterManifest(path string) (targetAdapterManifest, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return targetAdapterManifest{}, err
	}
	if !info.Mode().IsRegular() || info.Mode()&fs.ModeSymlink != 0 {
		return targetAdapterManifest{}, fmt.Errorf("target adapter manifest %s must be a regular file", path)
	}
	body, err := os.ReadFile(path)
	if err != nil {
		return targetAdapterManifest{}, err
	}
	if err := validateJSONNoDuplicateKeys(body); err != nil {
		return targetAdapterManifest{}, fmt.Errorf("read target adapter manifest: %w", err)
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	var manifest targetAdapterManifest
	if err := decoder.Decode(&manifest); err != nil {
		return targetAdapterManifest{}, fmt.Errorf("read target adapter manifest: %w", err)
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return targetAdapterManifest{}, fmt.Errorf("read target adapter manifest: trailing JSON values")
	}
	if err := validateTargetAdapterManifest(manifest); err != nil {
		return targetAdapterManifest{}, err
	}
	return manifest, nil
}

func validateTargetAdapterManifest(manifest targetAdapterManifest) error {
	if manifest.Version != 1 {
		return fmt.Errorf("unsupported target adapter manifest version %d", manifest.Version)
	}
	if _, ok := supportedCapabilityTargets[manifest.Target]; !ok {
		return fmt.Errorf("invalid target adapter manifest target %q", manifest.Target)
	}
	if manifest.PackageVersion == "" || manifest.CapabilityContractVersion != TargetCapabilityEvidenceContractVersion {
		return fmt.Errorf("invalid target adapter manifest metadata")
	}
	if len(manifest.Adapters) == 0 || !sort.StringsAreSorted(manifest.Adapters) {
		return fmt.Errorf("target adapter manifest adapters must be non-empty and sorted")
	}
	seenAdapters := map[string]bool{}
	for _, adapter := range manifest.Adapters {
		if adapter == "" || seenAdapters[adapter] {
			return fmt.Errorf("invalid or duplicate target adapter %q", adapter)
		}
		seenAdapters[adapter] = true
	}
	if len(manifest.Artifacts) == 0 || !sort.SliceIsSorted(manifest.Artifacts, func(i, j int) bool { return manifest.Artifacts[i].ID < manifest.Artifacts[j].ID }) {
		return fmt.Errorf("target adapter manifest artifacts must be non-empty and sorted")
	}
	seenIDs := map[string]bool{}
	seenDestinations := map[string]bool{}
	for _, artifact := range manifest.Artifacts {
		if artifact.ID == "" || seenIDs[artifact.ID] {
			return fmt.Errorf("invalid or duplicate target adapter artifact id %q", artifact.ID)
		}
		seenIDs[artifact.ID] = true
		if artifact.Kind != "instruction" && artifact.Kind != "hook-projection" && artifact.Kind != "hook-file" && artifact.Kind != "plugin" {
			return fmt.Errorf("invalid target adapter artifact kind %q", artifact.Kind)
		}
		if artifact.Kind == "instruction" {
			if artifact.SourcePath != "" || artifact.Destination != "project-instructions" {
				return fmt.Errorf("invalid managed instruction artifact paths")
			}
		} else {
			if !validTargetAdapterPath(artifact.SourcePath) || !validTargetAdapterPath(artifact.Destination) {
				return fmt.Errorf("invalid target adapter artifact path %q", artifact.SourcePath)
			}
			if seenDestinations[artifact.Destination] {
				return fmt.Errorf("duplicate target adapter artifact destination %q", artifact.Destination)
			}
			seenDestinations[artifact.Destination] = true
		}
		if artifact.Kind == "hook-file" || artifact.Kind == "plugin" {
			if artifact.Mode == nil || *artifact.Mode > 0o777 {
				return fmt.Errorf("invalid or missing target adapter artifact mode for %q", artifact.ID)
			}
		} else if artifact.Mode != nil {
			return fmt.Errorf("target adapter artifact kind %q must not declare a mode", artifact.Kind)
		}
		if !isHexString(artifact.SHA256) || len(artifact.SHA256) != 64 || strings.ToLower(artifact.SHA256) != artifact.SHA256 {
			return fmt.Errorf("invalid target adapter artifact digest for %q", artifact.ID)
		}
	}
	return nil
}

func validTargetAdapterPath(path string) bool {
	if path == "" || filepath.IsAbs(path) || strings.Contains(path, "\\") || filepath.ToSlash(filepath.Clean(path)) != path {
		return false
	}
	return fs.ValidPath(path) && path != "." && !strings.HasPrefix(path, "../")
}

func writeTargetAdapterManifest(path string, manifest targetAdapterManifest) error {
	sort.Strings(manifest.Adapters)
	sort.Slice(manifest.Artifacts, func(i, j int) bool { return manifest.Artifacts[i].ID < manifest.Artifacts[j].ID })
	if err := validateTargetAdapterManifest(manifest); err != nil {
		return err
	}
	body, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return err
	}
	body = append(body, '\n')
	return writeFileAtomically(path, body, 0o644)
}

func syncTargetAdapterManifest(options targetInstallOptions) error {
	buildPath := filepath.Join(options.DistDir, targetBuildManifestFile)
	desired, err := readTargetAdapterManifest(buildPath)
	if err != nil {
		return err
	}
	if desired.Target != options.Target {
		return fmt.Errorf("target adapter manifest target %q does not match install target %q", desired.Target, options.Target)
	}
	installedPath := filepath.Join(options.ConfigDir, targetInstallManifestFile)
	installed := targetAdapterManifest{}
	if _, err := os.Lstat(installedPath); err == nil {
		installed, err = readTargetAdapterManifest(installedPath)
		if err != nil {
			return err
		}
		if installed.Target != options.Target {
			return fmt.Errorf("installed target adapter manifest target %q does not match %q", installed.Target, options.Target)
		}
	} else if !os.IsNotExist(err) {
		return err
	}
	desiredByID := targetAdapterArtifactsByID(desired.Artifacts)
	installedByID := targetAdapterArtifactsByID(installed.Artifacts)
	states := map[string]targetAdapterSnapshot{}
	desiredDestinations := map[string]bool{}
	for _, artifact := range installed.Artifacts {
		if artifact.Kind == "instruction" {
			continue
		}
		path, err := targetAdapterDestination(options, artifact)
		if err != nil {
			return err
		}
		snapshot, err := readTargetAdapterSnapshot(path)
		if err != nil {
			return err
		}
		states[path] = snapshot
		if !snapshot.exists {
			continue
		}
		matchesInstalled, err := targetAdapterSnapshotMatchesArtifact(options.Target, artifact, snapshot)
		if err != nil {
			return fmt.Errorf("inspect managed target artifact %q: %w", artifact.ID, err)
		}
		matchesDesired := false
		if current, ok := desiredByID[artifact.ID]; ok {
			matchesDesired, err = targetAdapterSnapshotMatchesArtifact(options.Target, current, snapshot)
			if err != nil {
				return fmt.Errorf("inspect desired target artifact %q: %w", artifact.ID, err)
			}
		}
		if !matchesInstalled && !matchesDesired {
			return fmt.Errorf("managed target artifact %q was modified; refusing to overwrite or remove", artifact.ID)
		}
	}
	for _, artifact := range desired.Artifacts {
		if artifact.Kind == "instruction" {
			continue
		}
		if err := verifyTargetAdapterSource(options, artifact); err != nil {
			return err
		}
		path, err := targetAdapterDestination(options, artifact)
		if err != nil {
			return err
		}
		if _, ok := states[path]; !ok {
			snapshot, err := readTargetAdapterSnapshot(path)
			if err != nil {
				return err
			}
			states[path] = snapshot
		}
		desiredDestinations[path] = true
		if _, owned := installedByID[artifact.ID]; owned || !states[path].exists {
			continue
		}
		matchesDesired, err := targetAdapterSnapshotMatchesArtifact(options.Target, artifact, states[path])
		if err != nil {
			return fmt.Errorf("inspect target artifact migration %q: %w", artifact.ID, err)
		}
		if matchesDesired || targetAdapterLegacyOwnership(options.Target, artifact, states[path].body) {
			continue
		}
		if artifact.Kind == "hook-projection" && targetHookProjectionIsEmpty(options.Target, states[path].body) {
			continue
		}
		return fmt.Errorf("target artifact destination %q exists and is not managed by Loaf", artifact.Destination)
	}
	manifestSnapshot, err := readTargetAdapterSnapshot(installedPath)
	if err != nil {
		return err
	}
	changedPaths := make([]string, 0, len(states))
	for path := range states {
		changedPaths = append(changedPaths, path)
	}
	sort.Strings(changedPaths)
	if options.TargetAdapterOps != nil && options.TargetAdapterOps.beforePublish != nil {
		if err := options.TargetAdapterOps.beforePublish(); err != nil {
			return err
		}
	}
	for _, path := range changedPaths {
		current, err := readTargetAdapterSnapshot(path)
		if err != nil {
			return err
		}
		if !sameTargetAdapterSnapshot(current, states[path]) {
			return fmt.Errorf("target adapter destination %s changed during install", path)
		}
	}
	mutated := make([]targetAdapterSnapshot, 0, len(states)+1)
	mutatedPaths := map[string]bool{}
	fail := func(cause error) error {
		return rollbackTargetAdapterMutations(cause, mutated, options.TargetAdapterOps)
	}
	for _, artifact := range installed.Artifacts {
		if artifact.Kind == "instruction" {
			continue
		}
		if _, keep := desiredByID[artifact.ID]; keep {
			continue
		}
		path, err := targetAdapterDestination(options, artifact)
		if err != nil {
			return fail(err)
		}
		if desiredDestinations[path] {
			continue
		}
		if options.TargetAdapterOps != nil && options.TargetAdapterOps.beforeArtifact != nil {
			if err := options.TargetAdapterOps.beforeArtifact(artifact.ID); err != nil {
				return fail(err)
			}
		}
		if err := ensureTargetAdapterSnapshotUnchanged(path, states[path]); err != nil {
			return fail(err)
		}
		operationErr := removeTargetAdapterArtifact(options, artifact)
		if operationErr == nil && options.TargetAdapterOps != nil && options.TargetAdapterOps.afterArtifact != nil {
			operationErr = options.TargetAdapterOps.afterArtifact(artifact.ID)
		}
		if err := recordTargetAdapterMutation(path, states[path], &mutated, mutatedPaths); err != nil {
			if operationErr != nil {
				return fail(fmt.Errorf("%w; inspect target adapter destination after removal: %v", operationErr, err))
			}
			return fail(err)
		}
		if operationErr != nil {
			return fail(operationErr)
		}
	}
	for _, artifact := range desired.Artifacts {
		if artifact.Kind == "instruction" {
			continue
		}
		if options.TargetAdapterOps != nil && options.TargetAdapterOps.beforeArtifact != nil {
			if err := options.TargetAdapterOps.beforeArtifact(artifact.ID); err != nil {
				return fail(err)
			}
		}
		path, err := targetAdapterDestination(options, artifact)
		if err != nil {
			return fail(err)
		}
		if err := ensureTargetAdapterSnapshotUnchanged(path, states[path]); err != nil {
			return fail(err)
		}
		operationErr := publishTargetAdapterArtifact(options, artifact)
		if operationErr == nil && options.TargetAdapterOps != nil && options.TargetAdapterOps.afterArtifact != nil {
			operationErr = options.TargetAdapterOps.afterArtifact(artifact.ID)
		}
		if err := recordTargetAdapterMutation(path, states[path], &mutated, mutatedPaths); err != nil {
			if operationErr != nil {
				return fail(fmt.Errorf("%w; inspect target adapter destination after publication: %v", operationErr, err))
			}
			return fail(err)
		}
		if operationErr != nil {
			return fail(operationErr)
		}
	}
	if err := ensureTargetAdapterSnapshotUnchanged(installedPath, manifestSnapshot); err != nil {
		return fail(err)
	}
	if err := writeTargetAdapterManifest(installedPath, desired); err != nil {
		writeErr := fmt.Errorf("write installed target adapter manifest: %w", err)
		if stateErr := recordTargetAdapterMutation(installedPath, manifestSnapshot, &mutated, mutatedPaths); stateErr != nil {
			writeErr = fmt.Errorf("%w; inspect installed target adapter manifest after publication: %v", writeErr, stateErr)
		}
		return fail(writeErr)
	}
	return nil
}

func ensureTargetAdapterSnapshotUnchanged(path string, expected targetAdapterSnapshot) error {
	current, err := readTargetAdapterSnapshot(path)
	if err != nil {
		return err
	}
	if !sameTargetAdapterSnapshot(current, expected) {
		return fmt.Errorf("target adapter destination %s changed during install", path)
	}
	return nil
}

func recordTargetAdapterMutation(path string, expected targetAdapterSnapshot, mutated *[]targetAdapterSnapshot, mutatedPaths map[string]bool) error {
	current, err := readTargetAdapterSnapshot(path)
	if err != nil {
		if !mutatedPaths[path] {
			*mutated = append(*mutated, expected)
			mutatedPaths[path] = true
		}
		return fmt.Errorf("inspect target adapter destination %s after mutation: %w", path, err)
	}
	if !sameTargetAdapterSnapshot(current, expected) && !mutatedPaths[path] {
		*mutated = append(*mutated, expected)
		mutatedPaths[path] = true
	}
	return nil
}

func rollbackTargetAdapterMutations(cause error, mutated []targetAdapterSnapshot, operations *targetAdapterInstallOperations) error {
	restore := restoreTargetAdapterSnapshot
	if operations != nil && operations.restoreSnapshot != nil {
		restore = operations.restoreSnapshot
	}
	var failures []string
	for index := len(mutated) - 1; index >= 0; index-- {
		snapshot := mutated[index]
		if err := restore(snapshot); err != nil {
			failures = append(failures, fmt.Sprintf("%s: %v; current state: %s; expected state: %s", snapshot.path, err, describeTargetAdapterPathState(snapshot.path), describeTargetAdapterSnapshot(snapshot)))
		}
	}
	if len(failures) > 0 {
		return fmt.Errorf("%w; rollback failed: %s", cause, strings.Join(failures, "; "))
	}
	return cause
}

func describeTargetAdapterPathState(path string) string {
	snapshot, err := readTargetAdapterSnapshot(path)
	if err != nil {
		return "unreadable (" + err.Error() + ")"
	}
	return describeTargetAdapterSnapshot(snapshot)
}

func describeTargetAdapterSnapshot(snapshot targetAdapterSnapshot) string {
	if !snapshot.exists {
		return "absent"
	}
	return fmt.Sprintf("present mode=%#o sha256=%s", snapshot.mode, sha256Bytes(snapshot.body))
}

func targetAdapterArtifactsByID(artifacts []targetAdapterArtifact) map[string]targetAdapterArtifact {
	result := make(map[string]targetAdapterArtifact, len(artifacts))
	for _, artifact := range artifacts {
		result[artifact.ID] = artifact
	}
	return result
}

func targetAdapterDestination(options targetInstallOptions, artifact targetAdapterArtifact) (string, error) {
	root := options.ConfigDir
	if options.Target == "codex" && options.CodexHome != "" {
		root = options.CodexHome
	}
	if options.Target == "amp" && artifact.Kind == "plugin" && options.AmpPluginsDir != "" {
		root = options.AmpPluginsDir
		if artifact.Destination == "plugins/loaf.ts" {
			destination := filepath.Join(root, "loaf.ts")
			if err := validateTargetAdapterDestinationParents(root, destination); err != nil {
				return "", err
			}
			return destination, nil
		}
	}
	if root == "" || !validTargetAdapterPath(artifact.Destination) {
		return "", fmt.Errorf("invalid target adapter destination %q", artifact.Destination)
	}
	destination := filepath.Join(root, filepath.FromSlash(artifact.Destination))
	if err := validateTargetAdapterDestinationParents(root, destination); err != nil {
		return "", err
	}
	return destination, nil
}

func validateTargetAdapterDestinationParents(root string, destination string) error {
	rel, err := filepath.Rel(root, destination)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return fmt.Errorf("target adapter destination %s escapes %s", destination, root)
	}
	current := root
	parts := strings.Split(filepath.Dir(rel), string(filepath.Separator))
	for _, part := range parts {
		if part == "" || part == "." {
			continue
		}
		current = filepath.Join(current, part)
		info, err := os.Lstat(current)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return err
		}
		if info.Mode()&fs.ModeSymlink != 0 || !info.IsDir() {
			return fmt.Errorf("target adapter destination parent %s is not a real directory", current)
		}
	}
	return nil
}

func verifyTargetAdapterSource(options targetInstallOptions, artifact targetAdapterArtifact) error {
	path := filepath.Join(options.DistDir, filepath.FromSlash(artifact.SourcePath))
	info, err := os.Lstat(path)
	if err != nil {
		return fmt.Errorf("read target adapter source %q: %w", artifact.SourcePath, err)
	}
	if !info.Mode().IsRegular() || info.Mode()&fs.ModeSymlink != 0 {
		return fmt.Errorf("target adapter source %q must be a regular file", artifact.SourcePath)
	}
	body, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	digest := sha256Bytes(body)
	if artifact.Kind == "hook-projection" {
		digest, err = targetHookProjectionDigest(options.Target, body, false)
		if err != nil {
			return err
		}
	}
	if digest != artifact.SHA256 {
		return fmt.Errorf("target adapter source %q does not match its manifest digest", artifact.SourcePath)
	}
	if artifact.Mode != nil && uint32(info.Mode().Perm()) != *artifact.Mode {
		return fmt.Errorf("target adapter source %q mode %#o does not match manifest mode %#o", artifact.SourcePath, info.Mode().Perm(), *artifact.Mode)
	}
	return nil
}

func readTargetAdapterSnapshot(path string) (targetAdapterSnapshot, error) {
	info, err := os.Lstat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return targetAdapterSnapshot{path: path}, nil
		}
		return targetAdapterSnapshot{}, err
	}
	if !info.Mode().IsRegular() || info.Mode()&fs.ModeSymlink != 0 {
		return targetAdapterSnapshot{}, fmt.Errorf("target adapter destination %s must be a regular file", path)
	}
	body, err := os.ReadFile(path)
	if err != nil {
		return targetAdapterSnapshot{}, err
	}
	return targetAdapterSnapshot{path: path, exists: true, body: body, mode: info.Mode().Perm()}, nil
}

func sameTargetAdapterSnapshot(a targetAdapterSnapshot, b targetAdapterSnapshot) bool {
	return a.exists == b.exists && bytes.Equal(a.body, b.body) && a.mode == b.mode
}

func restoreTargetAdapterSnapshot(snapshot targetAdapterSnapshot) error {
	if !snapshot.exists {
		if err := os.Remove(snapshot.path); err != nil && !os.IsNotExist(err) {
			return err
		}
		return nil
	}
	return writeFileAtomically(snapshot.path, snapshot.body, snapshot.mode)
}

func targetAdapterInstalledDigest(target string, artifact targetAdapterArtifact, body []byte) (string, error) {
	if artifact.Kind == "hook-projection" {
		return targetHookProjectionDigest(target, body, true)
	}
	return sha256Bytes(body), nil
}

func targetAdapterSnapshotMatchesArtifact(target string, artifact targetAdapterArtifact, snapshot targetAdapterSnapshot) (bool, error) {
	if !snapshot.exists {
		return false, nil
	}
	digest, err := targetAdapterInstalledDigest(target, artifact, snapshot.body)
	if err != nil {
		return false, err
	}
	if digest != artifact.SHA256 {
		return false, nil
	}
	if artifact.Kind == "hook-file" || artifact.Kind == "plugin" {
		return artifact.Mode != nil && uint32(snapshot.mode.Perm()) == *artifact.Mode, nil
	}
	return artifact.Mode == nil, nil
}

func targetAdapterLegacyOwnership(target string, artifact targetAdapterArtifact, body []byte) bool {
	if artifact.Kind == "hook-projection" {
		switch target {
		case "cursor":
			var hooks codexHooksFile
			if json.Unmarshal(body, &hooks) != nil {
				return false
			}
			for _, entries := range hooks.Hooks {
				for _, entry := range entries {
					if isLoafInstallHook(entry) {
						return true
					}
				}
			}
		case "codex":
			return !targetHookProjectionIsEmpty(target, body)
		}
	}
	if artifact.Kind == "plugin" {
		text := string(body)
		return strings.Contains(text, "Auto-generated by loaf build system") &&
			((target == "amp" && strings.Contains(text, "Amp Plugin - Agent Skills Hooks")) ||
				(target == "opencode" && strings.Contains(text, "OpenCode Plugin - Agent Skills Hooks")))
	}
	return false
}

func targetHookProjectionIsEmpty(target string, body []byte) bool {
	switch target {
	case "cursor":
		var hooks codexHooksFile
		if json.Unmarshal(body, &hooks) != nil {
			return false
		}
		for _, entries := range hooks.Hooks {
			for _, entry := range entries {
				if isLoafInstallHook(entry) {
					return false
				}
			}
		}
		return true
	case "codex":
		hooks, err := decodeCodexHooksBodyStrict(body)
		if err != nil {
			return false
		}
		for _, entries := range hooks.Hooks {
			for _, rawEntry := range entries {
				entry, err := decodeCodexHookObject(rawEntry)
				if err != nil {
					return false
				}
				owned, conflict := codexHookOwnership(entry)
				if owned || conflict {
					return false
				}
			}
		}
		return true
	default:
		return false
	}
}

func publishTargetAdapterArtifact(options targetInstallOptions, artifact targetAdapterArtifact) error {
	source := filepath.Join(options.DistDir, filepath.FromSlash(artifact.SourcePath))
	destination, err := targetAdapterDestination(options, artifact)
	if err != nil {
		return err
	}
	if artifact.Kind == "hook-projection" {
		switch options.Target {
		case "cursor":
			err = mergeHookFiles(destination, source)
		case "codex":
			err = mergeCodexHookFiles(destination, source, options.ProjectRoot, options.CodexRuleOperations)
		default:
			err = fmt.Errorf("target %q does not support hook projection installation", options.Target)
		}
	} else {
		body, readErr := os.ReadFile(source)
		if readErr != nil {
			return readErr
		}
		if artifact.Mode == nil {
			return fmt.Errorf("target adapter artifact %q has no bound mode", artifact.ID)
		}
		err = writeFileAtomically(destination, body, fs.FileMode(*artifact.Mode))
	}
	if err != nil {
		return fmt.Errorf("publish target adapter artifact %q: %w", artifact.ID, err)
	}
	snapshot, err := readTargetAdapterSnapshot(destination)
	if err != nil {
		return err
	}
	matches, err := targetAdapterSnapshotMatchesArtifact(options.Target, artifact, snapshot)
	if err != nil {
		return err
	}
	if !matches {
		return fmt.Errorf("published target adapter artifact %q failed content or mode verification", artifact.ID)
	}
	return nil
}

func removeTargetAdapterArtifact(options targetInstallOptions, artifact targetAdapterArtifact) error {
	destination, err := targetAdapterDestination(options, artifact)
	if err != nil {
		return err
	}
	if artifact.Kind != "hook-projection" {
		if err := os.Remove(destination); err != nil && !os.IsNotExist(err) {
			return err
		}
		return nil
	}
	snapshot, err := readTargetAdapterSnapshot(destination)
	if err != nil {
		return err
	}
	if !snapshot.exists {
		return nil
	}
	switch options.Target {
	case "cursor":
		hooks, err := loadCodexHooksFile(destination)
		if err != nil {
			return err
		}
		for event, entries := range hooks.Hooks {
			kept := entries[:0]
			for _, entry := range entries {
				if !isLoafInstallHook(entry) {
					kept = append(kept, entry)
				}
			}
			if len(kept) == 0 {
				delete(hooks.Hooks, event)
			} else {
				hooks.Hooks[event] = kept
			}
		}
		body, err := json.MarshalIndent(hooks, "", "  ")
		if err != nil {
			return err
		}
		return writeFileAtomically(destination, append(body, '\n'), snapshot.mode)
	case "codex":
		hooks, err := loadCodexHooksRawFileStrict(destination)
		if err != nil {
			return err
		}
		for event, entries := range hooks.Hooks {
			kept := entries[:0]
			for _, rawEntry := range entries {
				entry, err := decodeCodexHookObject(rawEntry)
				if err != nil {
					return err
				}
				owned, conflict := codexHookOwnership(entry)
				if conflict {
					return fmt.Errorf("modified Loaf matcher group")
				}
				if !owned {
					kept = append(kept, rawEntry)
				}
			}
			hooks.Hooks[event] = kept
		}
		body, err := json.MarshalIndent(hooks, "", "  ")
		if err != nil {
			return err
		}
		return writeFileAtomically(destination, append(body, '\n'), snapshot.mode)
	default:
		return fmt.Errorf("target %q does not support hook projection removal", options.Target)
	}
}
