package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSingleCanonicalWrite(t *testing.T) {
	targets := []string{"opencode", "cursor", "codex", "amp"}
	orderings := [][]string{
		{"opencode", "cursor", "codex", "amp"},
		{"amp", "codex", "cursor", "opencode"},
		{"cursor", "amp", "opencode", "codex"},
	}
	sharedBody := canonicalSkillFixtureBody(false)
	opencodeBody := canonicalSkillFixtureBody(true)

	t.Run("one-planned-and-executed-write", func(t *testing.T) {
		root, home := setupInstallCommandFixture(t)
		seedCanonicalSkillsFixture(t, root, home, targets, sharedBody)

		managedSkillsSyncCalls.Store(0)
		runInstallFixture(t, root, "install", "--to", "all", "--yes")
		if got := managedSkillsSyncCalls.Load(); got != 1 {
			t.Fatalf("managedSkillsSyncCalls = %d, want 1 for install --to all", got)
		}
		assertInstallFile(t, filepath.Join(home, ".agents", "skills", "foundations", "SKILL.md"), sharedBody)

		// Mark all four installed, then dry-run the shared store once.
		plan := parseInstallPlanJSON(t, runInstallCapture(t, root, "upgrade", "--to", "all", "--dry-run", "--json"))
		if got := countPlanSkillDecisions(plan, "foundations"); got != 1 {
			t.Fatalf("planned foundations decisions = %d, want 1; skills=%#v", got, plan.Skills)
		}
		for _, target := range plan.Targets {
			for _, artifact := range target.Artifacts {
				if artifact.Kind == "skill" || strings.HasPrefix(artifact.ID, "skill:") {
					t.Fatalf("target %q still carries skill artifact %#v; skills must be plan-level", target.Target, artifact)
				}
			}
		}
	})

	t.Run("owned-key-membership-diff-succeeds", func(t *testing.T) {
		root, home := setupInstallCommandFixture(t)
		for _, target := range targets {
			body := sharedBody
			if target == "opencode" {
				body = opencodeBody
			}
			writeInstallFile(t, filepath.Join(root, "dist", target, "skills", "foundations", "SKILL.md"), body)
			if target == "cursor" || target == "codex" {
				installTestHookDistribution(t, root, target)
			}
			mkdirAll(t, defaultInstallConfigDirsForHome(home)[target])
		}

		managedSkillsSyncCalls.Store(0)
		runInstallFixture(t, root, "install", "--to", "all", "--yes")
		if got := managedSkillsSyncCalls.Load(); got != 1 {
			t.Fatalf("managedSkillsSyncCalls = %d, want 1 when only owned-key membership differs", got)
		}
		assertInstallFile(t, filepath.Join(home, ".agents", "skills", "foundations", "SKILL.md"), opencodeBody)
	})

	t.Run("one-conflict-report", func(t *testing.T) {
		root, home := setupInstallCommandFixture(t)
		// Install without orchestration so the later foreign directory is unowned.
		for _, target := range targets {
			writeInstallFile(t, filepath.Join(root, "dist", target, "skills", "foundations", "SKILL.md"), sharedBody)
			if target == "cursor" || target == "codex" {
				installTestHookDistribution(t, root, target)
			}
			// Seed adapter artifacts so upgrade has something concrete to refresh.
			if target == "opencode" {
				writeInstallFile(t, filepath.Join(root, "dist", target, "commands", "foundations.md"), "# Cmd\nSee [guide](references/guide.md).\n")
				writeInstallFile(t, filepath.Join(root, "dist", target, "agents", "implementer.md"), "# Implementer\n")
			}
			mkdirAll(t, defaultInstallConfigDirsForHome(home)[target])
		}
		runInstallFixture(t, root, "install", "--to", "all", "--yes")
		for _, target := range targets {
			writeInstallFile(t, filepath.Join(defaultInstallConfigDirsForHome(home)[target], loafInstallMarkerFile), "old\n")
		}

		sharedSkills := filepath.Join(home, ".agents", "skills")
		writeInstallFile(t, filepath.Join(sharedSkills, "orchestration", "SKILL.md"), "# Foreign orchestration\n")
		for _, target := range targets {
			writeInstallFile(t, filepath.Join(root, "dist", target, "skills", "orchestration", "SKILL.md"), canonicalOrchestrationFixtureBody())
		}

		plan := parseInstallPlanJSON(t, runInstallCapture(t, root, "upgrade", "--to", "all", "--dry-run", "--json"))
		conflicts := 0
		foundationsAction := ""
		for _, artifact := range plan.Skills {
			if artifact.ID == "skill:orchestration" && artifact.Action == planActionConflict {
				conflicts++
			}
			if artifact.ID == "skill:foundations" {
				foundationsAction = artifact.Action
			}
		}
		if conflicts != 1 {
			t.Fatalf("orchestration conflicts = %d, want 1; skills=%#v", conflicts, plan.Skills)
		}
		// Foundations is already current; conflict on orchestration must not
		// rewrite the plan into an indeterminate preserve/update either-or.
		if foundationsAction != planActionPreserve {
			t.Fatalf("foundations action = %q, want preserve while orchestration conflicts", foundationsAction)
		}
		// Skill conflicts leave the shared set incomplete but do not block
		// target adapter work — plan must match apply.
		for _, target := range plan.Targets {
			if target.Blocked {
				t.Fatalf("target %q Blocked = true, want false for conflict-only skill errors (adapters still run)", target.Target)
			}
		}

		var stdout bytes.Buffer
		err := Runner{Stdout: &stdout, WorkingDir: root, Executable: distributionFixtureExecutable(root)}.Run([]string{"upgrade", "--to", "all", "--yes"})
		if err == nil {
			t.Fatalf("upgrade with orchestration conflict error = nil, want non-zero conflict failure\n%s", stdout.String())
		}
		out := stdout.String()
		if strings.Count(out, "orchestration") < 1 {
			t.Fatalf("upgrade stdout missing orchestration conflict report:\n%s", out)
		}
		if strings.Count(out, "managed skills sync conflicts") != 1 {
			t.Fatalf("conflict report count = %d, want exactly 1\n%s", strings.Count(out, "managed skills sync conflicts"), out)
		}
		for _, target := range targets {
			config := defaultInstallConfigDirsForHome(home)[target]
			assertInstallFile(t, filepath.Join(config, loafInstallMarkerFile), "9.8.7-test.1\n")
			recordPath := installRecordPath(home, target)
			if _, statErr := os.Stat(recordPath); statErr != nil {
				t.Fatalf("install record for %s missing after conflicted upgrade: %v", target, statErr)
			}
			if !strings.Contains(out, installDisplayName(target)) {
				t.Fatalf("upgrade stdout missing refreshed target %q:\n%s", target, out)
			}
		}
		opencodeCommands := filepath.Join(defaultInstallConfigDirsForHome(home)["opencode"], "commands", "foundations.md")
		body, readErr := os.ReadFile(opencodeCommands)
		if readErr != nil {
			t.Fatalf("ReadFile(%s) error = %v", opencodeCommands, readErr)
		}
		if strings.Contains(string(body), "](references/") {
			t.Fatalf("opencode command still has unrewritten skill-local links:\n%s", body)
		}
		assertInstallFile(t, filepath.Join(sharedSkills, "foundations", "SKILL.md"), sharedBody)
		assertInstallFile(t, filepath.Join(sharedSkills, "orchestration", "SKILL.md"), "# Foreign orchestration\n")

		// Second run: unowned collision stays unowned; foundations stays managed;
		// no oscillation into claiming orchestration.
		state, stateErr := readManagedSkillsState(sharedSkills)
		if stateErr != nil {
			t.Fatalf("readManagedSkillsState error = %v", stateErr)
		}
		if _, ok := state.digests["orchestration"]; ok {
			t.Fatalf("manifest gained unowned orchestration after conflicted upgrade")
		}
		if _, ok := state.digests["foundations"]; !ok {
			t.Fatalf("manifest dropped foundations after conflicted upgrade")
		}
		var stdout2 bytes.Buffer
		err2 := Runner{Stdout: &stdout2, WorkingDir: root, Executable: distributionFixtureExecutable(root)}.Run([]string{"upgrade", "--to", "all", "--yes"})
		if err2 == nil {
			t.Fatalf("second upgrade error = nil, want persistent orchestration conflict\n%s", stdout2.String())
		}
		state2, stateErr := readManagedSkillsState(sharedSkills)
		if stateErr != nil {
			t.Fatalf("readManagedSkillsState(second) error = %v", stateErr)
		}
		if _, ok := state2.digests["orchestration"]; ok {
			t.Fatalf("second upgrade granted ownership of unowned orchestration")
		}
		assertInstallFile(t, filepath.Join(sharedSkills, "orchestration", "SKILL.md"), "# Foreign orchestration\n")
	})

	t.Run("order-invariant-destination-bytes", func(t *testing.T) {
		var digests []string
		for _, order := range orderings {
			root, home := setupInstallCommandFixture(t)
			seedCanonicalSkillsFixture(t, root, home, targets, sharedBody)
			// Distinct bodies per target would have made last-writer-wins visible;
			// identical trees prove the destination is stable across orderings.
			managedSkillsSyncCalls.Store(0)
			opts := make([]targetInstallOptions, 0, len(order))
			for _, target := range order {
				opts = append(opts, targetInstallOptions{
					Target:         target,
					DistDir:        filepath.Join(root, "dist", target),
					ConfigDir:      defaultInstallConfigDirs()[target],
					Version:        "9.8.7-test.1",
					HomeDir:        home,
					SkipSkillsSync: true,
					HookState:      installTestHookState(t),
				})
			}
			if err := syncCanonicalManagedSkills(opts); err != nil {
				t.Fatalf("syncCanonicalManagedSkills(%v) error = %v", order, err)
			}
			if got := managedSkillsSyncCalls.Load(); got != 1 {
				t.Fatalf("order %v: managedSkillsSyncCalls = %d, want 1", order, got)
			}
			for _, opt := range opts {
				if err := installTargetDistribution(opt); err != nil {
					t.Fatalf("installTargetDistribution(%q) error = %v", opt.Target, err)
				}
			}
			digest, err := hashInstallSkillTree(filepath.Join(home, ".agents", "skills"))
			if err != nil {
				t.Fatalf("hash destination after %v: %v", order, err)
			}
			digests = append(digests, digest)
			assertInstallFile(t, filepath.Join(home, ".agents", "skills", "foundations", "SKILL.md"), sharedBody)
		}
		for i := 1; i < len(digests); i++ {
			if digests[i] != digests[0] {
				t.Fatalf("destination digest for ordering %v = %s, want %s (same as %v)", orderings[i], digests[i], digests[0], orderings[0])
			}
		}
	})

	t.Run("divergent-trees-fail-loudly", func(t *testing.T) {
		root, home := setupInstallCommandFixture(t)
		seedCanonicalSkillsFixture(t, root, home, targets, sharedBody)
		writeInstallFile(t, filepath.Join(root, "dist", "cursor", "skills", "foundations", "SKILL.md"), strings.Replace(sharedBody, "# Shared foundations\n", "# Cursor-only flavor\n", 1))

		err := syncCanonicalManagedSkills([]targetInstallOptions{
			{Target: "opencode", DistDir: filepath.Join(root, "dist", "opencode"), ConfigDir: filepath.Join(home, ".config", "opencode"), HomeDir: home},
			{Target: "cursor", DistDir: filepath.Join(root, "dist", "cursor"), ConfigDir: filepath.Join(home, ".cursor"), HomeDir: home},
		})
		if err == nil || !strings.Contains(err.Error(), "skills trees diverge") || !strings.Contains(err.Error(), "cursor") || !strings.Contains(err.Error(), "opencode") {
			t.Fatalf("divergence error = %v, want named targets", err)
		}
	})

	t.Run("real-checked-in-dist-trees", func(t *testing.T) {
		repo := testRepositoryRoot(t)
		for _, target := range targets {
			if _, err := os.Stat(filepath.Join(repo, "dist", target, "skills")); err != nil {
				t.Skipf("checked-in dist/%s/skills unavailable: %v", target, err)
			}
		}
		home := t.TempDir()
		managedSkillsSyncCalls.Store(0)
		opts := make([]targetInstallOptions, 0, len(targets))
		for _, target := range targets {
			opts = append(opts, targetInstallOptions{
				Target:    target,
				DistDir:   filepath.Join(repo, "dist", target),
				ConfigDir: filepath.Join(home, "config", target),
				HomeDir:   home,
			})
		}
		if err := syncCanonicalManagedSkills(opts); err != nil {
			t.Fatalf("syncCanonicalManagedSkills(real dist) error = %v", err)
		}
		if got := managedSkillsSyncCalls.Load(); got != 1 {
			t.Fatalf("managedSkillsSyncCalls = %d, want 1 for real dist --to all shape", got)
		}
		research := filepath.Join(home, ".agents", "skills", "research", "SKILL.md")
		body, err := os.ReadFile(research)
		if err != nil {
			t.Fatalf("ReadFile(%s) error = %v", research, err)
		}
		if !strings.Contains(string(body), "subtask: false") {
			t.Fatalf("canonical store research SKILL.md missing OpenCode-owned subtask; got:\n%s", body)
		}
	})

	// Documented behaviour, not a security property: dist/ is trusted input and
	// source sidecars are not shipped, so owned-key *values* cannot be authorized
	// at install time. Membership strip treats subtask:true the same as
	// subtask:false; OpenCode remains canonical and the tampered value lands.
	t.Run("tampered-owned-key-value-installs-as-trusted-dist", func(t *testing.T) {
		root, home := setupInstallCommandFixture(t)
		tampered := strings.Replace(opencodeBody, "subtask: false\n", "subtask: true\n", 1)
		if tampered == opencodeBody {
			t.Fatal("fixture rewrite failed; expected subtask: false in opencodeBody")
		}
		for _, target := range targets {
			body := sharedBody
			if target == "opencode" {
				body = tampered
			}
			writeInstallFile(t, filepath.Join(root, "dist", target, "skills", "foundations", "SKILL.md"), body)
			if target == "cursor" || target == "codex" {
				installTestHookDistribution(t, root, target)
			}
			mkdirAll(t, defaultInstallConfigDirsForHome(home)[target])
		}

		managedSkillsSyncCalls.Store(0)
		runInstallFixture(t, root, "install", "--to", "all", "--yes")
		if got := managedSkillsSyncCalls.Load(); got != 1 {
			t.Fatalf("managedSkillsSyncCalls = %d, want 1 for tampered owned-key value", got)
		}
		assertInstallFile(t, filepath.Join(home, ".agents", "skills", "foundations", "SKILL.md"), tampered)
	})

	t.Run("symlinked-skills-root-rejected", func(t *testing.T) {
		root, home := setupInstallCommandFixture(t)
		seedCanonicalSkillsFixture(t, root, home, targets, sharedBody)
		external := t.TempDir()
		writeInstallFile(t, filepath.Join(external, "foundations", "SKILL.md"), sharedBody)
		cursorSkills := filepath.Join(root, "dist", "cursor", "skills")
		if err := os.RemoveAll(cursorSkills); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(external, cursorSkills); err != nil {
			t.Fatal(err)
		}
		err := syncCanonicalManagedSkills([]targetInstallOptions{
			{Target: "opencode", DistDir: filepath.Join(root, "dist", "opencode"), ConfigDir: filepath.Join(home, ".config", "opencode"), HomeDir: home},
			{Target: "cursor", DistDir: filepath.Join(root, "dist", "cursor"), ConfigDir: filepath.Join(home, ".cursor"), HomeDir: home},
		})
		if err == nil || !strings.Contains(err.Error(), "not a directory or is a symlink") {
			t.Fatalf("symlinked skills root error = %v, want root rejection", err)
		}
	})

	t.Run("symlink-inside-skill-directory-rejected", func(t *testing.T) {
		root, home := setupInstallCommandFixture(t)
		seedCanonicalSkillsFixture(t, root, home, targets, sharedBody)
		skillDir := filepath.Join(root, "dist", "cursor", "skills", "foundations")
		if err := os.Symlink("SKILL.md", filepath.Join(skillDir, "link.md")); err != nil {
			t.Fatal(err)
		}
		err := syncCanonicalManagedSkills([]targetInstallOptions{
			{Target: "opencode", DistDir: filepath.Join(root, "dist", "opencode"), ConfigDir: filepath.Join(home, ".config", "opencode"), HomeDir: home},
			{Target: "cursor", DistDir: filepath.Join(root, "dist", "cursor"), ConfigDir: filepath.Join(home, ".cursor"), HomeDir: home},
		})
		if err == nil || !strings.Contains(err.Error(), "contains symlink") {
			t.Fatalf("in-tree symlink error = %v, want contains symlink", err)
		}
	})

	// dist/<target> itself is a symlink to an external tree. Skills under that
	// tree are ordinary directories/files, so a leaf-only Lstat of .../skills misses
	// the escape. Both multi-target verify and single-target sync must refuse.
	t.Run("symlinked-dist-ancestor-rejected", func(t *testing.T) {
		root, home := setupInstallCommandFixture(t)
		seedCanonicalSkillsFixture(t, root, home, targets, sharedBody)
		external := t.TempDir()
		writeInstallFile(t, filepath.Join(external, "skills", "foundations", "SKILL.md"), sharedBody)
		cursorDist := filepath.Join(root, "dist", "cursor")
		if err := os.RemoveAll(cursorDist); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(external, cursorDist); err != nil {
			t.Fatal(err)
		}

		multiErr := syncCanonicalManagedSkills([]targetInstallOptions{
			{Target: "opencode", DistDir: filepath.Join(root, "dist", "opencode"), ConfigDir: filepath.Join(home, ".config", "opencode"), HomeDir: home},
			{Target: "cursor", DistDir: cursorDist, ConfigDir: filepath.Join(home, ".cursor"), HomeDir: home},
		})
		if multiErr == nil || !strings.Contains(multiErr.Error(), "not a directory or is a symlink") {
			t.Fatalf("multi-target symlinked DistDir error = %v, want root rejection", multiErr)
		}

		singleErr := syncCanonicalManagedSkills([]targetInstallOptions{
			{Target: "cursor", DistDir: cursorDist, ConfigDir: filepath.Join(home, ".cursor"), HomeDir: home},
		})
		if singleErr == nil || !strings.Contains(singleErr.Error(), "not a directory or is a symlink") {
			t.Fatalf("single-target symlinked DistDir error = %v, want root rejection", singleErr)
		}
	})

	// dist/ itself is the symlink; dist/<target> and dist/<target>/skills beneath
	// it are ordinary directories. Lstat of DistDir and skills alone both pass —
	// the class residual round 4 left open. Refuse on both multi- and single-target.
	t.Run("symlinked-dist-root-rejected", func(t *testing.T) {
		root, home := setupInstallCommandFixture(t)
		seedCanonicalSkillsFixture(t, root, home, targets, sharedBody)
		realDist := filepath.Join(root, "dist")
		externalDist := filepath.Join(t.TempDir(), "dist-contents")
		if err := os.Rename(realDist, externalDist); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(externalDist, realDist); err != nil {
			t.Fatal(err)
		}

		multiErr := syncCanonicalManagedSkills([]targetInstallOptions{
			{Target: "opencode", DistDir: filepath.Join(root, "dist", "opencode"), ConfigDir: filepath.Join(home, ".config", "opencode"), HomeDir: home},
			{Target: "cursor", DistDir: filepath.Join(root, "dist", "cursor"), ConfigDir: filepath.Join(home, ".cursor"), HomeDir: home},
		})
		if multiErr == nil || !strings.Contains(multiErr.Error(), "not a directory or is a symlink") {
			t.Fatalf("multi-target symlinked dist/ error = %v, want root rejection", multiErr)
		}

		singleErr := syncCanonicalManagedSkills([]targetInstallOptions{
			{Target: "cursor", DistDir: filepath.Join(root, "dist", "cursor"), ConfigDir: filepath.Join(home, ".cursor"), HomeDir: home},
		})
		if singleErr == nil || !strings.Contains(singleErr.Error(), "not a directory or is a symlink") {
			t.Fatalf("single-target symlinked dist/ error = %v, want root rejection", singleErr)
		}
	})

	// Ancestors *above* the distribution root (dist/) may be symlinks (macOS
	// /tmp → /private/tmp, or a checkout reached through a linked prefix). That
	// must still install.
	t.Run("symlinked-prefix-above-dist-still-installs", func(t *testing.T) {
		base := t.TempDir()
		realRoot := filepath.Join(base, "real-repo")
		linkRoot := filepath.Join(base, "link-repo")
		if err := os.MkdirAll(realRoot, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(realRoot, linkRoot); err != nil {
			t.Fatal(err)
		}
		home := filepath.Join(base, "home")
		if err := os.MkdirAll(home, 0o755); err != nil {
			t.Fatal(err)
		}
		t.Setenv("HOME", home)
		t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
		seedCanonicalSkillsFixture(t, linkRoot, home, targets, sharedBody)

		managedSkillsSyncCalls.Store(0)
		err := syncCanonicalManagedSkills([]targetInstallOptions{
			{Target: "opencode", DistDir: filepath.Join(linkRoot, "dist", "opencode"), ConfigDir: filepath.Join(home, ".config", "opencode"), HomeDir: home},
			{Target: "cursor", DistDir: filepath.Join(linkRoot, "dist", "cursor"), ConfigDir: filepath.Join(home, ".cursor"), HomeDir: home},
		})
		if err != nil {
			t.Fatalf("install through symlinked prefix above DistDir error = %v", err)
		}
		if got := managedSkillsSyncCalls.Load(); got != 1 {
			t.Fatalf("managedSkillsSyncCalls = %d, want 1 through symlinked prefix", got)
		}
		assertInstallFile(t, filepath.Join(home, ".agents", "skills", "foundations", "SKILL.md"), sharedBody)
	})
}

func TestStoreSharingOwnedSidecarKeysAtMostOne(t *testing.T) {
	var owners []string
	for _, target := range installValidTargets {
		if len(nativeBuildSidecarOwnedFrontmatterKeysByTarget[target]) > 0 {
			owners = append(owners, target)
		}
	}
	if len(owners) > 1 {
		t.Fatalf("store-sharing targets with owned sidecar keys = %v; canonical skills store can carry only one frontmatter shape — resolve the design conflict before adding a second owner", owners)
	}
}

func TestSelectCanonicalSkillsSourceWithOwnedKeys(t *testing.T) {
	opts := []targetInstallOptions{
		{Target: "amp", DistDir: "/tmp/amp"},
		{Target: "codex", DistDir: "/tmp/codex"},
		{Target: "cursor", DistDir: "/tmp/cursor"},
		{Target: "opencode", DistDir: "/tmp/opencode"},
	}

	t.Run("exactly-one-owner", func(t *testing.T) {
		got, err := selectCanonicalSkillsSourceWithOwnedKeys(opts, map[string]map[string]bool{
			"opencode": {"subtask": true},
		})
		if err != nil {
			t.Fatalf("error = %v", err)
		}
		if got.Target != "opencode" {
			t.Fatalf("target = %q, want opencode", got.Target)
		}
	})

	t.Run("no-owners-picks-first", func(t *testing.T) {
		got, err := selectCanonicalSkillsSourceWithOwnedKeys(opts, map[string]map[string]bool{})
		if err != nil {
			t.Fatalf("error = %v", err)
		}
		if got.Target != "amp" {
			t.Fatalf("target = %q, want amp (alphabetical first)", got.Target)
		}
	})

	t.Run("multiple-owners-fail", func(t *testing.T) {
		_, err := selectCanonicalSkillsSourceWithOwnedKeys(opts, map[string]map[string]bool{
			"cursor":   {"globs": true},
			"opencode": {"subtask": true},
		})
		if err == nil || !strings.Contains(err.Error(), "cursor") || !strings.Contains(err.Error(), "opencode") || !strings.Contains(err.Error(), "globs") || !strings.Contains(err.Error(), "subtask") {
			t.Fatalf("error = %v, want named conflicting targets and keys", err)
		}
	})
}

func canonicalSkillFixtureBody(withSubtask bool) string {
	var b strings.Builder
	b.WriteString("---\n")
	b.WriteString("name: foundations\n")
	b.WriteString("description: Shared\n")
	if withSubtask {
		b.WriteString("subtask: false\n")
	}
	b.WriteString("version: 9.8.7-test.1\n")
	b.WriteString("---\n\n")
	b.WriteString("# Shared foundations\n")
	return b.String()
}

func canonicalOrchestrationFixtureBody() string {
	return "---\nname: orchestration\ndescription: Loaf orchestration\nversion: 9.8.7-test.1\n---\n\n# Loaf orchestration\n"
}

func seedCanonicalSkillsFixture(t *testing.T, root, home string, targets []string, body string) {
	t.Helper()
	for _, target := range targets {
		writeInstallFile(t, filepath.Join(root, "dist", target, "skills", "foundations", "SKILL.md"), body)
		if target == "cursor" || target == "codex" {
			installTestHookDistribution(t, root, target)
		}
		// Ensure --to all detects every target without relying on host CLIs.
		mkdirAll(t, defaultInstallConfigDirsForHome(home)[target])
	}
}

func defaultInstallConfigDirsForHome(home string) map[string]string {
	xdgConfig := filepath.Join(home, ".config")
	return map[string]string{
		"opencode": filepath.Join(xdgConfig, "opencode"),
		"cursor":   filepath.Join(home, ".cursor"),
		"codex":    filepath.Join(home, ".codex"),
		"amp":      filepath.Join(xdgConfig, "amp"),
	}
}

func countPlanSkillDecisions(plan installDryRunPlan, skill string) int {
	count := 0
	for _, artifact := range plan.Skills {
		if artifact.ID == "skill:"+skill {
			count++
		}
	}
	return count
}
