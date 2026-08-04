package cli

import (
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

func TestSkillConflictIsolation(t *testing.T) {
	root := t.TempDir()
	home := filepath.Join(root, "home")
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	t.Setenv("LOAF_DB", filepath.Join(t.TempDir(), "loaf.sqlite"))

	src := filepath.Join(root, "dist", "opencode", "skills")
	dest := filepath.Join(home, ".agents", "skills")
	writeInstallFile(t, filepath.Join(src, "alpha", "SKILL.md"), "# Alpha\n")
	writeInstallFile(t, filepath.Join(src, "beta", "SKILL.md"), "# Beta\n")
	writeInstallFile(t, filepath.Join(src, "gamma", "SKILL.md"), "# Gamma desired\n")
	// Foreign/unowned collision: gamma exists and was never managed by Loaf.
	writeInstallFile(t, filepath.Join(dest, "gamma", "SKILL.md"), "# Foreign gamma\n")

	plan, err := planManagedSkills(src, dest)
	if err != nil {
		t.Fatalf("planManagedSkills error = %v", err)
	}
	planConflicts := map[string]string{}
	for _, decision := range plan {
		switch decision.ID {
		case "skill:gamma":
			if decision.Action != planActionConflict {
				t.Fatalf("plan gamma action = %q, want conflict", decision.Action)
			}
			planConflicts["gamma"] = decision.Detail
		case "skill:alpha":
			if decision.Action != planActionCreate {
				t.Fatalf("plan alpha action = %q, want create", decision.Action)
			}
		case "skill:beta":
			if decision.Action != planActionCreate {
				t.Fatalf("plan beta action = %q, want create", decision.Action)
			}
		}
	}
	if len(planConflicts) != 1 || planConflicts["gamma"] == "" {
		t.Fatalf("plan conflicts = %#v, want gamma once", planConflicts)
	}
	if !strings.Contains(planConflicts["gamma"], "not managed") {
		t.Fatalf("plan gamma detail = %q, want unowned/not-managed reason", planConflicts["gamma"])
	}

	err = syncManagedSkillsDirIfExists(src, dest)
	var conflicts *skillSyncConflictsError
	if !errors.As(err, &conflicts) {
		t.Fatalf("syncManagedSkillsDirIfExists error = %v, want skillSyncConflictsError", err)
	}
	if len(conflicts.Conflicts) != 1 || conflicts.Conflicts[0].Skill != "gamma" {
		t.Fatalf("apply conflicts = %#v, want gamma once", conflicts.Conflicts)
	}
	if !strings.Contains(conflicts.Conflicts[0].Reason, "not managed") {
		t.Fatalf("apply gamma reason = %q, want unowned/not-managed", conflicts.Conflicts[0].Reason)
	}
	if !strings.Contains(err.Error(), "gamma") || !strings.Contains(err.Error(), "not managed") {
		t.Fatalf("error text = %q, want gamma named with not-managed reason", err)
	}

	assertInstallFile(t, filepath.Join(dest, "alpha", "SKILL.md"), "# Alpha\n")
	assertInstallFile(t, filepath.Join(dest, "beta", "SKILL.md"), "# Beta\n")
	assertInstallFile(t, filepath.Join(dest, "gamma", "SKILL.md"), "# Foreign gamma\n")

	state, err := readManagedSkillsState(dest)
	if err != nil {
		t.Fatalf("readManagedSkillsState error = %v", err)
	}
	if _, ok := state.digests["gamma"]; ok {
		t.Fatalf("manifest retained unowned gamma; unowned collisions must not gain an entry")
	}
	if _, ok := state.digests["alpha"]; !ok {
		t.Fatalf("manifest missing alpha after partial success")
	}
	if _, ok := state.digests["beta"]; !ok {
		t.Fatalf("manifest missing beta after partial success")
	}
}

func TestGeneratedCommandLinksResolve(t *testing.T) {
	repo := testRepositoryRoot(t)
	home := t.TempDir()
	// Non-default config depth: not ~/.config/opencode.
	configHome := filepath.Join(home, "xdg", "nested", "config")
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("XDG_CONFIG_HOME", configHome)
	t.Setenv("LOAF_DB", filepath.Join(t.TempDir(), "loaf.sqlite"))

	distCommands := filepath.Join(repo, "dist", "opencode", "commands")
	if _, err := os.Stat(distCommands); err != nil {
		t.Skipf("checked-in dist/opencode/commands unavailable: %v", err)
	}
	skillsStore := filepath.Join(home, ".agents", "skills")
	// Seed the canonical skills store from the build output so link targets exist.
	distSkills := filepath.Join(repo, "dist", "opencode", "skills")
	if err := copyDirContentsForInstall(distSkills, skillsStore); err != nil {
		t.Fatalf("seed skills store: %v", err)
	}

	configDir := filepath.Join(configHome, "opencode")
	options := targetInstallOptions{
		Target:         "opencode",
		DistDir:        filepath.Join(repo, "dist", "opencode"),
		ConfigDir:      configDir,
		Version:        "9.8.7-test.1",
		HomeDir:        home,
		SkipSkillsSync: true,
	}
	if err := installTargetDistribution(options); err != nil {
		t.Fatalf("installTargetDistribution error = %v", err)
	}

	commandsDir := filepath.Join(configDir, "commands")
	entries, err := os.ReadDir(commandsDir)
	if err != nil {
		t.Fatalf("ReadDir(%s) error = %v", commandsDir, err)
	}
	linkRE := regexp.MustCompile(`\]\(([^)]+)\)`)
	checked := 0
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}
		skill := strings.TrimSuffix(entry.Name(), ".md")
		body, err := os.ReadFile(filepath.Join(commandsDir, entry.Name()))
		if err != nil {
			t.Fatalf("ReadFile(%s) error = %v", entry.Name(), err)
		}
		text := string(body)
		if strings.Contains(text, "](../skills/") {
			t.Fatalf("%s still contains build-time ../skills/ links; install must rewrite them", entry.Name())
		}
		if strings.Contains(text, "](templates/") || strings.Contains(text, "](references/") {
			t.Fatalf("%s still contains unrewritten skill-local template/reference links", entry.Name())
		}
		skillRoot := filepath.Join(skillsStore, skill)
		rel, err := filepath.Rel(commandsDir, skillRoot)
		if err != nil {
			t.Fatalf("filepath.Rel(%s, %s) error = %v", commandsDir, skillRoot, err)
		}
		linkBase := filepath.ToSlash(rel)
		templatesPrefix := linkBase + "/templates/"
		referencesPrefix := linkBase + "/references/"
		for _, match := range linkRE.FindAllStringSubmatch(text, -1) {
			target := match[1]
			if !(strings.HasPrefix(target, templatesPrefix) || strings.HasPrefix(target, referencesPrefix)) {
				continue
			}
			checked++
			resolved := target
			if !filepath.IsAbs(target) {
				resolved = filepath.Join(commandsDir, filepath.FromSlash(target))
			}
			if _, err := os.Stat(resolved); err != nil {
				t.Fatalf("%s rewritten link %q (skill %s) does not resolve from %s: %v", entry.Name(), target, skill, commandsDir, err)
			}
		}
	}
	if checked == 0 {
		t.Fatal("no rewritten template/reference links found in installed commands")
	}
}

func TestGeneratedCommandLinksResolveThroughSymlinkedConfig(t *testing.T) {
	base := t.TempDir()
	home := filepath.Join(base, "home")
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	t.Setenv("LOAF_DB", filepath.Join(t.TempDir(), "loaf.sqlite"))

	// Target at a different depth than the link so lexical Rel diverges from
	// resolved Rel — sibling-depth symlinks hide the bug.
	realConfig := filepath.Join(base, "deep", "nested", "real-opencode")
	linkConfig := filepath.Join(base, "link-opencode")
	if err := os.MkdirAll(realConfig, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(realConfig, linkConfig); err != nil {
		t.Fatal(err)
	}

	skillsStore := filepath.Join(home, ".agents", "skills")
	writeInstallFile(t, filepath.Join(skillsStore, "foundations", "references", "guide.md"), "# Guide\n")

	dist := filepath.Join(base, "dist", "opencode")
	writeInstallFile(t, filepath.Join(dist, "commands", "foundations.md"), "# Foundations\nSee [guide](references/guide.md).\n")

	if err := syncOpenCodeCommandsDir(dist, linkConfig, skillsStore); err != nil {
		t.Fatalf("syncOpenCodeCommandsDir through symlink error = %v", err)
	}

	body, err := os.ReadFile(filepath.Join(realConfig, "commands", "foundations.md"))
	if err != nil {
		t.Fatalf("ReadFile(real commands) error = %v", err)
	}
	text := string(body)
	if strings.Contains(text, "](references/") || strings.Contains(text, "](../skills/") {
		t.Fatalf("command still has unrewritten links:\n%s", text)
	}
	linkRE := regexp.MustCompile(`\]\(([^)]+)\)`)
	match := linkRE.FindStringSubmatch(text)
	if match == nil {
		t.Fatalf("no markdown link in rewritten command:\n%s", text)
	}
	target := match[1]
	commandsDir := filepath.Join(realConfig, "commands")
	resolved := target
	if !filepath.IsAbs(target) {
		resolved = filepath.Join(commandsDir, filepath.FromSlash(target))
	}
	if _, err := os.Stat(resolved); err != nil {
		t.Fatalf("rewritten link %q does not resolve from physical commands dir %s: %v\nbody:\n%s", target, commandsDir, err, text)
	}
	lexicalRel, err := filepath.Rel(filepath.Join(linkConfig, "commands"), filepath.Join(skillsStore, "foundations"))
	if err != nil {
		t.Fatalf("lexical Rel error = %v", err)
	}
	resolvedCommands, err := filepath.EvalSymlinks(commandsDir)
	if err != nil {
		t.Fatalf("EvalSymlinks(commands) error = %v", err)
	}
	resolvedStore, err := filepath.EvalSymlinks(skillsStore)
	if err != nil {
		t.Fatalf("EvalSymlinks(skills) error = %v", err)
	}
	wantRel, err := filepath.Rel(resolvedCommands, filepath.Join(resolvedStore, "foundations"))
	if err != nil {
		t.Fatalf("resolved Rel error = %v", err)
	}
	wantPrefix := filepath.ToSlash(wantRel) + "/references/"
	if !strings.HasPrefix(target, wantPrefix) {
		t.Fatalf("rewritten link = %q, want prefix %q (resolved); lexical would have been %q", target, wantPrefix, filepath.ToSlash(lexicalRel)+"/references/")
	}
	if filepath.Clean(lexicalRel) != filepath.Clean(wantRel) && strings.HasPrefix(target, filepath.ToSlash(lexicalRel)+"/") {
		t.Fatalf("wrote lexical (symlink-blind) link %q; resolved want %q", target, wantPrefix)
	}
}

func TestOpenCodeCommandsRequireSkillsStore(t *testing.T) {
	t.Run("apply-direct", func(t *testing.T) {
		root := t.TempDir()
		home := filepath.Join(root, "home")
		t.Setenv("HOME", home)
		t.Setenv("USERPROFILE", home)
		t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
		t.Setenv("LOAF_DB", filepath.Join(t.TempDir(), "loaf.sqlite"))

		dist := filepath.Join(root, "dist", "opencode")
		config := filepath.Join(home, ".config", "opencode")
		// Damaged/partial distribution: commands present, skills tree absent.
		writeInstallFile(t, filepath.Join(dist, "commands", "foundations.md"), "# Foundations\nSee [guide](references/guide.md).\n")
		writeInstallFile(t, filepath.Join(dist, "agents", "implementer.md"), "# Implementer\n")

		err := installTargetDistribution(targetInstallOptions{
			Target:         "opencode",
			DistDir:        dist,
			ConfigDir:      config,
			Version:        "9.8.7-test.1",
			HomeDir:        home,
			SkipSkillsSync: true,
		})
		if err == nil {
			t.Fatal("installTargetDistribution error = nil, want fail-closed when skills store is missing")
		}
		if !strings.Contains(err.Error(), "skills store") && !strings.Contains(err.Error(), "canonical skills") {
			t.Fatalf("error = %v, want skills-store requirement named", err)
		}
		assertInstallPathMissing(t, filepath.Join(config, "commands", "foundations.md"))
		assertInstallPathMissing(t, filepath.Join(config, loafInstallMarkerFile))
		assertInstallPathMissing(t, installRecordPath(home, "opencode"))
	})

	// Partial dist with commands but no skills tree: dry-run must surface the
	// same fail-closed store guard apply enforces, not look entirely preservable.
	t.Run("plan-apply-parity-missing-store", func(t *testing.T) {
		root, home := setupInstallCommandFixture(t)
		dist := filepath.Join(root, "dist", "opencode")
		config := filepath.Join(home, ".config", "opencode")
		writeInstallFile(t, filepath.Join(dist, "commands", "foundations.md"), "# Foundations\nSee [guide](references/guide.md).\n")
		writeInstallFile(t, filepath.Join(dist, "agents", "implementer.md"), "# Implementer\n")
		writeTestTargetAdapterManifest(t, dist, "opencode", nil)
		mkdirAll(t, config)
		writeInstallFile(t, filepath.Join(config, loafInstallMarkerFile), "old\n")

		opts := targetInstallOptions{
			Target:    "opencode",
			DistDir:   dist,
			ConfigDir: config,
			Version:   "9.8.7-test.1",
			HomeDir:   home,
		}
		decisions, err := planTargetDistribution(opts)
		if err != nil {
			t.Fatalf("planTargetDistribution error = %v", err)
		}
		var storeGuard artifactPlanDecision
		found := false
		for _, decision := range decisions {
			if decision.Action == planActionConflict &&
				(strings.Contains(decision.Detail, "skills store") || strings.Contains(decision.Detail, "canonical skills")) {
				storeGuard = decision
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("plan decisions = %#v, want a conflict naming the missing skills store", decisions)
		}
		if storeGuard.Kind != "commands" && !strings.Contains(storeGuard.ID, "command") {
			t.Fatalf("store-guard decision = %#v, want a commands-kind / command-id artifact", storeGuard)
		}

		plan := assertDryRunNonMutating(t, root, home, "upgrade", "--to", "opencode", "--dry-run", "--json")
		opencode := findTargetPlan(t, plan, "opencode")
		if !opencode.Blocked {
			t.Fatal("opencode plan Blocked = false, want true when commands sync cannot satisfy the skills-store guard")
		}

		applyErr := installTargetDistribution(targetInstallOptions{
			Target:         "opencode",
			DistDir:        dist,
			ConfigDir:      config,
			Version:        "9.8.7-test.1",
			HomeDir:        home,
			SkipSkillsSync: true,
		})
		if applyErr == nil {
			t.Fatal("apply error = nil, want fail-closed when skills store is missing")
		}
		if !strings.Contains(applyErr.Error(), "skills store") && !strings.Contains(applyErr.Error(), "canonical skills") {
			t.Fatalf("apply error = %v, want skills-store requirement named", applyErr)
		}
		if !strings.Contains(storeGuard.Detail, "skills store") && !strings.Contains(storeGuard.Detail, "canonical skills") {
			t.Fatalf("plan detail = %q, want skills-store guard text matching apply", storeGuard.Detail)
		}
		assertInstallPathMissing(t, filepath.Join(config, "commands", "foundations.md"))
	})
}

// TestGeneratedCommandLinksResolveRejectsSymlinkedCommandsSource proves the
// distribution-source symlink guard: a symlinked commands directory or nested
// command symlink is refused before any destination mutation. Distinct from
// TestGeneratedCommandLinksResolveThroughSymlinkedConfig, which covers a
// legitimate symlinked *config destination*.
func TestGeneratedCommandLinksResolveRejectsSymlinkedCommandsSource(t *testing.T) {
	t.Run("symlinked-commands-directory-refused-before-wipe", func(t *testing.T) {
		base := t.TempDir()
		home := filepath.Join(base, "home")
		t.Setenv("HOME", home)
		t.Setenv("USERPROFILE", home)
		t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
		t.Setenv("LOAF_DB", filepath.Join(t.TempDir(), "loaf.sqlite"))

		skillsStore := filepath.Join(home, ".agents", "skills")
		writeInstallFile(t, filepath.Join(skillsStore, "foundations", "SKILL.md"), "# Foundations\n")

		config := filepath.Join(home, ".config", "opencode")
		existing := filepath.Join(config, "commands", "keep-me.md")
		writeInstallFile(t, existing, "# User command that must survive a refused sync\n")

		foreign := filepath.Join(base, "foreign-commands")
		writeInstallFile(t, filepath.Join(foreign, "injected.md"), "# Injected from outside the distribution\n")

		dist := filepath.Join(base, "dist", "opencode")
		mkdirAll(t, dist)
		if err := os.Symlink(foreign, filepath.Join(dist, "commands")); err != nil {
			t.Fatal(err)
		}

		err := syncOpenCodeCommandsDir(dist, config, skillsStore)
		if err == nil {
			t.Fatal("syncOpenCodeCommandsDir error = nil, want refusal of symlinked commands source")
		}
		if !strings.Contains(err.Error(), "symlink") {
			t.Fatalf("error = %v, want symlink refusal", err)
		}
		assertInstallFile(t, existing, "# User command that must survive a refused sync\n")
		assertInstallPathMissing(t, filepath.Join(config, "commands", "injected.md"))
	})

	t.Run("nested-command-symlink-refused-before-wipe", func(t *testing.T) {
		base := t.TempDir()
		home := filepath.Join(base, "home")
		t.Setenv("HOME", home)
		t.Setenv("USERPROFILE", home)
		t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
		t.Setenv("LOAF_DB", filepath.Join(t.TempDir(), "loaf.sqlite"))

		skillsStore := filepath.Join(home, ".agents", "skills")
		writeInstallFile(t, filepath.Join(skillsStore, "foundations", "SKILL.md"), "# Foundations\n")

		config := filepath.Join(home, ".config", "opencode")
		existing := filepath.Join(config, "commands", "keep-me.md")
		writeInstallFile(t, existing, "# User command that must survive a refused sync\n")

		secret := filepath.Join(base, "secret.md")
		writeInstallFile(t, secret, "exfiltrate-me\n")

		dist := filepath.Join(base, "dist", "opencode")
		writeInstallFile(t, filepath.Join(dist, "commands", "foundations.md"), "# Foundations\n")
		mkdirAll(t, filepath.Join(dist, "commands", "group"))
		if err := os.Symlink(secret, filepath.Join(dist, "commands", "group", "secret.md")); err != nil {
			t.Fatal(err)
		}

		err := syncOpenCodeCommandsDir(dist, config, skillsStore)
		if err == nil {
			t.Fatal("syncOpenCodeCommandsDir error = nil, want refusal of nested command symlink")
		}
		if !strings.Contains(err.Error(), "symlink") {
			t.Fatalf("error = %v, want symlink refusal", err)
		}
		assertInstallFile(t, existing, "# User command that must survive a refused sync\n")
		assertInstallPathMissing(t, filepath.Join(config, "commands", "foundations.md"))
		assertInstallPathMissing(t, filepath.Join(config, "commands", "group", "secret.md"))
	})

	t.Run("direct-entry-symlink-refused-before-wipe", func(t *testing.T) {
		base := t.TempDir()
		home := filepath.Join(base, "home")
		t.Setenv("HOME", home)
		t.Setenv("USERPROFILE", home)
		t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
		t.Setenv("LOAF_DB", filepath.Join(t.TempDir(), "loaf.sqlite"))

		skillsStore := filepath.Join(home, ".agents", "skills")
		writeInstallFile(t, filepath.Join(skillsStore, "foundations", "SKILL.md"), "# Foundations\n")

		config := filepath.Join(home, ".config", "opencode")
		existing := filepath.Join(config, "commands", "keep-me.md")
		writeInstallFile(t, existing, "# User command that must survive a refused sync\n")

		secret := filepath.Join(base, "secret.md")
		writeInstallFile(t, secret, "exfiltrate-me\n")

		dist := filepath.Join(base, "dist", "opencode")
		mkdirAll(t, filepath.Join(dist, "commands"))
		if err := os.Symlink(secret, filepath.Join(dist, "commands", "leak.md")); err != nil {
			t.Fatal(err)
		}

		err := syncOpenCodeCommandsDir(dist, config, skillsStore)
		if err == nil {
			t.Fatal("syncOpenCodeCommandsDir error = nil, want refusal of command symlink")
		}
		if !strings.Contains(err.Error(), "symlink") {
			t.Fatalf("error = %v, want symlink refusal", err)
		}
		assertInstallFile(t, existing, "# User command that must survive a refused sync\n")
		assertInstallPathMissing(t, filepath.Join(config, "commands", "leak.md"))
	})
}
