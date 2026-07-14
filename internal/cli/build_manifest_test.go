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

func TestTargetAdapterManifestIsDeterministicAndInstructionDigestIgnoresVersion(t *testing.T) {
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
			{ID: "managed-instructions", Kind: "instruction", Destination: "project-instructions", SHA256: fencedContentFingerprint(generateFencedContent("1.0.0"))},
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
	if got, want := parsed.Artifacts[1].SHA256, fencedContentFingerprint(generateFencedContent("99.0.0")); got != want {
		t.Fatalf("instruction digest = %s, want version-independent %s", got, want)
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
		"projection mode":       strings.Replace(valid, `"kind":"plugin"`, `"kind":"hook-projection"`, 1),
	} {
		t.Run(name, func(t *testing.T) {
			writeInstallFile(t, path, body)
			if _, err := readTargetAdapterManifest(path); err == nil {
				t.Fatalf("readTargetAdapterManifest(%s) error = nil", name)
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
