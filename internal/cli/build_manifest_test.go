package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestTargetCapabilityAdaptersAreSortedUniqueWithoutStatusPromotion(t *testing.T) {
	body, err := os.ReadFile(testTargetCapabilityEvidencePath(t))
	if err != nil {
		t.Fatal(err)
	}
	contract, err := DecodeTargetCapabilityEvidence(body)
	if err != nil {
		t.Fatal(err)
	}
	if got := targetCapabilityAdapters(contract, "cursor"); len(got) != 1 || got[0] != "cursor-session-start-v1" {
		t.Fatalf("cursor adapters = %v, want one deduplicated candidate adapter", got)
	}
	if got := targetCapabilityAdapters(contract, "pi"); len(got) != 0 {
		t.Fatalf("deferred Pi adapters = %v, want none", got)
	}
}

func TestTargetAdapterManifestIsDeterministic(t *testing.T) {
	root := realpath(t, t.TempDir())
	path := filepath.Join(root, targetBuildManifestFile)
	hookMode := uint32(0o755)
	manifest := targetAdapterManifest{
		Version:                   1,
		Target:                    "cursor",
		PackageVersion:            "9.8.7-test.1",
		CapabilityContractVersion: 3,
		Adapters:                  []string{"z-adapter-v1", "a-adapter-v1"},
		Artifacts: []targetAdapterArtifact{
			{ID: "managed-instructions", Kind: "instruction", Destination: "project-instructions", SHA256: fencedContentFingerprint(generateFencedContent())},
			{ID: "hook-file:hooks/z.sh", Kind: "hook-file", SourcePath: "hooks/z.sh", Destination: "hooks/z.sh", SHA256: strings.Repeat("b", 64), Mode: &hookMode},
		},
	}
	if err := writeTargetAdapterManifest(path, manifest); err != nil {
		t.Fatal(err)
	}
	first := string(readFileBytes(t, path))
	if err := writeTargetAdapterManifest(path, manifest); err != nil {
		t.Fatal(err)
	}
	if second := string(readFileBytes(t, path)); second != first {
		t.Fatalf("manifest changed across deterministic writes:\n%s\n---\n%s", first, second)
	}
	parsed, err := readTargetAdapterManifest(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(parsed.Adapters, ",") != "a-adapter-v1,z-adapter-v1" {
		t.Fatalf("sorted adapters = %v", parsed.Adapters)
	}
	if got := []string{parsed.Artifacts[0].ID, parsed.Artifacts[1].ID}; got[0] != "hook-file:hooks/z.sh" || got[1] != "managed-instructions" {
		t.Fatalf("sorted artifacts = %v, want hook-file then managed-instructions", got)
	}
}

func TestReadTargetAdapterManifestRejectsUnsafeAndNonStrictShapes(t *testing.T) {
	valid := `{"version":1,"target":"amp","package_version":"1.0.0","capability_contract_version":3,"adapters":["amp-plugin-v1"],"artifacts":[{"id":"managed-instructions","kind":"instruction","destination":"project-instructions","sha256":"` + strings.Repeat("a", 64) + `"},{"id":"plugin:.amp/plugins/loaf.ts","kind":"plugin","source_path":".amp/plugins/loaf.ts","destination":"plugins/loaf.ts","sha256":"` + strings.Repeat("b", 64) + `","mode":420}]}`
	for name, body := range map[string]string{
		"unknown field":         strings.Replace(valid, `"target":"amp"`, `"target":"amp","unknown":true`, 1),
		"duplicate key":         strings.Replace(valid, `"target":"amp"`, `"target":"amp","target":"amp"`, 1),
		"trailing value":        valid + `{}`,
		"traversal":             strings.Replace(valid, `"plugins/loaf.ts"`, `"../plugins/loaf.ts"`, 1),
		"absolute":              strings.Replace(valid, `"plugins/loaf.ts"`, `"/plugins/loaf.ts"`, 1),
		"backslash":             strings.Replace(valid, `"plugins/loaf.ts"`, `"plugins\\loaf.ts"`, 1),
		"uppercase digest":      strings.Replace(valid, strings.Repeat("b", 64), strings.Repeat("B", 64), 1),
		"duplicate destination": strings.Replace(valid, `"project-instructions"`, `"plugins/loaf.ts"`, 1),
		"unknown kind":          strings.Replace(valid, `"kind":"plugin"`, `"kind":"binary"`, 1),
	} {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "manifest.json")
			writeInstallFile(t, path, body)
			if _, err := readTargetAdapterManifest(path); err == nil {
				t.Fatalf("readTargetAdapterManifest(%s) error = nil", name)
			}
		})
	}

	real := filepath.Join(t.TempDir(), "real.json")
	writeInstallFile(t, real, valid)
	link := filepath.Join(filepath.Dir(real), "link.json")
	if err := os.Symlink(real, link); err != nil {
		t.Fatal(err)
	}
	if _, err := readTargetAdapterManifest(link); err == nil {
		t.Fatal("symlinked target adapter manifest was accepted")
	}
}

func TestReadTargetAdapterManifestRequiresConcreteModesAndForbidsProjectionModes(t *testing.T) {
	digest := strings.Repeat("a", 64)
	valid := `{"version":1,"target":"opencode","package_version":"1.0.0","capability_contract_version":3,"adapters":["opencode-plugin-v1"],"artifacts":[{"id":"managed-instructions","kind":"instruction","destination":"project-instructions","sha256":"` + digest + `"},{"id":"plugin:plugins/hooks.ts","kind":"plugin","source_path":"plugins/hooks.ts","destination":"plugins/hooks.ts","sha256":"` + digest + `","mode":493}]}`
	path := filepath.Join(t.TempDir(), "manifest.json")
	writeInstallFile(t, path, valid)
	if _, err := readTargetAdapterManifest(path); err != nil {
		t.Fatalf("valid concrete mode rejected: %v", err)
	}
	for name, body := range map[string]string{
		"missing concrete mode": strings.Replace(valid, `,"mode":493`, "", 1),
		"mode out of range":     strings.Replace(valid, `"mode":493`, `"mode":512`, 1),
		"instruction mode":      strings.Replace(valid, `"destination":"project-instructions"`, `"destination":"project-instructions","mode":420`, 1),
	} {
		t.Run(name, func(t *testing.T) {
			writeInstallFile(t, path, body)
			if _, err := readTargetAdapterManifest(path); err == nil {
				t.Fatalf("readTargetAdapterManifest(%s) error = nil", name)
			}
		})
	}
}

// The retired whole-file hooks row: an installed manifest an older release
// wrote still reads, the row is gone from what was read, and the write that
// follows carries no trace of it. Nothing about the row is validated on the way
// past — a kind this version has no semantics for must not get a second chance
// to matter.
func TestReadTargetAdapterManifestDropsTheRetiredHookProjectionRow(t *testing.T) {
	digest := strings.Repeat("a", 64)
	legacy := `{"version":1,"target":"cursor","package_version":"0.2.20","capability_contract_version":3,"adapters":["cursor-session-start-v1"],"artifacts":[` +
		`{"id":"hook-file:hooks/x.sh","kind":"hook-file","source_path":"hooks/x.sh","destination":"hooks/x.sh","sha256":"` + digest + `","mode":493},` +
		`{"id":"hook-projection:hooks.json","kind":"hook-projection","source_path":"hooks.json","destination":"hooks.json","sha256":"` + digest + `"},` +
		`{"id":"managed-instructions","kind":"instruction","destination":"project-instructions","sha256":"` + digest + `"}]}`
	dir := realpath(t, t.TempDir())
	path := filepath.Join(dir, "manifest.json")
	writeInstallFile(t, path, legacy)

	manifest, err := readTargetAdapterManifest(path)
	if err != nil {
		t.Fatalf("readTargetAdapterManifest(legacy) error = %v", err)
	}
	if !manifest.carriedObsoleteHookRow {
		t.Fatal("carriedObsoleteHookRow = false; prior-install detection reads that row and nothing else does")
	}
	for _, artifact := range manifest.Artifacts {
		if artifact.Kind == obsoleteHookProjectionKind {
			t.Fatalf("artifacts = %#v, want the retired row dropped on read", manifest.Artifacts)
		}
	}
	if len(manifest.Artifacts) != 2 {
		t.Fatalf("artifacts = %#v, want the surviving hook-file and instruction rows", manifest.Artifacts)
	}

	rewritten := filepath.Join(dir, "rewritten.json")
	if err := writeTargetAdapterManifest(rewritten, manifest); err != nil {
		t.Fatalf("writeTargetAdapterManifest error = %v", err)
	}
	if body := string(readFileBytes(t, rewritten)); strings.Contains(body, obsoleteHookProjectionKind) {
		t.Fatalf("rewritten manifest = %s, want the retired row absent after the next write", body)
	}

	// Nothing writes the kind any more, so a manifest that names it is only ever
	// something to drop — never something to publish back.
	fresh := filepath.Join(dir, "fresh.json")
	if err := writeTargetAdapterManifest(fresh, targetAdapterManifest{
		Version: 1, Target: "cursor", PackageVersion: "9.9.9", CapabilityContractVersion: 3,
		Adapters:  []string{"cursor-session-start-v1"},
		Artifacts: []targetAdapterArtifact{{ID: "hook-projection:hooks.json", Kind: obsoleteHookProjectionKind, SourcePath: "hooks.json", Destination: "hooks.json", SHA256: digest}},
	}); err == nil {
		t.Fatal("writeTargetAdapterManifest accepted the retired kind")
	}
}

// Tolerance has to survive the row being wrong, not merely being retired. The
// strict rules apply to the whole document, so a retired row carrying a field a
// later release added, a field whose type changed, or a repeated key would
// abort the read — and aborting the read aborts the upgrade that was about to
// absorb and drop it. Each defect is then moved into a live row to prove
// strictness was narrowed to the retired kind and nowhere else.
func TestReadTargetAdapterManifestToleratesDefectsInsideTheRetiredHookRow(t *testing.T) {
	digest := strings.Repeat("a", 64)
	manifest := func(hookRow string, liveRow string) string {
		return `{"version":1,"target":"cursor","package_version":"0.2.20","capability_contract_version":3,` +
			`"adapters":["cursor-session-start-v1"],"artifacts":[` + hookRow + `,` + liveRow + `,` +
			`{"id":"managed-instructions","kind":"instruction","destination":"project-instructions","sha256":"` + digest + `"}]}`
	}
	liveRow := `{"id":"hook-file:hooks/x.sh","kind":"hook-file","source_path":"hooks/x.sh","destination":"hooks/x.sh","sha256":"` + digest + `","mode":493}`
	for name, defect := range map[string]struct{ hook, live string }{
		"unknown field": {
			hook: `{"id":"hook-projection:hooks.json","kind":"hook-projection","source_path":"hooks.json","destination":"hooks.json","sha256":"` + digest + `","projection_generation":7}`,
			live: `{"id":"hook-file:hooks/x.sh","kind":"hook-file","source_path":"hooks/x.sh","destination":"hooks/x.sh","sha256":"` + digest + `","mode":493,"projection_generation":7}`,
		},
		"wrong-typed field": {
			hook: `{"id":"hook-projection:hooks.json","kind":"hook-projection","source_path":"hooks.json","destination":"hooks.json","sha256":12345}`,
			live: `{"id":"hook-file:hooks/x.sh","kind":"hook-file","source_path":"hooks/x.sh","destination":"hooks/x.sh","sha256":12345,"mode":493}`,
		},
		"duplicate key inside the row": {
			hook: `{"id":"hook-projection:hooks.json","kind":"hook-projection","source_path":"hooks.json","destination":"hooks.json","sha256":"` + digest + `","sha256":"` + digest + `"}`,
			live: `{"id":"hook-file:hooks/x.sh","kind":"hook-file","source_path":"hooks/x.sh","destination":"hooks/x.sh","sha256":"` + digest + `","sha256":"` + digest + `","mode":493}`,
		},
	} {
		t.Run(name, func(t *testing.T) {
			dir := realpath(t, t.TempDir())
			path := filepath.Join(dir, "manifest.json")
			writeInstallFile(t, path, manifest(defect.hook, liveRow))

			read, err := readTargetAdapterManifest(path)
			if err != nil {
				t.Fatalf("readTargetAdapterManifest(%s in the retired row) error = %v, want it read tolerantly", name, err)
			}
			if !read.carriedObsoleteHookRow {
				t.Fatal("carriedObsoleteHookRow = false; prior-install detection must still see the row that was dropped")
			}
			if len(read.Artifacts) != 2 {
				t.Fatalf("artifacts = %#v, want the retired row dropped and both live rows kept", read.Artifacts)
			}
			rewritten := filepath.Join(dir, "rewritten.json")
			if err := writeTargetAdapterManifest(rewritten, read); err != nil {
				t.Fatalf("writeTargetAdapterManifest error = %v", err)
			}
			if body := string(readFileBytes(t, rewritten)); strings.Contains(body, obsoleteHookProjectionKind) {
				t.Fatalf("rewritten manifest = %s, want the retired row absent after the next write", body)
			}

			// The same defect in a row this version does have semantics for is
			// still a refusal: tolerance is scoped to the retired kind.
			strict := filepath.Join(dir, "strict.json")
			writeInstallFile(t, strict, manifest(defect.hook, defect.live))
			if _, err := readTargetAdapterManifest(strict); err == nil {
				t.Fatalf("readTargetAdapterManifest(%s in a live row) error = nil, want the live row still judged strictly", name)
			}
		})
	}
}

// Two documents that would let a defect through the strip rather than be caught
// by it. Both are refusals, and in both the strip has to leave the bytes exactly
// as it found them — a repair applied on the way past is how a document reaches
// the strict pass already made to look valid.
func TestStripObsoleteHookProjectionRowsRefusesToIdentifyAmbiguousDocuments(t *testing.T) {
	digest := strings.Repeat("a", 64)
	live := `{"id":"hook-file:hooks/x.sh","kind":"hook-file","source_path":"hooks/x.sh","destination":"hooks/x.sh","sha256":"` + digest + `","mode":493}`
	instruction := `{"id":"managed-instructions","kind":"instruction","destination":"project-instructions","sha256":"` + digest + `"}`
	manifest := func(rows string, trailing string) string {
		return `{"version":1,"target":"cursor","package_version":"0.2.20","capability_contract_version":3,` +
			`"adapters":["cursor-session-start-v1"],"artifacts":[` + rows + `]}` + trailing
	}
	// A row claiming both kinds: last-wins identification would drop it, which
	// launders the duplicate key and discards a row whose first kind is live.
	duplicateKind := `{"id":"hook-projection:hooks.json","kind":"hook-file","kind":"hook-projection","source_path":"hooks.json","destination":"hooks.json","sha256":"` + digest + `","mode":493}`
	// A retired row that would strip cleanly, behind a stray closing delimiter
	// that a rebuild would silently drop.
	retired := `{"id":"hook-projection:hooks.json","kind":"hook-projection","source_path":"hooks.json","destination":"hooks.json","sha256":"` + digest + `"}`

	for name, testCase := range map[string]struct{ body, wantError string }{
		"duplicate kind field":  {body: manifest(duplicateKind+","+instruction, ""), wantError: `duplicate object key "kind"`},
		"trailing delimiter":    {body: manifest(retired+","+live+","+instruction, "}"), wantError: "trailing JSON values"},
		"trailing array delim":  {body: manifest(retired+","+live+","+instruction, "]"), wantError: "trailing JSON values"},
		"trailing second value": {body: manifest(retired+","+live+","+instruction, "{}"), wantError: "trailing JSON values"},
		// Bytes Unicode calls whitespace and JSON calls a syntax error. Each is
		// invalid trailing content the strict pass rejects, so the strip must not
		// erase it on the way past.
		"trailing vertical tab":       {body: manifest(retired+","+live+","+instruction, "\v"), wantError: "trailing JSON values"},
		"trailing form feed":          {body: manifest(retired+","+live+","+instruction, "\f"), wantError: "trailing JSON values"},
		"trailing non-breaking space": {body: manifest(retired+","+live+","+instruction, " "), wantError: "trailing JSON values"},
	} {
		t.Run(name, func(t *testing.T) {
			stripped, dropped := stripObsoleteHookProjectionRows([]byte(testCase.body))
			if dropped {
				t.Fatalf("stripObsoleteHookProjectionRows reported a drop for %s; an unidentifiable document must be handed back whole", name)
			}
			if string(stripped) != testCase.body {
				t.Fatalf("stripped = %s, want the original bytes untouched so the strict pass sees what was written", stripped)
			}

			path := filepath.Join(realpath(t, t.TempDir()), "manifest.json")
			writeInstallFile(t, path, testCase.body)
			_, err := readTargetAdapterManifest(path)
			if err == nil {
				t.Fatalf("readTargetAdapterManifest(%s) error = nil, want the strict refusal", name)
			}
			if !strings.Contains(err.Error(), testCase.wantError) {
				t.Fatalf("readTargetAdapterManifest(%s) error = %v, want it to name %q", name, err, testCase.wantError)
			}
		})
	}

	// The distinction the rule turns on: a repeat in a field that is not the
	// identity leaves the identity unambiguous, so that row still drops.
	repeatedDigest := `{"id":"hook-projection:hooks.json","kind":"hook-projection","source_path":"hooks.json","destination":"hooks.json","sha256":"` + digest + `","sha256":"` + digest + `"}`
	stripped, dropped := stripObsoleteHookProjectionRows([]byte(manifest(repeatedDigest+","+live+","+instruction, "")))
	if !dropped || strings.Contains(string(stripped), obsoleteHookProjectionKind) {
		t.Fatalf("stripObsoleteHookProjectionRows(repeated non-identity field) = %s, %v, want the row still dropped", stripped, dropped)
	}

	// And the four bytes JSON does allow after the document, which is how every
	// manifest anyone actually wrote ends. Narrowing the character class must
	// not cost the common case its strip.
	for name, trailing := range map[string]string{
		"newline":          "\n",
		"carriage return":  "\r\n",
		"spaces and tabs":  " \t ",
		"nothing at all":   "",
		"blank final line": "\n\n",
	} {
		t.Run("trailing "+name+" still strips", func(t *testing.T) {
			body := manifest(retired+","+live+","+instruction, trailing)
			stripped, dropped := stripObsoleteHookProjectionRows([]byte(body))
			if !dropped {
				t.Fatalf("stripObsoleteHookProjectionRows(%q) reported no drop; valid trailing whitespace must not block the strip", trailing)
			}
			if strings.Contains(string(stripped), obsoleteHookProjectionKind) {
				t.Fatalf("stripped = %s, want the retired row gone", stripped)
			}
			path := filepath.Join(realpath(t, t.TempDir()), "manifest.json")
			writeInstallFile(t, path, body)
			read, err := readTargetAdapterManifest(path)
			if err != nil {
				t.Fatalf("readTargetAdapterManifest error = %v, want the ordinary document read", err)
			}
			if !read.carriedObsoleteHookRow || len(read.Artifacts) != 2 {
				t.Fatalf("read = %#v, want the row seen, dropped, and both live rows kept", read)
			}
		})
	}
}

func TestCollectTargetAdapterArtifactsRejectsSymlinks(t *testing.T) {
	root := realpath(t, t.TempDir())
	writeInstallFile(t, filepath.Join(root, "plugins", "hooks.ts"), "plugin\n")
	if err := os.Symlink("hooks.ts", filepath.Join(root, "plugins", "linked.ts")); err != nil {
		t.Fatal(err)
	}
	if _, err := collectTargetAdapterArtifacts("opencode", root); err == nil || !strings.Contains(err.Error(), "symlink") {
		t.Fatalf("collectTargetAdapterArtifacts error = %v, want symlink refusal", err)
	}
}

func TestCollectTargetAdapterArtifactsCollectsIndependentAmpPlugins(t *testing.T) {
	root := realpath(t, t.TempDir())
	writeInstallFile(t, filepath.Join(root, ".amp", "plugins", "loaf.ts"), "hooks\n")
	writeInstallFile(t, filepath.Join(root, ".amp", "plugins", "loaf-modes.ts"), "modes\n")
	writeInstallFile(t, filepath.Join(root, ".amp", "plugins", "company.ts"), "company\n")
	artifacts, err := collectTargetAdapterArtifacts("amp", root)
	if err != nil {
		t.Fatalf("collectTargetAdapterArtifacts error = %v", err)
	}
	got := map[string]string{}
	for _, artifact := range artifacts {
		if artifact.Kind != "plugin" {
			t.Fatalf("artifact = %#v, want plugin kind", artifact)
		}
		got[artifact.Destination] = artifact.ID
	}
	if got["plugins/loaf.ts"] != "plugin:.amp/plugins/loaf.ts" || got["plugins/loaf-modes.ts"] != "plugin:.amp/plugins/loaf-modes.ts" {
		t.Fatalf("artifacts = %#v, want independent loaf.ts and loaf-modes.ts plugins", artifacts)
	}
	if _, ok := got["plugins/company.ts"]; ok {
		t.Fatalf("artifacts = %#v, want Loaf-owned plugin files only", artifacts)
	}
	if len(got) != 2 {
		t.Fatalf("artifacts = %#v, want exactly two Amp plugins", artifacts)
	}
}

func TestAmpModesPluginExactPredecessorIsClosed(t *testing.T) {
	body := []byte(testAmpModesPredecessor(t))
	artifact := targetAdapterArtifact{
		ID:          ampModesPluginArtifactID,
		Kind:        "plugin",
		SourcePath:  ampModesPluginSourcePath,
		Destination: ampModesPluginDestination,
	}
	if !ampModesPluginExactPredecessor("amp", artifact, body) {
		t.Fatal("exact predecessor digest was refused")
	}
	if ampModesPluginExactPredecessor("opencode", artifact, body) {
		t.Fatal("non-amp target adopted the predecessor digest")
	}
	wrongID := artifact
	wrongID.ID = "plugin:.amp/plugins/loaf.ts"
	if ampModesPluginExactPredecessor("amp", wrongID, body) {
		t.Fatal("hook plugin identity adopted the modes predecessor digest")
	}
	modified := append(append([]byte{}, body...), ' ')
	if ampModesPluginExactPredecessor("amp", artifact, modified) {
		t.Fatal("one-byte modification adopted the predecessor digest")
	}
	if targetAdapterLegacyOwnership("amp", artifact, body) {
		t.Fatal("predecessor matched the generated-header heuristic")
	}
	if !strings.Contains(string(body), "name: 'grok-implementation-agent'") || !strings.Contains(string(body), "reasoningEffort: 'high'") {
		t.Fatal("predecessor fixture lost the known old high-effort Grok configuration")
	}
}
