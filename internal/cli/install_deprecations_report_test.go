package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestDeprecationReport is the TASK-007 contract: the deprecation report says
// something only when there is something to say, while every TASK-006 load-bearing
// action keeps printing.
func TestDeprecationReport(t *testing.T) {
	t.Run("absent_retirement_omitted", func(t *testing.T) {
		root, home := setupInstallCommandFixture(t)
		mkdirAll(t, filepath.Join(home, ".agents", "skills"))
		writeInstallDeprecationManifest(t, root, `{
  "version": 1,
  "retired_targets": [],
  "retired_skills": [
    {
      "skill": "already-gone",
      "since": "v9.9.0",
      "reason": "absent retirement is a no-op",
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
		if strings.Contains(out, "already absent") {
			t.Fatalf("absent retirement must not be reported:\n%s", out)
		}
		if strings.Contains(out, "already-gone") && strings.Contains(out, "install deprecation cleanup") {
			t.Fatalf("absent retirement must not appear in the deprecation report:\n%s", out)
		}
	})

	t.Run("unowned_path_omitted", func(t *testing.T) {
		root, home := setupInstallCommandFixture(t)
		unowned := filepath.Join(home, ".unmarked-tool")
		writeInstallFile(t, filepath.Join(unowned, "user-file.txt"), "keep me\n")
		writeInstallDeprecationManifest(t, root, `{
  "version": 1,
  "retired_targets": [
    {
      "target": "unmarked-tool",
      "since": "v9.9.0",
      "reason": "unowned path Loaf will never touch",
      "paths": ["${HOME}/.unmarked-tool"]
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
		assertInstallFile(t, filepath.Join(unowned, "user-file.txt"), "keep me\n")
		if strings.Contains(out, "not marked as Loaf-owned") || strings.Contains(out, "skip-unmarked") {
			t.Fatalf("unowned path must not be reported:\n%s", out)
		}
		if strings.Contains(out, "unmarked-tool") && strings.Contains(out, "install deprecation cleanup") {
			t.Fatalf("unowned retired path must not appear in the deprecation report:\n%s", out)
		}
	})

	t.Run("present_retirement_still_reported", func(t *testing.T) {
		root, home := setupInstallCommandFixture(t)
		retired := filepath.Join(home, ".retired-tool")
		writeInstallFile(t, filepath.Join(retired, loafInstallMarkerFile), "old\n")
		writeInstallDeprecationManifest(t, root, `{
  "version": 1,
  "retired_targets": [
    {
      "target": "retired-tool",
      "since": "v9.9.0",
      "reason": "present retirement must still report",
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
		out := stdout.String()
		if _, err := os.Stat(retired); !os.IsNotExist(err) {
			t.Fatalf("retired target stat = %v, want removed", err)
		}
		if !strings.Contains(out, "removed retired target retired-tool") {
			t.Fatalf("present retirement must still be reported:\n%s", out)
		}
	})

	t.Run("empty_retired_targets_ok", func(t *testing.T) {
		home := t.TempDir()
		pathContext := map[string]string{
			"HOME":            home,
			"XDG_CONFIG_HOME": filepath.Join(home, ".config"),
		}
		manifest := installDeprecationManifest{
			Version:        1,
			RetiredTargets: []retiredInstallTarget{},
			RetiredSkills:  []retiredInstallSkill{},
			RetiredAgents:  []retiredInstallAgent{},
		}
		result, err := applyInstallDeprecationCleanup(manifest, pathContext, true)
		if err != nil {
			t.Fatalf("empty retired_targets must not error: %v", err)
		}
		var buf bytes.Buffer
		writeInstallDeprecationCleanup(&buf, result)
		if buf.Len() != 0 {
			t.Fatalf("empty cleanup must write nothing, got:\n%s", buf.String())
		}
	})

	t.Run("confirmation_required_still_prints", func(t *testing.T) {
		// Pre-fix-green regression guard for TASK-006 confirmation-required.
		root, home := setupInstallCommandFixture(t)
		confirmTarget := filepath.Join(home, ".confirm-tool")
		writeInstallFile(t, filepath.Join(confirmTarget, loafInstallMarkerFile), "old\n")
		writeInstallDeprecationManifest(t, root, `{
  "version": 1,
  "retired_targets": [{
    "target": "confirm-tool",
    "since": "v9.9.0",
    "reason": "confirmation-required must still print",
    "paths": ["${HOME}/.confirm-tool"]
  }],
  "retired_skills": [],
  "retired_agents": [],
  "relocations": [],
  "aliases": []
}`)
		var stdout bytes.Buffer
		if err := (Runner{Stdout: &stdout, WorkingDir: root, Executable: distributionFixtureExecutable(root)}).Run([]string{"upgrade"}); err != nil {
			t.Fatalf("upgrade (no --yes) error = %v\n%s", err, stdout.String())
		}
		if !strings.Contains(stdout.String(), "rerun with --yes to apply destructive deprecation cleanup") {
			t.Fatalf("TASK-006 action confirmation-required must still print:\n%s", stdout.String())
		}
	})

	t.Run("task006_actions_still_print", func(t *testing.T) {
		// Regression guard: silencing unmanaged-missing, legacy-v1, or
		// quarantine-orphan (or the other TASK-006 report actions) fails here.
		root, home := setupInstallCommandFixture(t)
		skillHome := filepath.Join(home, ".agents", "skills")
		mkdirAll(t, skillHome)

		// unmanaged-missing: digest claim with no tree.
		upsertManagedSkillDigest(t, skillHome, "ghost-skill", strings.Repeat("aa", 32))

		// dangling: managed symlink to nowhere.
		dangling := filepath.Join(skillHome, "dangling-skill")
		if err := os.Symlink(filepath.Join(skillHome, "missing-target-dir"), dangling); err != nil {
			t.Fatalf("Symlink dangling: %v", err)
		}
		upsertManagedSkillDigest(t, skillHome, "dangling-skill", strings.Repeat("cd", 32))

		// quarantine-orphan: stranded quarantine dir.
		orphan := filepath.Join(skillHome, ".orphan-skill.loaf-quarantine-deadbeefcafebabe")
		writeInstallFile(t, filepath.Join(orphan, "SKILL.md"), "# Stranded quarantine\n")

		// legacy-v1: pre-v2 name-only claim with a present tree (separate home
		// so the v1 seed does not overwrite the canonical digest map).
		legacyHome := filepath.Join(home, ".cursor", "skills")
		mkdirAll(t, legacyHome)
		writeInstallFile(t, filepath.Join(legacyHome, "legacy-skill", "SKILL.md"), "# Legacy v1\n")
		seedManagedSkillsManifestV1(t, legacyHome, []string{"legacy-skill"})

		// relocated: owned skill only at the source.
		relocateFrom := filepath.Join(home, ".old-agents", "skills")
		writeInstallFile(t, filepath.Join(relocateFrom, loafInstallMarkerFile), "old\n")
		seedOwnedManagedSkill(t, relocateFrom, "move-me", "# Move me\n")

		// removed-stale: source matches an already-owned destination copy.
		// Separate home so movedAny cannot collapse this into "relocated".
		staleFrom := filepath.Join(home, ".stale-agents", "skills")
		writeInstallFile(t, filepath.Join(staleFrom, loafInstallMarkerFile), "old\n")
		sameBody := "# Already at dest\n"
		seedOwnedManagedSkill(t, staleFrom, "stale-equiv", sameBody)
		seedOwnedManagedSkill(t, skillHome, "stale-equiv", sameBody)

		writeInstallDeprecationManifest(t, root, `{
  "version": 1,
  "retired_targets": [],
  "retired_skills": [
    {
      "skill": "ghost-skill",
      "since": "v9.9.0",
      "reason": "unmanaged-missing must still print",
      "skill_homes": ["${HOME}/.agents/skills"]
    },
    {
      "skill": "legacy-skill",
      "since": "v9.9.0",
      "reason": "legacy-v1 must still print",
      "skill_homes": ["${HOME}/.cursor/skills"]
    },
    {
      "skill": "dangling-skill",
      "since": "v9.9.0",
      "reason": "dangling must still print",
      "skill_homes": ["${HOME}/.agents/skills"]
    }
  ],
  "retired_agents": [],
  "relocations": [
    {
      "id": "old-agents-skills",
      "from": "${HOME}/.old-agents/skills",
      "to": "${HOME}/.agents/skills",
      "since": "v9.9.0",
      "reason": "relocated must still print"
    },
    {
      "id": "stale-agents-skills",
      "from": "${HOME}/.stale-agents/skills",
      "to": "${HOME}/.agents/skills",
      "since": "v9.9.0",
      "reason": "removed-stale must still print"
    }
  ],
  "aliases": []
}`)

		var stdout bytes.Buffer
		if err := (Runner{Stdout: &stdout, WorkingDir: root, Executable: distributionFixtureExecutable(root)}).Run([]string{"upgrade", "--yes"}); err != nil {
			t.Fatalf("upgrade --yes error = %v\n%s", err, stdout.String())
		}
		out := stdout.String()

		// Needles bind action verb templates to fixture identities so one
		// action's rendered line cannot satisfy another's assertion. Shared
		// vocabulary (relocated/removed/stale/unmanaged/missing) makes bare
		// substrings unsafe — e.g. "relocated" alone is a substring of
		// "removed stale relocated".
		required := []struct {
			action string
			needle string
		}{
			{"unmanaged-missing", "un-managed missing skill ghost-skill"},
			{"legacy-v1", "un-managed legacy-v1 skill legacy-skill"},
			{"quarantine-orphan", "quarantine orphan skill orphan-skill"},
			{"dangling", "un-managed dangling skill dangling-skill"},
			{"relocated", "relocated path old-agents-skills"},
			{"removed-stale", "removed stale relocated path stale-agents-skills"},
		}
		for _, want := range required {
			if !strings.Contains(out, want.needle) {
				t.Fatalf("TASK-006 action %q must still print (needle %q); stdout:\n%s", want.action, want.needle, out)
			}
		}
	})
}
