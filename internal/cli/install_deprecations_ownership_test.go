package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// TestDestructiveMigrationPreservesUnowned exercises the real upgrade --yes
// destructive deprecation path. Ownership must come from the managed-skills
// digest manifest — not from SKILL.md existence — and unowned / mismatched /
// dangling targets must survive while Loaf-proven copies are retired.
func TestDestructiveMigrationPreservesUnowned(t *testing.T) {
	root, home := setupInstallCommandFixture(t)
	pathContext := map[string]string{
		"HOME":            home,
		"XDG_CONFIG_HOME": filepath.Join(home, ".config"),
	}
	homes, err := installHarnessSkillSearchHomes(pathContext)
	if err != nil {
		t.Fatalf("installHarnessSkillSearchHomes: %v", err)
	}
	if len(homes) == 0 {
		t.Fatal("derived migration path set is empty")
	}

	canonical := filepath.Join(home, ".agents", "skills")
	foreignBody := "# Foreign orchestration\n"
	writeInstallFile(t, filepath.Join(canonical, "orchestration", "SKILL.md"), foreignBody)

	// Manifest claims orchestration with a digest that does not match the
	// foreign tree — the ambiguous mismatch case. Hands off.
	foreignDigest, err := hashInstallSkillTree(filepath.Join(canonical, "orchestration"))
	if err != nil {
		t.Fatalf("hash foreign orchestration: %v", err)
	}
	mismatchDigest := strings.Repeat("ab", 32)
	if mismatchDigest == foreignDigest {
		t.Fatal("test digest collision")
	}

	// Owned retired skill present in every derived prior skill home.
	ownedBody := "# Loaf-owned retired skill\n"
	for _, skillHome := range homes {
		seedOwnedManagedSkill(t, skillHome, "cli-reference", ownedBody)
	}

	// Manifest entry whose digest no longer matches (separate from foreign).
	writeInstallFile(t, filepath.Join(canonical, "tampered", "SKILL.md"), "# Tampered after install\n")
	upsertManagedSkillDigest(t, canonical, "orchestration", mismatchDigest)
	upsertManagedSkillDigest(t, canonical, "tampered", mismatchDigest)

	// Dangling symlink at a managed target in the canonical store.
	dangling := filepath.Join(canonical, "dangling-managed")
	if err := os.Symlink(filepath.Join(canonical, "missing-target-dir"), dangling); err != nil {
		t.Fatalf("Symlink dangling: %v", err)
	}
	upsertManagedSkillDigest(t, canonical, "dangling-managed", strings.Repeat("cd", 32))

	writeInstallDeprecationManifest(t, root, `{
  "version": 1,
  "retired_targets": [],
  "retired_skills": [
    {
      "skill": "cli-reference",
      "since": "v9.9.0",
      "window": "one-release",
      "reason": "retired for ownership hardening test",
      "skill_homes": [
        "${HOME}/.agents/skills",
        "${HOME}/.config/agents/skills",
        "${HOME}/.config/opencode/skills",
        "${HOME}/.cursor/skills",
        "${HOME}/.claude/skills"
      ]
    },
    {
      "skill": "orchestration",
      "since": "v9.9.0",
      "reason": "should not delete foreign or mismatched",
      "skill_homes": ["${HOME}/.agents/skills"]
    },
    {
      "skill": "tampered",
      "since": "v9.9.0",
      "reason": "digest mismatch must un-manage, not delete",
      "skill_homes": ["${HOME}/.agents/skills"]
    },
    {
      "skill": "dangling-managed",
      "since": "v9.9.0",
      "reason": "dangling symlink must be left alone",
      "skill_homes": ["${HOME}/.agents/skills"]
    }
  ],
  "retired_agents": [],
  "externalized_skills": [
    {
      "skill": "thermo-nuclear-code-quality-review",
      "since": "v9.9.0",
      "reason": "report-only externalized skill",
      "source": "https://example.com/vendor-skill",
      "install_command": "loaf skill add https://example.com/vendor-skill",
      "skill_homes": ["${HOME}/.agents/skills"]
    }
  ],
  "relocations": [],
  "aliases": []
}`)

	// Externalized skill present — report only, never remove.
	extPath := filepath.Join(canonical, "thermo-nuclear-code-quality-review", "SKILL.md")
	writeInstallFile(t, extPath, "# Vendor externalized\n")

	var stdout bytes.Buffer
	if err := (Runner{Stdout: &stdout, WorkingDir: root, Executable: distributionFixtureExecutable(root)}).Run([]string{"upgrade", "--yes"}); err != nil {
		t.Fatalf("upgrade --yes error = %v\n%s", err, stdout.String())
	}
	out := stdout.String()

	// Foreign orchestration untouched.
	assertInstallFile(t, filepath.Join(canonical, "orchestration", "SKILL.md"), foreignBody)
	// Digest-mismatched tree untouched.
	assertInstallFile(t, filepath.Join(canonical, "tampered", "SKILL.md"), "# Tampered after install\n")
	// Dangling symlink untouched.
	if info, err := os.Lstat(dangling); err != nil || info.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("dangling symlink Lstat = %v info=%v, want preserved symlink", err, info)
	}
	// Externalized report-only: still present.
	assertInstallFile(t, extPath, "# Vendor externalized\n")

	// Every derived prior home: owned retired skill removed.
	for _, skillHome := range homes {
		assertInstallPathMissing(t, filepath.Join(skillHome, "cli-reference"))
	}

	// Distinct reporting: retired vs un-managed (not collapsed).
	if !strings.Contains(out, "removed retired skill cli-reference") && !strings.Contains(out, "retired skill cli-reference") {
		t.Fatalf("stdout missing retired reporting:\n%s", out)
	}
	if !strings.Contains(out, "un-managed") && !strings.Contains(out, "unmanaged") {
		t.Fatalf("stdout missing un-managed reporting:\n%s", out)
	}
	if !strings.Contains(out, "orchestration") || !strings.Contains(out, "tampered") {
		t.Fatalf("stdout should name un-managed skills:\n%s", out)
	}
	if !strings.Contains(out, "dangling") {
		t.Fatalf("stdout should mention dangling symlink handling:\n%s", out)
	}
	if !strings.Contains(out, "migration receipt") || !strings.Contains(out, "before=") || !strings.Contains(out, "after=") {
		t.Fatalf("stdout missing before/after migration receipt:\n%s", out)
	}
	if !strings.Contains(out, "externalized skill thermo-nuclear-code-quality-review") {
		t.Fatalf("stdout missing externalized report-only line:\n%s", out)
	}

	// Manifest no longer claims mismatched / dangling / retired entries.
	state, err := readManagedSkillsState(canonical)
	if err != nil {
		t.Fatalf("readManagedSkillsState: %v", err)
	}
	for _, name := range []string{"cli-reference", "orchestration", "tampered", "dangling-managed"} {
		if _, ok := state.digests[name]; ok {
			t.Fatalf("manifest still claims %q after un-manage/retire: %#v", name, state.digests)
		}
	}

	// Interruption then retry: remove an owned skill dir but leave the digest
	// claim (crash between RemoveAll and manifest rewrite), then re-run.
	interruptedHome := homes[0]
	seedOwnedManagedSkill(t, interruptedHome, "cli-reference", ownedBody)
	if err := os.RemoveAll(filepath.Join(interruptedHome, "cli-reference")); err != nil {
		t.Fatalf("interrupt RemoveAll: %v", err)
	}
	// Manifest still claims it — simulate crash before un-manage rewrite.
	staleDigest := strings.Repeat("ef", 32)
	upsertManagedSkillDigest(t, interruptedHome, "cli-reference", staleDigest)

	stdout.Reset()
	if err := (Runner{Stdout: &stdout, WorkingDir: root, Executable: distributionFixtureExecutable(root)}).Run([]string{"upgrade", "--yes"}); err != nil {
		t.Fatalf("retry upgrade --yes error = %v\n%s", err, stdout.String())
	}
	assertInstallPathMissing(t, filepath.Join(interruptedHome, "cli-reference"))
	retryState, err := readManagedSkillsState(interruptedHome)
	if err != nil {
		t.Fatalf("retry readManagedSkillsState: %v", err)
	}
	if _, ok := retryState.digests["cli-reference"]; ok {
		t.Fatalf("retry left stale manifest claim: %#v", retryState.digests)
	}
	// Foreign content still intact after retry.
	assertInstallFile(t, filepath.Join(canonical, "orchestration", "SKILL.md"), foreignBody)
}

func seedOwnedManagedSkill(t *testing.T, skillHome, name, body string) {
	t.Helper()
	path := filepath.Join(skillHome, name)
	writeInstallFile(t, filepath.Join(path, "SKILL.md"), body)
	digest, err := hashInstallSkillTree(path)
	if err != nil {
		t.Fatalf("hashInstallSkillTree(%s): %v", path, err)
	}
	upsertManagedSkillDigest(t, skillHome, name, digest)
}

func upsertManagedSkillDigest(t *testing.T, skillHome, name, digest string) {
	t.Helper()
	state, err := readManagedSkillsState(skillHome)
	if err != nil {
		t.Fatalf("readManagedSkillsState(%s): %v", skillHome, err)
	}
	if state.digests == nil {
		state.digests = map[string]string{}
	}
	state.digests[name] = digest
	manifest := managedSkillsManifestV2{Version: 2, Skills: make([]managedSkillDigest, 0, len(state.digests))}
	names := make([]string, 0, len(state.digests))
	for n := range state.digests {
		names = append(names, n)
	}
	for i := 0; i < len(names); i++ {
		for j := i + 1; j < len(names); j++ {
			if names[j] < names[i] {
				names[i], names[j] = names[j], names[i]
			}
		}
	}
	for _, n := range names {
		manifest.Skills = append(manifest.Skills, managedSkillDigest{Name: n, SHA256: state.digests[n]})
	}
	if err := writeManagedSkillsManifest(skillHome, manifest); err != nil {
		t.Fatalf("writeManagedSkillsManifest(%s): %v", skillHome, err)
	}
	raw, err := os.ReadFile(filepath.Join(skillHome, loafSkillManifestFile))
	if err != nil {
		t.Fatalf("ReadFile manifest: %v", err)
	}
	var probe map[string]any
	if err := json.Unmarshal(raw, &probe); err != nil {
		t.Fatalf("manifest JSON: %v\n%s", err, raw)
	}
}

func seedManagedSkillsManifestV1(t *testing.T, skillHome string, names []string) {
	t.Helper()
	mkdirAll(t, skillHome)
	body, err := json.MarshalIndent(map[string]any{"version": 1, "skills": names}, "", "  ")
	if err != nil {
		t.Fatalf("marshal v1 manifest: %v", err)
	}
	writeInstallFile(t, filepath.Join(skillHome, loafSkillManifestFile), string(body)+"\n")
}

func TestDestructiveMigrationPreservesForeignSkillInRelocationSource(t *testing.T) {
	root, home := setupInstallCommandFixture(t)
	oldPath := filepath.Join(home, ".old-agents", "skills")
	newPath := filepath.Join(home, ".agents", "skills")
	writeInstallFile(t, filepath.Join(oldPath, loafInstallMarkerFile), "old\n")
	seedOwnedManagedSkill(t, oldPath, "foundations", "# Foundations\n")
	foreignBody := "# Foreign orchestration in relocation source\n"
	writeInstallFile(t, filepath.Join(oldPath, "orchestration", "SKILL.md"), foreignBody)

	writeInstallDeprecationManifest(t, root, `{
  "version": 1,
  "retired_targets": [],
  "retired_skills": [],
  "retired_agents": [],
  "relocations": [
    {
      "id": "old-agents-skills",
      "from": "${HOME}/.old-agents/skills",
      "to": "${HOME}/.agents/skills",
      "since": "v9.9.0",
      "reason": "skills moved"
    }
  ],
  "aliases": []
}`)

	var stdout bytes.Buffer
	if err := (Runner{Stdout: &stdout, WorkingDir: root, Executable: distributionFixtureExecutable(root)}).Run([]string{"upgrade", "--yes"}); err != nil {
		t.Fatalf("upgrade --yes error = %v\n%s", err, stdout.String())
	}
	assertInstallFile(t, filepath.Join(oldPath, "orchestration", "SKILL.md"), foreignBody)
	assertInstallFile(t, filepath.Join(newPath, "foundations", "SKILL.md"), "# Foundations\n")
	assertInstallPathMissing(t, filepath.Join(oldPath, "foundations"))
	if !strings.Contains(stdout.String(), "un-managed") || !strings.Contains(stdout.String(), "orchestration") {
		t.Fatalf("stdout missing un-managed foreign skill report:\n%s", stdout.String())
	}
}

func TestDestructiveMigrationRefusesSymlinkedSkillHome(t *testing.T) {
	root, home := setupInstallCommandFixture(t)
	realHome := filepath.Join(home, "vendor-skills")
	seedOwnedManagedSkill(t, realHome, "cli-reference", "# Owned through symlink home\n")
	linkHome := filepath.Join(home, ".agents", "skills")
	mkdirAll(t, filepath.Dir(linkHome))
	if err := os.Symlink(realHome, linkHome); err != nil {
		t.Fatalf("Symlink skill home: %v", err)
	}
	writeInstallDeprecationManifest(t, root, `{
  "version": 1,
  "retired_targets": [],
  "retired_skills": [
    {
      "skill": "cli-reference",
      "since": "v9.9.0",
      "reason": "must not mutate through symlinked home",
      "skill_homes": ["${HOME}/.agents/skills"]
    }
  ],
  "retired_agents": [],
  "relocations": [],
  "aliases": []
}`)

	var stdout bytes.Buffer
	if err := (Runner{Stdout: &stdout, WorkingDir: root, Executable: distributionFixtureExecutable(root)}).Run([]string{"upgrade", "--yes"}); err != nil {
		t.Fatalf("upgrade --yes error = %v\n%s", err, stdout.String())
	}
	assertInstallFile(t, filepath.Join(realHome, "cli-reference", "SKILL.md"), "# Owned through symlink home\n")
	state, err := readManagedSkillsState(realHome)
	if err != nil {
		t.Fatalf("readManagedSkillsState: %v", err)
	}
	if _, ok := state.digests["cli-reference"]; !ok {
		t.Fatalf("symlinked-home mutation dropped claim: %#v", state.digests)
	}
}

func TestDestructiveMigrationPreservesMarkerPresentForeignTarget(t *testing.T) {
	root, home := setupInstallCommandFixture(t)
	retiredTarget := filepath.Join(home, ".retired-tool")
	writeInstallFile(t, filepath.Join(retiredTarget, loafInstallMarkerFile), "old\n")
	writeInstallFile(t, filepath.Join(retiredTarget, "user-settings.json"), "{\"keep\":true}\n")
	writeInstallDeprecationManifest(t, root, `{
  "version": 1,
  "retired_targets": [
    {
      "target": "retired-tool",
      "since": "v9.9.0",
      "reason": "marker must not authorize RemoveAll",
      "paths": ["${HOME}/.retired-tool"]
    }
  ],
  "retired_skills": [],
  "retired_agents": [],
  "relocations": [],
  "aliases": []
}`)

	var stdout bytes.Buffer
	if err := (Runner{Stdout: &stdout, WorkingDir: root, Executable: distributionFixtureExecutable(root)}).Run([]string{"upgrade", "--yes"}); err != nil {
		t.Fatalf("upgrade --yes error = %v\n%s", err, stdout.String())
	}
	assertInstallFile(t, filepath.Join(retiredTarget, "user-settings.json"), "{\"keep\":true}\n")
	if _, err := os.Stat(retiredTarget); err != nil {
		t.Fatalf("foreign target directory must survive: %v", err)
	}
	if strings.Contains(stdout.String(), "removed retired target retired-tool") {
		t.Fatalf("must not RemoveAll foreign target:\n%s", stdout.String())
	}
}

func TestDestructiveMigrationPreservesMarkerPresentForeignAgent(t *testing.T) {
	root, home := setupInstallCommandFixture(t)
	agentHome := filepath.Join(home, ".cursor", "agents")
	retiredAgent := filepath.Join(agentHome, "old-agent.md")
	writeInstallFile(t, filepath.Join(root, "dist", "cursor", "skills", "foundations", "SKILL.md"), "# Foundations\n")
	installTestHookDistribution(t, root, "cursor")
	writeInstallFile(t, filepath.Join(home, ".cursor", loafInstallMarkerFile), "old\n")
	writeInstallFile(t, retiredAgent, "# User-authored agent\n")
	writeInstallDeprecationManifest(t, root, `{
  "version": 1,
  "retired_targets": [],
  "retired_skills": [],
  "retired_agents": [
    {
      "agent": "old-agent",
      "since": "v9.9.0",
      "reason": "marker must not authorize agent deletion",
      "agent_homes": ["${HOME}/.cursor/agents"]
    }
  ],
  "relocations": [],
  "aliases": []
}`)

	var stdout bytes.Buffer
	if err := (Runner{Stdout: &stdout, WorkingDir: root, Executable: distributionFixtureExecutable(root)}).Run([]string{"upgrade", "--yes"}); err != nil {
		t.Fatalf("upgrade --yes error = %v\n%s", err, stdout.String())
	}
	assertInstallFile(t, retiredAgent, "# User-authored agent\n")
	if strings.Contains(stdout.String(), "removed retired agent old-agent") {
		t.Fatalf("must not delete agent on marker alone:\n%s", stdout.String())
	}
}

func TestDestructiveMigrationRetiresFromMultiEntryV1Manifest(t *testing.T) {
	root, home := setupInstallCommandFixture(t)
	skillHome := filepath.Join(home, ".agents", "skills")
	writeInstallFile(t, filepath.Join(skillHome, "keep-me", "SKILL.md"), "# Keep\n")
	writeInstallFile(t, filepath.Join(skillHome, "retire-me", "SKILL.md"), "# Retire\n")
	seedManagedSkillsManifestV1(t, skillHome, []string{"keep-me", "retire-me"})
	writeInstallDeprecationManifest(t, root, `{
  "version": 1,
  "retired_targets": [],
  "retired_skills": [
    {
      "skill": "retire-me",
      "since": "v9.9.0",
      "reason": "v1 retire must not corrupt survivors",
      "skill_homes": ["${HOME}/.agents/skills"]
    }
  ],
  "retired_agents": [],
  "relocations": [],
  "aliases": []
}`)

	var stdout bytes.Buffer
	if err := (Runner{Stdout: &stdout, WorkingDir: root, Executable: distributionFixtureExecutable(root)}).Run([]string{"upgrade", "--yes"}); err != nil {
		t.Fatalf("upgrade --yes error = %v\n%s", err, stdout.String())
	}
	assertInstallFile(t, filepath.Join(skillHome, "keep-me", "SKILL.md"), "# Keep\n")
	assertInstallFile(t, filepath.Join(skillHome, "retire-me", "SKILL.md"), "# Retire\n")

	raw, err := os.ReadFile(filepath.Join(skillHome, loafSkillManifestFile))
	if err != nil {
		t.Fatalf("ReadFile surviving manifest: %v", err)
	}
	var probe map[string]any
	if err := json.Unmarshal(raw, &probe); err != nil {
		t.Fatalf("surviving manifest JSON: %v\n%s", err, raw)
	}
	version, _ := probe["version"].(float64)
	if version == 2 {
		skills, _ := probe["skills"].([]any)
		for _, entry := range skills {
			obj, _ := entry.(map[string]any)
			sha, _ := obj["sha256"].(string)
			if sha == "" {
				t.Fatalf("v1 unmanage wrote unreadable empty v2 digest:\n%s", raw)
			}
		}
	}
	state, err := readManagedSkillsState(skillHome)
	if err != nil {
		t.Fatalf("readManagedSkillsState after v1 retire: %v\n%s", err, raw)
	}
	if _, ok := state.digests["keep-me"]; !ok {
		t.Fatalf("survivor keep-me missing from claim map: %#v\n%s", state.digests, raw)
	}
	if _, ok := state.digests["retire-me"]; ok {
		t.Fatalf("retire-me still claimed: %#v", state.digests)
	}
}

func TestDestructiveMigrationRelocationTransfersClaimForCanonicalSync(t *testing.T) {
	root, home := setupInstallCommandFixture(t)
	oldPath := filepath.Join(home, ".config", "opencode", "skills")
	newPath := filepath.Join(home, ".agents", "skills")
	ownerMarker := filepath.Join(home, ".config", "opencode", loafInstallMarkerFile)
	writeInstallFile(t, ownerMarker, "old\n")
	seedOwnedManagedSkill(t, oldPath, "foundations", "# Old foundations\n")
	writeInstallFile(t, filepath.Join(root, "dist", "opencode", "skills", "foundations", "SKILL.md"), "# New foundations\n")
	writeInstallDeprecationManifest(t, root, `{
  "version": 1,
  "retired_targets": [],
  "retired_skills": [],
  "retired_agents": [],
  "relocations": [
    {
      "id": "opencode-skills-to-agents-home",
      "from": "${XDG_CONFIG_HOME}/opencode/skills",
      "to": "${HOME}/.agents/skills",
      "owner_marker": "${XDG_CONFIG_HOME}/opencode/.loaf-version",
      "since": "v9.9.0",
      "reason": "skills moved"
    }
  ],
  "aliases": []
}`)

	var stdout bytes.Buffer
	if err := (Runner{Stdout: &stdout, WorkingDir: root, Executable: distributionFixtureExecutable(root)}).Run([]string{"upgrade", "--yes"}); err != nil {
		t.Fatalf("upgrade --yes error = %v\n%s", err, stdout.String())
	}
	assertInstallFile(t, filepath.Join(newPath, "foundations", "SKILL.md"), "# New foundations\n")
	state, err := readManagedSkillsState(newPath)
	if err != nil {
		t.Fatalf("destination claim missing after relocation+sync: %v", err)
	}
	if _, ok := state.digests["foundations"]; !ok {
		t.Fatalf("destination must claim foundations after relocation: %#v", state.digests)
	}
}

func TestDestructiveMigrationRelocationRenameInterruptRetry(t *testing.T) {
	root, home := setupInstallCommandFixture(t)
	oldPath := filepath.Join(home, ".old-agents", "skills")
	newPath := filepath.Join(home, ".agents", "skills")
	writeInstallFile(t, filepath.Join(oldPath, loafInstallMarkerFile), "old\n")
	seedOwnedManagedSkill(t, oldPath, "foundations", "# Foundations\n")
	state, err := readManagedSkillsState(oldPath)
	if err != nil {
		t.Fatalf("read source state: %v", err)
	}
	digest := state.digests["foundations"]

	// Simulate rename success + crash before source claim drop / dest claim write.
	mkdirAll(t, newPath)
	if err := os.Rename(filepath.Join(oldPath, "foundations"), filepath.Join(newPath, "foundations")); err != nil {
		t.Fatalf("simulate rename: %v", err)
	}
	upsertManagedSkillDigest(t, oldPath, "foundations", digest)

	writeInstallDeprecationManifest(t, root, `{
  "version": 1,
  "retired_targets": [],
  "retired_skills": [],
  "retired_agents": [],
  "relocations": [
    {
      "id": "old-agents-skills",
      "from": "${HOME}/.old-agents/skills",
      "to": "${HOME}/.agents/skills",
      "since": "v9.9.0",
      "reason": "skills moved"
    }
  ],
  "aliases": []
}`)

	var stdout bytes.Buffer
	if err := (Runner{Stdout: &stdout, WorkingDir: root, Executable: distributionFixtureExecutable(root)}).Run([]string{"upgrade", "--yes"}); err != nil {
		t.Fatalf("retry upgrade --yes error = %v\n%s", err, stdout.String())
	}
	assertInstallFile(t, filepath.Join(newPath, "foundations", "SKILL.md"), "# Foundations\n")
	srcState, err := readManagedSkillsState(oldPath)
	if err == nil {
		if _, ok := srcState.digests["foundations"]; ok {
			t.Fatalf("source still claims foundations after retry: %#v", srcState.digests)
		}
	}
	destState, err := readManagedSkillsState(newPath)
	if err != nil {
		t.Fatalf("destination claim after retry: %v", err)
	}
	if got := destState.digests["foundations"]; got != digest {
		t.Fatalf("destination digest = %q, want %q", got, digest)
	}
}

func TestDestructiveMigrationRelocationPreservesWhenDestinationHomeEmpty(t *testing.T) {
	root, home := setupInstallCommandFixture(t)
	oldPath := filepath.Join(home, ".old-agents", "skills")
	newPath := filepath.Join(home, ".agents", "skills")
	writeInstallFile(t, filepath.Join(oldPath, loafInstallMarkerFile), "old\n")
	seedOwnedManagedSkill(t, oldPath, "foundations", "# Foundations only copy\n")
	// Destination home already exists (normal post-install state) but is empty —
	// no foundations at dest. Source must be moved, never deleted.
	mkdirAll(t, newPath)
	writeInstallDeprecationManifest(t, root, `{
  "version": 1,
  "retired_targets": [],
  "retired_skills": [],
  "retired_agents": [],
  "relocations": [
    {
      "id": "old-agents-skills",
      "from": "${HOME}/.old-agents/skills",
      "to": "${HOME}/.agents/skills",
      "since": "v9.9.0",
      "reason": "skills moved"
    }
  ],
  "aliases": []
}`)

	var stdout bytes.Buffer
	if err := (Runner{Stdout: &stdout, WorkingDir: root, Executable: distributionFixtureExecutable(root)}).Run([]string{"upgrade", "--yes"}); err != nil {
		t.Fatalf("upgrade --yes error = %v\n%s", err, stdout.String())
	}
	assertInstallFile(t, filepath.Join(newPath, "foundations", "SKILL.md"), "# Foundations only copy\n")
	assertInstallPathMissing(t, filepath.Join(oldPath, "foundations"))
	state, err := readManagedSkillsState(newPath)
	if err != nil {
		t.Fatalf("destination claim after empty-home relocation: %v", err)
	}
	if _, ok := state.digests["foundations"]; !ok {
		t.Fatalf("destination must claim foundations after move: %#v", state.digests)
	}
}

func TestDestructiveMigrationRelocationPreservesWhenDestinationForeign(t *testing.T) {
	root, home := setupInstallCommandFixture(t)
	oldPath := filepath.Join(home, ".old-agents", "skills")
	newPath := filepath.Join(home, ".agents", "skills")
	writeInstallFile(t, filepath.Join(oldPath, loafInstallMarkerFile), "old\n")
	seedOwnedManagedSkill(t, oldPath, "foundations", "# Source foundations\n")
	mkdirAll(t, newPath)
	foreignBody := "# Foreign foundations at destination\n"
	writeInstallFile(t, filepath.Join(newPath, "foundations", "SKILL.md"), foreignBody)
	writeInstallDeprecationManifest(t, root, `{
  "version": 1,
  "retired_targets": [],
  "retired_skills": [],
  "retired_agents": [],
  "relocations": [
    {
      "id": "old-agents-skills",
      "from": "${HOME}/.old-agents/skills",
      "to": "${HOME}/.agents/skills",
      "since": "v9.9.0",
      "reason": "skills moved"
    }
  ],
  "aliases": []
}`)

	var stdout bytes.Buffer
	if err := (Runner{Stdout: &stdout, WorkingDir: root, Executable: distributionFixtureExecutable(root)}).Run([]string{"upgrade", "--yes"}); err != nil {
		t.Fatalf("upgrade --yes error = %v\n%s", err, stdout.String())
	}
	assertInstallFile(t, filepath.Join(oldPath, "foundations", "SKILL.md"), "# Source foundations\n")
	assertInstallFile(t, filepath.Join(newPath, "foundations", "SKILL.md"), foreignBody)
	srcState, err := readManagedSkillsState(oldPath)
	if err != nil {
		t.Fatalf("source claim must survive foreign destination: %v", err)
	}
	if _, ok := srcState.digests["foundations"]; !ok {
		t.Fatalf("source must keep foundations claim when dest is foreign: %#v", srcState.digests)
	}
}

func TestDestructiveMigrationNoReceiptWhenDestructiveNoOp(t *testing.T) {
	root, home := setupInstallCommandFixture(t)
	// Wet run (--yes) with nothing to mutate: retired skill already absent.
	writeInstallDeprecationManifest(t, root, `{
  "version": 1,
  "retired_targets": [],
  "retired_skills": [
    {
      "skill": "already-gone",
      "since": "v9.9.0",
      "reason": "destructive no-op must not emit receipt",
      "skill_homes": ["${HOME}/.agents/skills"]
    }
  ],
  "retired_agents": [],
  "relocations": [],
  "aliases": []
}`)

	var stdout bytes.Buffer
	if err := (Runner{Stdout: &stdout, WorkingDir: root, Executable: distributionFixtureExecutable(root)}).Run([]string{"upgrade", "--yes"}); err != nil {
		t.Fatalf("upgrade --yes error = %v\n%s", err, stdout.String())
	}
	if strings.Contains(stdout.String(), "migration receipt") {
		t.Fatalf("destructive no-op must not emit migration receipt:\n%s", stdout.String())
	}
	receiptDir := filepath.Join(home, ".agents", "loaf", "migration-receipts")
	if entries, err := os.ReadDir(receiptDir); err == nil && len(entries) > 0 {
		t.Fatalf("destructive no-op must not persist receipt entries: %v", entries)
	}
}

func TestDestructiveMigrationRelocationRenameInterruptRetryIdempotent(t *testing.T) {
	root, home := setupInstallCommandFixture(t)
	oldPath := filepath.Join(home, ".old-agents", "skills")
	newPath := filepath.Join(home, ".agents", "skills")
	writeInstallFile(t, filepath.Join(oldPath, loafInstallMarkerFile), "old\n")
	seedOwnedManagedSkill(t, oldPath, "foundations", "# Foundations\n")
	state, err := readManagedSkillsState(oldPath)
	if err != nil {
		t.Fatalf("read source state: %v", err)
	}
	digest := state.digests["foundations"]

	mkdirAll(t, newPath)
	if err := os.Rename(filepath.Join(oldPath, "foundations"), filepath.Join(newPath, "foundations")); err != nil {
		t.Fatalf("simulate rename: %v", err)
	}
	upsertManagedSkillDigest(t, oldPath, "foundations", digest)

	writeInstallDeprecationManifest(t, root, `{
  "version": 1,
  "retired_targets": [],
  "retired_skills": [],
  "retired_agents": [],
  "relocations": [
    {
      "id": "old-agents-skills",
      "from": "${HOME}/.old-agents/skills",
      "to": "${HOME}/.agents/skills",
      "since": "v9.9.0",
      "reason": "skills moved"
    }
  ],
  "aliases": []
}`)

	var stdout bytes.Buffer
	for attempt := 1; attempt <= 3; attempt++ {
		stdout.Reset()
		if err := (Runner{Stdout: &stdout, WorkingDir: root, Executable: distributionFixtureExecutable(root)}).Run([]string{"upgrade", "--yes"}); err != nil {
			t.Fatalf("retry %d upgrade --yes error = %v\n%s", attempt, err, stdout.String())
		}
		assertInstallFile(t, filepath.Join(newPath, "foundations", "SKILL.md"), "# Foundations\n")
		srcState, err := readManagedSkillsState(oldPath)
		if err == nil {
			if _, ok := srcState.digests["foundations"]; ok {
				t.Fatalf("attempt %d: source still claims foundations: %#v", attempt, srcState.digests)
			}
		}
		destState, err := readManagedSkillsState(newPath)
		if err != nil {
			t.Fatalf("attempt %d: destination claim: %v", attempt, err)
		}
		if got := destState.digests["foundations"]; got != digest {
			t.Fatalf("attempt %d: destination digest = %q, want %q", attempt, got, digest)
		}
	}
}

func TestDestructiveMigrationReceiptRefusesSymlinkedLoafDir(t *testing.T) {
	root, home := setupInstallCommandFixture(t)
	skillHome := filepath.Join(home, ".agents", "skills")
	seedOwnedManagedSkill(t, skillHome, "old-skill", "# Old skill\n")
	vendor := filepath.Join(home, "vendor-loaf-state")
	mkdirAll(t, vendor)
	loafDir := filepath.Join(home, ".agents", "loaf")
	mkdirAll(t, filepath.Dir(loafDir))
	if err := os.Symlink(vendor, loafDir); err != nil {
		t.Fatalf("Symlink .agents/loaf: %v", err)
	}
	writeInstallDeprecationManifest(t, root, `{
  "version": 1,
  "retired_targets": [],
  "retired_skills": [
    {
      "skill": "old-skill",
      "since": "v9.9.0",
      "reason": "mutate then refuse symlinked receipt path",
      "skill_homes": ["${HOME}/.agents/skills"]
    }
  ],
  "retired_agents": [],
  "relocations": [],
  "aliases": []
}`)

	var stdout bytes.Buffer
	if err := (Runner{Stdout: &stdout, WorkingDir: root, Executable: distributionFixtureExecutable(root)}).Run([]string{"upgrade", "--yes"}); err != nil {
		t.Fatalf("upgrade must not abort on receipt persistence failure: %v\n%s", err, stdout.String())
	}
	assertInstallPathMissing(t, filepath.Join(skillHome, "old-skill"))
	out := stdout.String()
	if !strings.Contains(out, "install deprecation cleanup") {
		t.Fatalf("cleanup report must still be emitted:\n%s", out)
	}
	if !strings.Contains(out, "symlink") && !strings.Contains(out, "persistence failed") {
		t.Fatalf("stdout must report receipt persistence problem loudly:\n%s", out)
	}
	if _, err := os.Stat(filepath.Join(vendor, "migration-receipts", "latest.json")); err == nil {
		t.Fatal("must not write migration receipt through symlinked .agents/loaf into vendor state")
	}
}

func TestDestructiveMigrationReceiptOnlyWhenMutationOccurs(t *testing.T) {
	root, home := setupInstallCommandFixture(t)
	skillHome := filepath.Join(home, ".agents", "skills")
	seedOwnedManagedSkill(t, skillHome, "old-skill", "# Old skill\n")
	writeInstallDeprecationManifest(t, root, `{
  "version": 1,
  "retired_targets": [],
  "retired_skills": [
    {
      "skill": "old-skill",
      "since": "v9.9.0",
      "reason": "mutate then check receipt",
      "skill_homes": ["${HOME}/.agents/skills"]
    }
  ],
  "retired_agents": [],
  "relocations": [],
  "aliases": []
}`)

	var dry bytes.Buffer
	if err := (Runner{Stdout: &dry, WorkingDir: root, Executable: distributionFixtureExecutable(root)}).Run([]string{"upgrade"}); err != nil {
		t.Fatalf("dry upgrade error = %v\n%s", err, dry.String())
	}
	if strings.Contains(dry.String(), "migration receipt") {
		t.Fatalf("dry upgrade must not emit migration receipt:\n%s", dry.String())
	}

	var wet bytes.Buffer
	if err := (Runner{Stdout: &wet, WorkingDir: root, Executable: distributionFixtureExecutable(root)}).Run([]string{"upgrade", "--yes"}); err != nil {
		t.Fatalf("wet upgrade error = %v\n%s", err, wet.String())
	}
	out := wet.String()
	if !strings.Contains(out, "migration receipt") || !strings.Contains(out, "before=") || !strings.Contains(out, "after=") {
		t.Fatalf("mutating upgrade missing receipt:\n%s", out)
	}
	beforeIdx := strings.Index(out, "before=")
	afterIdx := strings.Index(out, "after=")
	if beforeIdx < 0 || afterIdx < 0 {
		t.Fatalf("receipt fields missing:\n%s", out)
	}
	before := strings.Fields(out[beforeIdx:])[0]
	after := strings.Fields(out[afterIdx:])[0]
	before = strings.TrimPrefix(before, "before=")
	after = strings.TrimPrefix(after, "after=")
	if before == after {
		t.Fatalf("mutating upgrade receipt before==after (%s); want divergence\n%s", before, out)
	}
	receiptDir := filepath.Join(home, ".agents", "loaf", "migration-receipts")
	entries, err := os.ReadDir(receiptDir)
	if err != nil || len(entries) == 0 {
		t.Fatalf("expected persisted migration receipt under %s: err=%v entries=%v", receiptDir, err, entries)
	}
}

func TestDestructiveMigrationQuarantineRollbackFailureIsLoud(t *testing.T) {
	_, home := setupInstallCommandFixture(t)
	skillHome := filepath.Join(home, ".agents", "skills")
	body := "# Owned skill for quarantine rollback\n"
	seedOwnedManagedSkill(t, skillHome, "cli-reference", body)
	state, err := readManagedSkillsState(skillHome)
	if err != nil {
		t.Fatalf("readManagedSkillsState: %v", err)
	}
	digest := state.digests["cli-reference"]

	var strandedQuarantine string
	t.Cleanup(func() { quarantinePostRenameHook = nil })
	quarantinePostRenameHook = func(home, skill, quarantine string) error {
		strandedQuarantine = quarantine
		// Simulate another process recreating the original path during revalidation.
		if err := os.MkdirAll(filepath.Join(home, skill), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(home, skill, "SKILL.md"), []byte("# raced recreate\n"), 0o644); err != nil {
			return err
		}
		// Tamper the quarantined tree so revalidation fails and rollback is attempted.
		return os.WriteFile(filepath.Join(quarantine, "SKILL.md"), []byte("# tampered quarantine\n"), 0o644)
	}

	err = removeOwnedManagedSkillTree(skillHome, "cli-reference", digest)
	if err == nil {
		t.Fatal("expected quarantine rollback failure")
	}
	msg := err.Error()
	if strandedQuarantine == "" {
		t.Fatal("hook did not capture quarantine path")
	}
	for _, want := range []string{
		"quarantine rollback failed",
		"stranded at",
		strandedQuarantine,
		"cli-reference",
	} {
		if !strings.Contains(msg, want) {
			t.Fatalf("rollback error = %q, want substring %q", msg, want)
		}
	}
	// Real tree must remain discoverable at the quarantine path.
	assertInstallFile(t, filepath.Join(strandedQuarantine, "SKILL.md"), "# tampered quarantine\n")
}

func TestDestructiveMigrationDiscoversQuarantineOrphanOnSubsequentRun(t *testing.T) {
	root, home := setupInstallCommandFixture(t)
	skillHome := filepath.Join(home, ".agents", "skills")
	mkdirAll(t, skillHome)
	writeInstallFile(t, filepath.Join(skillHome, loafInstallMarkerFile), "owned\n")

	orphanBody := "# Stranded quarantine tree\n"
	quarantine := filepath.Join(skillHome, ".cli-reference.loaf-quarantine-deadbeefcafebabe")
	writeInstallFile(t, filepath.Join(quarantine, "SKILL.md"), orphanBody)
	// Original name absent — cleanup must report the orphan and must not rename it.
	assertInstallPathMissing(t, filepath.Join(skillHome, "cli-reference"))

	foreignOrphanBody := "# Foreign directory that only looks like a quarantine name\n"
	foreignQuarantine := filepath.Join(skillHome, ".foreign.loaf-quarantine-deadbeefcafebabe")
	writeInstallFile(t, filepath.Join(foreignQuarantine, "SKILL.md"), foreignOrphanBody)

	writeInstallDeprecationManifest(t, root, `{
  "version": 1,
  "retired_targets": [],
  "retired_skills": [
    {
      "skill": "unrelated-absent",
      "since": "v9.9.0",
      "reason": "drive a cleanup pass that must notice quarantine orphans",
      "skill_homes": ["${HOME}/.agents/skills"]
    }
  ],
  "retired_agents": [],
  "relocations": [],
  "aliases": []
}`)

	var stdout bytes.Buffer
	if err := (Runner{Stdout: &stdout, WorkingDir: root, Executable: distributionFixtureExecutable(root)}).Run([]string{"upgrade", "--yes"}); err != nil {
		t.Fatalf("upgrade --yes error = %v\n%s", err, stdout.String())
	}
	out := stdout.String()
	if _, err := os.Lstat(filepath.Join(skillHome, "cli-reference")); err == nil {
		t.Fatalf("must not recover/rename quarantine orphan into place; stdout:\n%s", out)
	}
	assertInstallFile(t, filepath.Join(quarantine, "SKILL.md"), orphanBody)
	if !strings.Contains(out, quarantine) && !strings.Contains(out, ".cli-reference.loaf-quarantine-") {
		t.Fatalf("stdout must name orphan quarantine path; got:\n%s", out)
	}
	if !strings.Contains(out, "quarantine orphan") {
		t.Fatalf("stdout must report quarantine orphan (not recovery):\n%s", out)
	}
	if strings.Contains(out, "recovered quarantine") {
		t.Fatalf("stdout must not claim recovery:\n%s", out)
	}

	// Foreign-looking quarantine name: report, never move into "foreign".
	assertInstallFile(t, filepath.Join(foreignQuarantine, "SKILL.md"), foreignOrphanBody)
	assertInstallPathMissing(t, filepath.Join(skillHome, "foreign"))
	if !strings.Contains(out, foreignQuarantine) && !strings.Contains(out, ".foreign.loaf-quarantine-") {
		t.Fatalf("stdout must name foreign-looking quarantine path; got:\n%s", out)
	}
}

func TestDestructiveMigrationDiscoversQuarantineOrphanUnderRetiredTargetSkillsHome(t *testing.T) {
	root, home := setupInstallCommandFixture(t)
	targetHome := filepath.Join(home, ".gemini")
	skillsHome := filepath.Join(targetHome, "skills")
	mkdirAll(t, skillsHome)
	writeInstallFile(t, filepath.Join(targetHome, loafInstallMarkerFile), "owned\n")

	orphanBody := "# Stranded under retired target nested skills home\n"
	quarantine := filepath.Join(skillsHome, ".cli-reference.loaf-quarantine-aabbccddeeff0011")
	writeInstallFile(t, filepath.Join(quarantine, "SKILL.md"), orphanBody)

	writeInstallDeprecationManifest(t, root, `{
  "version": 1,
  "retired_targets": [
    {
      "target": "gemini",
      "since": "v9.9.0",
      "reason": "retired target whose nested skills home can strand quarantine",
      "paths": ["${HOME}/.gemini"]
    }
  ],
  "retired_skills": [],
  "retired_agents": [],
  "relocations": [],
  "aliases": []
}`)

	var stdout bytes.Buffer
	if err := (Runner{Stdout: &stdout, WorkingDir: root, Executable: distributionFixtureExecutable(root)}).Run([]string{"upgrade", "--yes"}); err != nil {
		t.Fatalf("upgrade --yes error = %v\n%s", err, stdout.String())
	}
	out := stdout.String()
	assertInstallFile(t, filepath.Join(quarantine, "SKILL.md"), orphanBody)
	assertInstallPathMissing(t, filepath.Join(skillsHome, "cli-reference"))
	if !strings.Contains(out, quarantine) && !strings.Contains(out, ".cli-reference.loaf-quarantine-") {
		t.Fatalf("stdout must discover orphan under retired target skills home; got:\n%s", out)
	}
	if !strings.Contains(out, "quarantine orphan") {
		t.Fatalf("stdout must report quarantine orphan:\n%s", out)
	}
}

func TestParseLoafQuarantineDirNameRoundTripEmbeddedMarker(t *testing.T) {
	// Skill names may contain ".loaf-quarantine-" (validManagedSkillName allows '.').
	// Generation and parsing must be exact inverses — parse from the right.
	skill := "foo.loaf-quarantine-bar"
	suffix := "deadbeefcafebabe"
	generated := "." + skill + loafQuarantineMarker + suffix
	parsed, ok := parseLoafQuarantineDirName(generated)
	if !ok {
		t.Fatalf("parseLoafQuarantineDirName(%q) = false, want skill %q", generated, skill)
	}
	if parsed != skill {
		t.Fatalf("parseLoafQuarantineDirName(%q) = %q, want %q", generated, parsed, skill)
	}
}

func TestDestructiveMigrationPreservesSymlinkedRelocationDestination(t *testing.T) {
	root, home := setupInstallCommandFixture(t)
	oldPath := filepath.Join(home, ".old-agents", "skills")
	realDest := filepath.Join(home, "vendor-dest-skills")
	newPath := filepath.Join(home, ".agents", "skills")
	mkdirAll(t, filepath.Join(home, ".agents"))
	mkdirAll(t, realDest)
	writeInstallFile(t, filepath.Join(oldPath, loafInstallMarkerFile), "old\n")
	seedOwnedManagedSkill(t, oldPath, "foundations", "# Foundations through symlink dest\n")
	if err := os.Symlink(realDest, newPath); err != nil {
		t.Fatalf("Symlink relocation destination: %v", err)
	}

	writeInstallDeprecationManifest(t, root, `{
  "version": 1,
  "retired_targets": [],
  "retired_skills": [],
  "retired_agents": [],
  "relocations": [
    {
      "id": "old-agents-skills",
      "from": "${HOME}/.old-agents/skills",
      "to": "${HOME}/.agents/skills",
      "since": "v9.9.0",
      "reason": "skills moved"
    }
  ],
  "aliases": []
}`)

	var stdout bytes.Buffer
	err := (Runner{Stdout: &stdout, WorkingDir: root, Executable: distributionFixtureExecutable(root)}).Run([]string{"upgrade", "--yes"})
	out := stdout.String()
	if err != nil {
		t.Fatalf("symlinked destination must preserve and report, not abort: %v\n%s", err, out)
	}
	assertInstallFile(t, filepath.Join(oldPath, "foundations", "SKILL.md"), "# Foundations through symlink dest\n")
	if _, err := os.Lstat(filepath.Join(realDest, "foundations")); err == nil {
		t.Fatalf("must not relocate into symlinked destination tree")
	}
	if !strings.Contains(out, "foundations") {
		t.Fatalf("stdout must mention preserved skill:\n%s", out)
	}
	if !strings.Contains(out, "un-managed") && !strings.Contains(out, "destination") && !strings.Contains(out, "symlink") {
		t.Fatalf("stdout must report destination refusal / preservation:\n%s", out)
	}
}

func TestDestructiveMigrationPreservesNonDirectoryRelocationDestination(t *testing.T) {
	// Deliberate destination refusals: regular file, symlink-to-file, dangling
	// symlink. Same class as a symlinked directory — preserve source, report
	// unmanaged, never abort upgrade.
	tests := []struct {
		name string
		prep func(t *testing.T, home, newPath string)
	}{
		{
			name: "regular_file",
			prep: func(t *testing.T, home, newPath string) {
				mkdirAll(t, filepath.Dir(newPath))
				writeInstallFile(t, newPath, "not a skills directory\n")
			},
		},
		{
			name: "symlink_to_regular_file",
			prep: func(t *testing.T, home, newPath string) {
				mkdirAll(t, filepath.Dir(newPath))
				target := filepath.Join(home, "skills-as-file")
				writeInstallFile(t, target, "file behind symlink\n")
				if err := os.Symlink(target, newPath); err != nil {
					t.Fatalf("Symlink to file: %v", err)
				}
			},
		},
		{
			name: "dangling_symlink",
			prep: func(t *testing.T, home, newPath string) {
				mkdirAll(t, filepath.Dir(newPath))
				if err := os.Symlink(filepath.Join(home, "missing-skills-target"), newPath); err != nil {
					t.Fatalf("dangling Symlink: %v", err)
				}
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			root, home := setupInstallCommandFixture(t)
			oldPath := filepath.Join(home, ".old-agents", "skills")
			newPath := filepath.Join(home, ".agents", "skills")
			writeInstallFile(t, filepath.Join(oldPath, loafInstallMarkerFile), "old\n")
			seedOwnedManagedSkill(t, oldPath, "foundations", "# Foundations blocked dest\n")
			tc.prep(t, home, newPath)

			writeInstallDeprecationManifest(t, root, `{
  "version": 1,
  "retired_targets": [],
  "retired_skills": [],
  "retired_agents": [],
  "relocations": [
    {
      "id": "old-agents-skills",
      "from": "${HOME}/.old-agents/skills",
      "to": "${HOME}/.agents/skills",
      "since": "v9.9.0",
      "reason": "skills moved"
    }
  ],
  "aliases": []
}`)

			var stdout bytes.Buffer
			err := (Runner{Stdout: &stdout, WorkingDir: root, Executable: distributionFixtureExecutable(root)}).Run([]string{"upgrade", "--yes"})
			out := stdout.String()
			if err != nil {
				t.Fatalf("non-directory destination must preserve and report, not abort: %v\n%s", err, out)
			}
			assertInstallFile(t, filepath.Join(oldPath, "foundations", "SKILL.md"), "# Foundations blocked dest\n")
			if !strings.Contains(out, "foundations") {
				t.Fatalf("stdout must mention preserved skill:\n%s", out)
			}
			if !strings.Contains(out, "un-managed") {
				t.Fatalf("stdout must report unmanaged preservation:\n%s", out)
			}
		})
	}
}

func TestDestructiveMigrationStrandedClaimPreservesNonDirectoryDestination(t *testing.T) {
	// Stranded claim (digest present, skill tree absent) must not die at
	// MkdirAll when the destination root is a non-directory — same refusal
	// class as the owned relocation path.
	root, home := setupInstallCommandFixture(t)
	oldPath := filepath.Join(home, ".old-agents", "skills")
	newPath := filepath.Join(home, ".agents", "skills")
	writeInstallFile(t, filepath.Join(oldPath, loafInstallMarkerFile), "old\n")
	upsertManagedSkillDigest(t, oldPath, "foundations", strings.Repeat("ab", 32))
	mkdirAll(t, filepath.Dir(newPath))
	writeInstallFile(t, newPath, "skills home is a file\n")

	writeInstallDeprecationManifest(t, root, `{
  "version": 1,
  "retired_targets": [],
  "retired_skills": [],
  "retired_agents": [],
  "relocations": [
    {
      "id": "old-agents-skills",
      "from": "${HOME}/.old-agents/skills",
      "to": "${HOME}/.agents/skills",
      "since": "v9.9.0",
      "reason": "skills moved"
    }
  ],
  "aliases": []
}`)

	var stdout bytes.Buffer
	err := (Runner{Stdout: &stdout, WorkingDir: root, Executable: distributionFixtureExecutable(root)}).Run([]string{"upgrade", "--yes"})
	out := stdout.String()
	if err != nil {
		t.Fatalf("stranded claim with non-directory dest must preserve and report, not abort: %v\n%s", err, out)
	}
	assertInstallFile(t, newPath, "skills home is a file\n")
	srcState, stateErr := readManagedSkillsState(oldPath)
	if stateErr != nil {
		t.Fatalf("source claim must remain readable: %v", stateErr)
	}
	if _, ok := srcState.digests["foundations"]; !ok {
		t.Fatalf("stranded claim must be preserved at source; digests=%#v", srcState.digests)
	}
	if !strings.Contains(out, "foundations") {
		t.Fatalf("stdout must mention stranded skill:\n%s", out)
	}
	if !strings.Contains(out, "un-managed") {
		t.Fatalf("stdout must report unmanaged stranded preservation:\n%s", out)
	}
}

func TestDestructiveMigrationRelocationDestinationPermissionErrorSurfaces(t *testing.T) {
	// Mirror risk guard: destination inspection EACCES must still abort, not
	// flatten into an unmanaged refusal.
	if runtime.GOOS == "windows" {
		t.Skip("POSIX permission bits do not deny reads on Windows")
	}
	if os.Geteuid() == 0 {
		t.Skip("running as root; permission bits deny nothing")
	}

	root, home := setupInstallCommandFixture(t)
	oldPath := filepath.Join(home, ".old-agents", "skills")
	agentsParent := filepath.Join(home, ".agents")
	newPath := filepath.Join(agentsParent, "skills")
	writeInstallFile(t, filepath.Join(oldPath, loafInstallMarkerFile), "old\n")
	seedOwnedManagedSkill(t, oldPath, "foundations", "# Foundations\n")
	mkdirAll(t, agentsParent)
	writeInstallDeprecationManifest(t, root, `{
  "version": 1,
  "retired_targets": [],
  "retired_skills": [],
  "retired_agents": [],
  "relocations": [
    {
      "id": "old-agents-skills",
      "from": "${HOME}/.old-agents/skills",
      "to": "${HOME}/.agents/skills",
      "since": "v9.9.0",
      "reason": "must surface EACCES on relocation destination"
    }
  ],
  "aliases": []
}`)
	chmodForTest(t, agentsParent, 0o000)

	var stdout bytes.Buffer
	err := (Runner{Stdout: &stdout, WorkingDir: root, Executable: distributionFixtureExecutable(root)}).Run([]string{"upgrade", "--yes"})
	out := stdout.String()
	if err == nil {
		t.Fatalf("expected destination inspection I/O error; stdout:\n%s", out)
	}
	msg := err.Error() + "\n" + out
	if strings.Contains(msg, "already absent") || strings.Contains(msg, "not marked as Loaf-owned") {
		t.Fatalf("I/O failure flattened to missing/unmarked:\n%s", msg)
	}
	if strings.Contains(msg, "un-managed") && !strings.Contains(strings.ToLower(msg), "permission") {
		t.Fatalf("permission failure must not become unmanaged refusal:\n%s", msg)
	}
	if !strings.Contains(strings.ToLower(msg), "permission") && !strings.Contains(msg, agentsParent) && !strings.Contains(msg, newPath) {
		t.Fatalf("error should surface permission/inspect failure for destination:\n%s", msg)
	}
}

func TestDestructiveMigrationInspectionIOErrorsSurface(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX permission bits do not deny reads on Windows")
	}
	if os.Geteuid() == 0 {
		t.Skip("running as root; permission bits deny nothing")
	}

	tests := []struct {
		name string
		prep func(t *testing.T, root, home string) (args []string, denyPath string)
	}{
		{
			name: "retired_skill_home",
			prep: func(t *testing.T, root, home string) ([]string, string) {
				skillHome := filepath.Join(home, ".agents", "skills")
				mkdirAll(t, skillHome)
				writeInstallFile(t, filepath.Join(skillHome, "old-skill", "SKILL.md"), "# present\n")
				writeInstallDeprecationManifest(t, root, `{
  "version": 1,
  "retired_targets": [],
  "retired_skills": [{
    "skill": "old-skill",
    "since": "v9.9.0",
    "reason": "must surface EACCES on home inspect",
    "skill_homes": ["${HOME}/.agents/skills"]
  }],
  "retired_agents": [],
  "relocations": [],
  "aliases": []
}`)
				parent := filepath.Join(home, ".agents")
				chmodForTest(t, parent, 0o000)
				return []string{"upgrade"}, parent
			},
		},
		{
			name: "relocation_source",
			prep: func(t *testing.T, root, home string) ([]string, string) {
				oldParent := filepath.Join(home, ".old-agents")
				oldPath := filepath.Join(oldParent, "skills")
				mkdirAll(t, oldPath)
				writeInstallFile(t, filepath.Join(oldPath, loafInstallMarkerFile), "old\n")
				seedOwnedManagedSkill(t, oldPath, "foundations", "# Foundations\n")
				writeInstallDeprecationManifest(t, root, `{
  "version": 1,
  "retired_targets": [],
  "retired_skills": [],
  "retired_agents": [],
  "relocations": [{
    "id": "old-agents-skills",
    "from": "${HOME}/.old-agents/skills",
    "to": "${HOME}/.agents/skills",
    "since": "v9.9.0",
    "reason": "must surface EACCES on relocation source"
  }],
  "aliases": []
}`)
				chmodForTest(t, oldParent, 0o000)
				return []string{"upgrade"}, oldParent
			},
		},
		{
			name: "relocation_candidate_manifest",
			prep: func(t *testing.T, root, home string) ([]string, string) {
				oldPath := filepath.Join(home, ".old-agents", "skills")
				mkdirAll(t, oldPath)
				// No install marker — candidacy depends on managed-skills digest map.
				seedOwnedManagedSkill(t, oldPath, "foundations", "# Foundations\n")
				manifestPath := filepath.Join(oldPath, loafSkillManifestFile)
				chmodForTest(t, manifestPath, 0o000)
				writeInstallDeprecationManifest(t, root, `{
  "version": 1,
  "retired_targets": [],
  "retired_skills": [],
  "retired_agents": [],
  "relocations": [{
    "id": "old-agents-skills",
    "from": "${HOME}/.old-agents/skills",
    "to": "${HOME}/.agents/skills",
    "since": "v9.9.0",
    "reason": "must surface EACCES reading managed-skills state"
  }],
  "aliases": []
}`)
				return []string{"upgrade"}, manifestPath
			},
		},
		{
			name: "retired_target_root",
			prep: func(t *testing.T, root, home string) ([]string, string) {
				targetParent := filepath.Join(home, ".config")
				target := filepath.Join(targetParent, "retired-tool")
				mkdirAll(t, target)
				writeInstallFile(t, filepath.Join(target, loafInstallMarkerFile), "v1\n")
				writeInstallDeprecationManifest(t, root, `{
  "version": 1,
  "retired_targets": [{
    "target": "retired-tool",
    "since": "v9.9.0",
    "reason": "must surface EACCES on retired target root",
    "paths": ["${HOME}/.config/retired-tool"]
  }],
  "retired_skills": [],
  "retired_agents": [],
  "relocations": [],
  "aliases": []
}`)
				chmodForTest(t, targetParent, 0o000)
				return []string{"upgrade"}, targetParent
			},
		},
		{
			name: "retired_agent_file",
			prep: func(t *testing.T, root, home string) ([]string, string) {
				agentHome := filepath.Join(home, ".config", "opencode", "agent")
				mkdirAll(t, agentHome)
				writeInstallFile(t, filepath.Join(agentHome, loafInstallMarkerFile), "owned\n")
				agentFile := filepath.Join(agentHome, "old-agent.md")
				writeInstallFile(t, agentFile, "# agent\n")
				writeInstallDeprecationManifest(t, root, `{
  "version": 1,
  "retired_targets": [],
  "retired_skills": [],
  "retired_agents": [{
    "agent": "old-agent",
    "since": "v9.9.0",
    "reason": "must surface EACCES on retired agent inspect",
    "agent_homes": ["${XDG_CONFIG_HOME}/opencode/agent"]
  }],
  "relocations": [],
  "aliases": []
}`)
				// Deny lookup of the agent file via its parent; orphan scanning does not
				// walk agent homes, so this hits the retired-agent inspection path.
				chmodForTest(t, agentHome, 0o000)
				return []string{"upgrade"}, agentHome
			},
		},
		{
			name: "externalized_skill_path",
			prep: func(t *testing.T, root, home string) ([]string, string) {
				skillHome := filepath.Join(home, ".agents", "skills")
				skillPath := filepath.Join(skillHome, "thermo-nuclear-code-quality-review")
				writeInstallFile(t, filepath.Join(skillPath, "SKILL.md"), "# Vendor\n")
				writeInstallDeprecationManifest(t, root, `{
  "version": 1,
  "retired_targets": [],
  "retired_skills": [],
  "retired_agents": [],
  "externalized_skills": [{
    "skill": "thermo-nuclear-code-quality-review",
    "since": "v9.9.0",
    "reason": "must surface EACCES on externalized inspect",
    "source": "https://example.com/vendor-skill",
    "install_command": "loaf skill add https://example.com/vendor-skill",
    "skill_homes": ["${HOME}/.agents/skills"]
  }],
  "relocations": [],
  "aliases": []
}`)
				// Deny traversal into the skill tree only — skill home stays readable so
				// quarantine orphan scanning does not preempt this path.
				chmodForTest(t, skillPath, 0o000)
				return []string{"upgrade"}, skillPath
			},
		}}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			root, home := setupInstallCommandFixture(t)
			args, denyPath := tc.prep(t, root, home)
			var stdout bytes.Buffer
			err := (Runner{Stdout: &stdout, WorkingDir: root, Executable: distributionFixtureExecutable(root)}).Run(args)
			out := stdout.String()
			if err == nil {
				t.Fatalf("expected inspection I/O error; stdout:\n%s", out)
			}
			msg := err.Error() + "\n" + out
			if strings.Contains(msg, "already absent") || strings.Contains(msg, "not marked as Loaf-owned") {
				t.Fatalf("I/O failure flattened to missing/unmarked:\n%s", msg)
			}
			if !strings.Contains(strings.ToLower(msg), "permission") && !strings.Contains(msg, denyPath) {
				t.Fatalf("error should surface permission/inspect failure for %s:\n%s", denyPath, msg)
			}
		})
	}
}

// TestClassifyManagedSkillOwnershipSurfacesChildPermissionError proves the
// classifier path directly. The upgrade-level classify_home_root table case was
// removed: quarantine orphan scanning now ReadDirs each skill home first
// (applyInstallDeprecationCleanup), so a mode-000 home fails there before
// classifyManagedSkillOwnership runs. A full-upgrade fixture therefore cannot
// name the classifier path honestly.
func TestClassifyManagedSkillOwnershipSurfacesChildPermissionError(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX permission bits do not deny reads on Windows")
	}
	if os.Geteuid() == 0 {
		t.Skip("running as root; permission bits deny nothing")
	}
	root, home := setupInstallCommandFixture(t)
	_ = root
	skillHome := filepath.Join(home, ".agents", "skills")
	mkdirAll(t, skillHome)
	writeInstallFile(t, filepath.Join(skillHome, "old-skill", "SKILL.md"), "# present\n")
	upsertManagedSkillDigest(t, skillHome, "old-skill", strings.Repeat("ab", 32))
	chmodForTest(t, skillHome, 0o000)
	_, err := classifyManagedSkillOwnership(skillHome, "old-skill")
	if err == nil {
		t.Fatal("expected permission error from classifyManagedSkillOwnership")
	}
	msg := err.Error()
	if strings.Contains(msg, "already absent") || strings.Contains(msg, "not marked as Loaf-owned") {
		t.Fatalf("I/O failure flattened: %v", err)
	}
	if !strings.Contains(strings.ToLower(msg), "permission") && !strings.Contains(msg, skillHome) {
		t.Fatalf("error should surface permission/inspect failure: %v", err)
	}
}
