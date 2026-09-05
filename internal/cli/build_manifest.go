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

// obsoleteHookProjectionKind is the artifact kind releases up to and including
// 0.2.20 gave a target's shared hooks file. Entry-level reconciliation retired
// it: that file is not an artifact Loaf owns, so no digest of it means anything
// and no divergence from one can refuse anything. Nothing writes the kind any
// more, and the name survives in exactly one place — the reader below, which
// tolerates the rows an older release left in an installed manifest and drops
// them so the next write is rid of them.
const obsoleteHookProjectionKind = "hook-projection"

const (
	ampModesPluginArtifactID        = "plugin:.amp/plugins/loaf-modes.ts"
	ampModesPluginSourcePath        = ".amp/plugins/loaf-modes.ts"
	ampModesPluginDestination       = "plugins/loaf-modes.ts"
	ampModesPluginPredecessorSHA256 = "27ff4c82dbb0cd21b6f9ff694e20017fe62653521d23d5b586ba8f3457b64c5f"
)

type targetAdapterManifest struct {
	Version                   int                     `json:"version"`
	Target                    string                  `json:"target"`
	PackageVersion            string                  `json:"package_version"`
	CapabilityContractVersion int                     `json:"capability_contract_version"`
	Adapters                  []string                `json:"adapters"`
	Artifacts                 []targetAdapterArtifact `json:"artifacts"`

	// carriedObsoleteHookRow records that the manifest as read from disk still
	// held one of the retired rows. It is deliberately unexported and unmarshal-
	// only: prior-install detection is the single thing that still cares, and it
	// asks a question about the past that must not survive into what is written.
	carriedObsoleteHookRow bool
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
	// A target's shared hooks file is absent from every one of these lists.
	// Cursor's `hooks.json` and Codex's `.codex/hooks.json` are converged one
	// entry at a time against the built catalog, so there is nothing here for a
	// whole-file artifact to own, publish, remove, or hold a digest of. Claude
	// Code's hooks live inside the plugin bundle Loaf writes outright, which is
	// why they stay ordinary hook files.
	var paths []string
	switch target {
	case "claude-code":
		paths = []string{"hooks"}
	case "opencode":
		paths = []string{"plugins"}
	case "cursor":
		paths = []string{"hooks"}
	case "codex":
		paths = nil
	case "amp":
		paths = []string{".amp/plugins"}
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
				relSlash := filepath.ToSlash(rel)
				if target == "amp" && !managedAmpPluginSource(relSlash) {
					return nil
				}
				return appendTargetAdapterArtifact(&artifacts, seen, target, outputDir, relSlash)
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

func managedAmpPluginSource(sourcePath string) bool {
	switch sourcePath {
	case ".amp/plugins/loaf.ts", ".amp/plugins/loaf-modes.ts":
		return true
	default:
		return false
	}
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
		destination = "plugins/" + filepath.Base(filepath.FromSlash(sourcePath))
	case "opencode":
		if sourcePath == "plugins/hooks.ts" {
			kind = "plugin"
		}
	}
	info, err := os.Lstat(filepath.Join(outputDir, filepath.FromSlash(sourcePath)))
	if err != nil {
		return err
	}
	mode := uint32(info.Mode().Perm())
	*artifacts = append(*artifacts, targetAdapterArtifact{
		ID:          kind + ":" + sourcePath,
		Kind:        kind,
		SourcePath:  sourcePath,
		Destination: destination,
		SHA256:      sha256Bytes(body),
		Mode:        &mode,
	})
	return nil
}

func readTargetAdapterManifest(path string) (targetAdapterManifest, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return targetAdapterManifest{}, err
	}
	if !info.Mode().IsRegular() || info.Mode()&fs.ModeSymlink != 0 {
		return targetAdapterManifest{}, fmt.Errorf("target adapter manifest %s must be a regular file", path)
	}
	raw, err := readRegularFile(path, projectFileReadLimit)
	if err != nil {
		return targetAdapterManifest{}, err
	}
	// The retired rows come out before any strict rule is applied, because every
	// strict rule applies to the whole document: the duplicate-key walk descends
	// into each row, and DisallowUnknownFields judges each row's fields. A row
	// this version has no semantics for must not be able to fail a read — that
	// failure would abort the very upgrade that was about to absorb and drop it,
	// which is the file-level refusal this Change removed wearing a new hat.
	body, carriedObsoleteHookRow := stripObsoleteHookProjectionRows(raw)
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
	manifest.carriedObsoleteHookRow = carriedObsoleteHookRow
	if err := validateTargetAdapterManifest(manifest); err != nil {
		return targetAdapterManifest{}, err
	}
	return manifest, nil
}

// stripObsoleteHookProjectionRows removes the retired rows from the manifest
// bytes, and is the only code that ever looks at one. It looks at exactly one
// field — `kind` — so an unknown field a later release added, a field whose type
// changed, and a duplicate key inside the row are all invisible rather than
// fatal.
//
// Strictness for every live row is untouched. Kept rows are written back as the
// bytes they were read as, top-level keys keep their order and their repeats,
// and the strict pass then runs over the result, so a duplicate key or unknown
// field anywhere Loaf still has semantics for fails exactly as before. A
// document with nothing to drop is returned unchanged, byte for byte, and one
// this cannot parse is handed back for the strict pass to report.
func stripObsoleteHookProjectionRows(body []byte) ([]byte, bool) {
	fields, ok := decodeJSONObjectFields(body)
	if !ok {
		return body, false
	}
	dropped := false
	for index, field := range fields {
		if field.name != "artifacts" {
			continue
		}
		var rows []json.RawMessage
		if err := json.Unmarshal(field.value, &rows); err != nil {
			continue
		}
		kept := make([]json.RawMessage, 0, len(rows))
		obsolete := false
		for _, row := range rows {
			if isObsoleteHookProjectionRow(row) {
				obsolete = true
				continue
			}
			kept = append(kept, row)
		}
		if !obsolete {
			continue
		}
		encoded, err := json.Marshal(kept)
		if err != nil {
			return body, false
		}
		fields[index].value = encoded
		dropped = true
	}
	if !dropped {
		return body, false
	}
	rebuilt, err := encodeJSONObjectFields(fields)
	if err != nil {
		return body, false
	}
	return rebuilt, true
}

// isObsoleteHookProjectionRow reads the one field that identifies a row, and
// requires that field to be unambiguous: exactly one `kind`, holding a JSON
// string. A row with no `kind`, with several, or with a non-string one is not
// identified — it stays where it is and the strict pass judges it.
//
// The repeated case is the one worth spelling out. Deciding by last-wins on
// `{"kind":"hook-file","kind":"hook-projection"}` would do two wrong things at
// once: launder a duplicate-key defect past the walk that exists to catch it,
// and discard a row whose first `kind` says it is live. Leaving it in place
// makes the duplicate fail the duplicate-key walk, which is what should happen.
//
// Repeats in any other field are a different question. There the row's identity
// is not in doubt, so it drops whole and whatever else it carried goes with it.
func isObsoleteHookProjectionRow(row json.RawMessage) bool {
	fields, ok := decodeJSONObjectFields(row)
	if !ok {
		return false
	}
	var kind json.RawMessage
	declared := 0
	for _, field := range fields {
		if field.name != "kind" {
			continue
		}
		declared++
		kind = field.value
	}
	if declared != 1 {
		return false
	}
	var value string
	if err := json.Unmarshal(kind, &value); err != nil {
		return false
	}
	return value == obsoleteHookProjectionKind
}

// jsonObjectField is one written key-value pair. Repeats are pairs too: this
// decodes for a rewrite that the duplicate-key walk still has to be able to
// reject, so collapsing them would launder a defect past it.
type jsonObjectField struct {
	name  string
	value json.RawMessage
}

func decodeJSONObjectFields(body []byte) ([]jsonObjectField, bool) {
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.UseNumber()
	token, err := decoder.Token()
	if err != nil {
		return nil, false
	}
	if delimiter, ok := token.(json.Delim); !ok || delimiter != '{' {
		return nil, false
	}
	var fields []jsonObjectField
	for decoder.More() {
		key, err := decoder.Token()
		if err != nil {
			return nil, false
		}
		name, ok := key.(string)
		if !ok {
			return nil, false
		}
		var value json.RawMessage
		if err := decoder.Decode(&value); err != nil {
			return nil, false
		}
		fields = append(fields, jsonObjectField{name: name, value: value})
	}
	if _, err := decoder.Token(); err != nil {
		return nil, false
	}
	// Everything after the closing brace must be whitespace JSON itself allows.
	// decoder.More() answers a different question — it looks for the start of
	// another value — so a stray closing delimiter walks straight past it, and a
	// document rebuilt without that delimiter would reach the strict pass
	// already repaired. Anything else trailing means this cannot strip the
	// document, and the original bytes go to the strict pass to be refused.
	if !isJSONWhitespace(body[decoder.InputOffset():]) {
		return nil, false
	}
	return fields, true
}

// isJSONWhitespace reports whether every byte is one JSON's grammar permits
// between tokens. The set is exactly four, and it is narrower than the one
// strings.TrimSpace trims: Unicode calls a vertical tab, a form feed, and a
// non-breaking space whitespace, while JSON calls all three a syntax error. A
// document ending in one of those is invalid and the strict pass says so — but
// only if it sees the bytes that were written, which it would not if this
// treated the suffix as harmless and the rebuild quietly erased it.
func isJSONWhitespace(remainder []byte) bool {
	for _, character := range remainder {
		switch character {
		case ' ', '\t', '\n', '\r':
		default:
			return false
		}
	}
	return true
}

func encodeJSONObjectFields(fields []jsonObjectField) ([]byte, error) {
	var out bytes.Buffer
	out.WriteByte('{')
	for index, field := range fields {
		if index > 0 {
			out.WriteByte(',')
		}
		name, err := json.Marshal(field.name)
		if err != nil {
			return nil, err
		}
		out.Write(name)
		out.WriteByte(':')
		if err := json.Compact(&out, field.value); err != nil {
			return nil, err
		}
	}
	out.WriteByte('}')
	return out.Bytes(), nil
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
		if artifact.Kind != "instruction" && artifact.Kind != "hook-file" && artifact.Kind != "plugin" {
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
		if artifact.Kind == "instruction" {
			if artifact.Mode != nil {
				return fmt.Errorf("target adapter artifact kind %q must not declare a mode", artifact.Kind)
			}
		} else if artifact.Mode == nil || *artifact.Mode > 0o777 {
			return fmt.Errorf("invalid or missing target adapter artifact mode for %q", artifact.ID)
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
		if skipTargetAdapterArtifact(artifact) {
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
		matchesInstalled := targetAdapterSnapshotMatchesArtifact(artifact, snapshot)
		matchesDesired := false
		if current, ok := desiredByID[artifact.ID]; ok {
			matchesDesired = targetAdapterSnapshotMatchesArtifact(current, snapshot)
		}
		if !matchesInstalled && !matchesDesired {
			return fmt.Errorf("managed target artifact %q was modified; refusing to overwrite or remove", artifact.ID)
		}
	}
	for _, artifact := range desired.Artifacts {
		if skipTargetAdapterArtifact(artifact) {
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
		if targetAdapterSnapshotMatchesArtifact(artifact, states[path]) || targetAdapterLegacyOwnership(options.Target, artifact, states[path].body) || ampModesPluginExactPredecessor(options.Target, artifact, states[path].body) {
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
		if skipTargetAdapterArtifact(artifact) {
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
		if skipTargetAdapterArtifact(artifact) {
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

// skipTargetAdapterArtifact names the one artifact the whole-file machinery
// does not handle: managed instructions live inside a project file rather than
// at a destination of their own. A target's shared hooks file is not on this
// list because it is not on any manifest — the reconciler owns it, and a
// whole-file row for it is exactly the file-level verdict this replaces.
func skipTargetAdapterArtifact(artifact targetAdapterArtifact) bool {
	return artifact.Kind == "instruction"
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
		if strings.HasPrefix(artifact.Destination, "plugins/") {
			name := strings.TrimPrefix(artifact.Destination, "plugins/")
			if name != "" && !strings.Contains(name, "/") && validTargetAdapterPath(name) {
				destination := filepath.Join(root, filepath.FromSlash(name))
				if err := validateTargetAdapterDestinationParents(root, destination); err != nil {
					return "", err
				}
				return destination, nil
			}
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
	if digest := sha256Bytes(body); digest != artifact.SHA256 {
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
	body, err := readRegularFile(path, projectFileReadLimit)
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

func targetAdapterSnapshotMatchesArtifact(artifact targetAdapterArtifact, snapshot targetAdapterSnapshot) bool {
	if !snapshot.exists || sha256Bytes(snapshot.body) != artifact.SHA256 {
		return false
	}
	if artifact.Kind == "instruction" {
		return artifact.Mode == nil
	}
	return artifact.Mode != nil && uint32(snapshot.mode.Perm()) == *artifact.Mode
}

func targetAdapterLegacyOwnership(target string, artifact targetAdapterArtifact, body []byte) bool {
	if artifact.Kind == "plugin" {
		text := string(body)
		return strings.Contains(text, "Auto-generated by loaf build system") &&
			((target == "amp" && strings.Contains(text, "Amp Plugin - Agent Skills Hooks")) ||
				(target == "opencode" && strings.Contains(text, "OpenCode Plugin - Agent Skills Hooks")))
	}
	return false
}

// ampModesPluginExactPredecessor is a one-time closed digest migration for the
// recovered private Amp modes plugin that predates packaged ownership. Loaf
// may adopt only an unrecorded destination when the target is amp, the artifact
// identity/source/destination are exactly the Loaf modes plugin, and the body
// SHA-256 is the known predecessor. Any other byte, identity, or target stays
// foreign. This does not broaden the generated-header heuristic.
func ampModesPluginExactPredecessor(target string, artifact targetAdapterArtifact, body []byte) bool {
	return target == "amp" &&
		artifact.Kind == "plugin" &&
		artifact.ID == ampModesPluginArtifactID &&
		artifact.SourcePath == ampModesPluginSourcePath &&
		artifact.Destination == ampModesPluginDestination &&
		sha256Bytes(body) == ampModesPluginPredecessorSHA256
}

func publishTargetAdapterArtifact(options targetInstallOptions, artifact targetAdapterArtifact) error {
	source := filepath.Join(options.DistDir, filepath.FromSlash(artifact.SourcePath))
	destination, err := targetAdapterDestination(options, artifact)
	if err != nil {
		return err
	}
	body, err := os.ReadFile(source)
	if err != nil {
		return err
	}
	if artifact.Mode == nil {
		return fmt.Errorf("target adapter artifact %q has no bound mode", artifact.ID)
	}
	if err := writeFileAtomically(destination, body, fs.FileMode(*artifact.Mode)); err != nil {
		return fmt.Errorf("publish target adapter artifact %q: %w", artifact.ID, err)
	}
	snapshot, err := readTargetAdapterSnapshot(destination)
	if err != nil {
		return err
	}
	if !targetAdapterSnapshotMatchesArtifact(artifact, snapshot) {
		return fmt.Errorf("published target adapter artifact %q failed content or mode verification", artifact.ID)
	}
	return nil
}

func removeTargetAdapterArtifact(options targetInstallOptions, artifact targetAdapterArtifact) error {
	destination, err := targetAdapterDestination(options, artifact)
	if err != nil {
		return err
	}
	if err := os.Remove(destination); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}
