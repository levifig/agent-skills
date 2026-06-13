package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestEnsureInstallSymlinkCreatesAndDetectsCorrectLink(t *testing.T) {
	root := realpath(t, t.TempDir())
	canonical := filepath.Join(root, ".agents", "AGENTS.md")
	link := filepath.Join(root, "AGENTS.md")
	writeInstallFile(t, canonical, "# Canonical\n")

	result := ensureInstallSymlink(link, ".agents/AGENTS.md", "./AGENTS.md", installSymlinkOptions{
		CanonicalPath: canonical,
		ProjectRoot:   root,
	})
	if result.Action != "created" || result.Error != "" {
		t.Fatalf("create result = %#v, want created without error", result)
	}
	assertInstallSymlinkTarget(t, link, canonical)

	result = ensureInstallSymlink(link, ".agents/AGENTS.md", "./AGENTS.md", installSymlinkOptions{
		CanonicalPath: canonical,
		ProjectRoot:   root,
	})
	if result.Action != "already-correct" || result.Error != "" {
		t.Fatalf("second result = %#v, want already-correct without error", result)
	}
}

func TestEnsureInstallSymlinkRelinksWrongTargetWithConsentControls(t *testing.T) {
	t.Run("assume yes relinks", func(t *testing.T) {
		root := realpath(t, t.TempDir())
		canonical := filepath.Join(root, ".agents", "AGENTS.md")
		link := filepath.Join(root, "AGENTS.md")
		writeInstallFile(t, canonical, "# Canonical\n")
		writeInstallFile(t, filepath.Join(root, "legacy.md"), "legacy\n")
		if err := os.Symlink("legacy.md", link); err != nil {
			t.Fatalf("Symlink legacy error = %v", err)
		}

		result := ensureInstallSymlink(link, ".agents/AGENTS.md", "./AGENTS.md", installSymlinkOptions{AssumeYes: true})
		if result.Action != "relinked" || result.Error != "" {
			t.Fatalf("relink result = %#v, want relinked without error", result)
		}
		assertInstallSymlinkTarget(t, link, canonical)
	})

	t.Run("prompt decline leaves link", func(t *testing.T) {
		root := realpath(t, t.TempDir())
		link := filepath.Join(root, "AGENTS.md")
		writeInstallFile(t, filepath.Join(root, "legacy.md"), "legacy\n")
		if err := os.Symlink("legacy.md", link); err != nil {
			t.Fatalf("Symlink legacy error = %v", err)
		}

		prompted := false
		result := ensureInstallSymlink(link, ".agents/AGENTS.md", "./AGENTS.md", installSymlinkOptions{
			Prompt: func(question string) bool {
				prompted = strings.Contains(question, "Relink?")
				return false
			},
		})
		if result.Action != "declined-relink" || !prompted {
			t.Fatalf("decline result = %#v prompted=%v, want declined-relink with prompt", result, prompted)
		}
		assertRawSymlink(t, link, "legacy.md")
	})

	t.Run("non interactive skips", func(t *testing.T) {
		root := realpath(t, t.TempDir())
		link := filepath.Join(root, "AGENTS.md")
		writeInstallFile(t, filepath.Join(root, "legacy.md"), "legacy\n")
		if err := os.Symlink("legacy.md", link); err != nil {
			t.Fatalf("Symlink legacy error = %v", err)
		}

		result := ensureInstallSymlink(link, ".agents/AGENTS.md", "./AGENTS.md", installSymlinkOptions{NonInteractive: true})
		if result.Action != "skipped-no-tty" || result.Error != "" {
			t.Fatalf("skip result = %#v, want skipped-no-tty without error", result)
		}
		assertRawSymlink(t, link, "legacy.md")
	})
}

func TestEnsureInstallSymlinkReplacesRealFileWithBackupAndCanonicalMerge(t *testing.T) {
	root := realpath(t, t.TempDir())
	canonical := filepath.Join(root, ".agents", "AGENTS.md")
	link := filepath.Join(root, "AGENTS.md")
	writeInstallFile(t, canonical, "# Canonical\n")
	writeInstallFile(t, link, "# Project Instructions\n\nKeep this user text.\n")

	result := ensureInstallSymlink(link, ".agents/AGENTS.md", "./AGENTS.md", installSymlinkOptions{
		AssumeYes:     true,
		CanonicalPath: canonical,
		ProjectRoot:   root,
	})
	if result.Action != "replaced-file" || result.BackupPath != link+".bak" || !result.Merged || result.Error != "" {
		t.Fatalf("replace result = %#v, want replaced-file backup and merge", result)
	}
	assertInstallSymlinkTarget(t, link, canonical)
	assertInstallFile(t, link+".bak", "# Project Instructions\n\nKeep this user text.\n")

	body := string(readFileBytes(t, canonical))
	if !strings.Contains(body, "# Canonical") || !strings.Contains(body, "## Migrated from AGENTS.md") || !strings.Contains(body, "Keep this user text.") {
		t.Fatalf("canonical body = %q, want original plus migrated user content", body)
	}
}

func TestEnsureInstallSymlinkDeclinesOrSkipsRealFileReplacement(t *testing.T) {
	for _, tc := range []struct {
		name    string
		options installSymlinkOptions
		want    string
	}{
		{
			name: "prompt decline",
			options: installSymlinkOptions{
				Prompt: func(string) bool { return false },
			},
			want: "declined-replace",
		},
		{
			name:    "non interactive",
			options: installSymlinkOptions{NonInteractive: true},
			want:    "skipped-no-tty",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := realpath(t, t.TempDir())
			link := filepath.Join(root, "AGENTS.md")
			writeInstallFile(t, link, "# Project Instructions\n")

			result := ensureInstallSymlink(link, ".agents/AGENTS.md", "./AGENTS.md", tc.options)
			if result.Action != tc.want || result.Error != "" {
				t.Fatalf("result = %#v, want %s without error", result, tc.want)
			}
			assertInstallFile(t, link, "# Project Instructions\n")
		})
	}
}

func TestEnsureInstallSymlinkCreatesCanonicalWhenSourceOnlyManagedFence(t *testing.T) {
	root := realpath(t, t.TempDir())
	canonical := filepath.Join(root, ".agents", "AGENTS.md")
	link := filepath.Join(root, "AGENTS.md")
	writeInstallFile(t, link, "<!-- loaf:managed:start v1.0.0 -->\nmanaged\n<!-- loaf:managed:end -->\n")

	result := ensureInstallSymlink(link, ".agents/AGENTS.md", "./AGENTS.md", installSymlinkOptions{
		AssumeYes:     true,
		CanonicalPath: canonical,
		ProjectRoot:   root,
	})
	if result.Action != "replaced-file" || result.Merged {
		t.Fatalf("result = %#v, want replaced-file without merge", result)
	}
	assertInstallSymlinkTarget(t, link, canonical)
	assertInstallFile(t, canonical, "")
}

func TestEnsureInstallSymlinkStripsManagedFenceBeforeMerge(t *testing.T) {
	root := realpath(t, t.TempDir())
	canonical := filepath.Join(root, ".agents", "AGENTS.md")
	link := filepath.Join(root, ".claude", "CLAUDE.md")
	writeInstallFile(t, canonical, "# Canonical\n")
	writeInstallFile(t, link, "# User Notes\n\n<!-- loaf:managed:start v1.0.0 -->\nold managed\n<!-- loaf:managed:end -->\n\nKeep this.\n")

	result := ensureInstallSymlink(link, "../.agents/AGENTS.md", ".claude/CLAUDE.md", installSymlinkOptions{
		AssumeYes:     true,
		CanonicalPath: canonical,
		ProjectRoot:   root,
	})
	if result.Action != "replaced-file" || !result.Merged {
		t.Fatalf("result = %#v, want replaced-file with merge", result)
	}
	body := string(readFileBytes(t, canonical))
	if strings.Contains(body, "old managed") || strings.Contains(body, "<!-- loaf:managed:start") {
		t.Fatalf("canonical body = %q, want managed fence stripped", body)
	}
	if !strings.Contains(body, "## Migrated from .claude/CLAUDE.md") || !strings.Contains(body, "# User Notes") || !strings.Contains(body, "Keep this.") {
		t.Fatalf("canonical body = %q, want migrated user notes", body)
	}
}

func TestEnsureProjectInstallSymlinksRoutesSelectedTargets(t *testing.T) {
	t.Run("claude and root targets create shared canonical", func(t *testing.T) {
		root := realpath(t, t.TempDir())
		results := ensureProjectInstallSymlinks(root, []string{"cursor"}, true, installSymlinkOptions{AssumeYes: true})
		if results[".claude/CLAUDE.md"].Action != "created" || results["./AGENTS.md"].Action != "created" {
			t.Fatalf("results = %#v, want both project symlinks created", results)
		}
		canonical := filepath.Join(root, ".agents", "AGENTS.md")
		assertInstallFile(t, canonical, "")
		assertInstallSymlinkTarget(t, filepath.Join(root, ".claude", "CLAUDE.md"), canonical)
		assertInstallSymlinkTarget(t, filepath.Join(root, "AGENTS.md"), canonical)
	})

	t.Run("no matching targets is noop", func(t *testing.T) {
		root := realpath(t, t.TempDir())
		results := ensureProjectInstallSymlinks(root, nil, false, installSymlinkOptions{AssumeYes: true})
		if len(results) != 0 {
			t.Fatalf("results = %#v, want empty", results)
		}
		if _, err := os.Stat(filepath.Join(root, ".agents", "AGENTS.md")); !os.IsNotExist(err) {
			t.Fatalf("canonical stat err = %v, want not exist", err)
		}
	})

	t.Run("codex only creates root agents link", func(t *testing.T) {
		root := realpath(t, t.TempDir())
		results := ensureProjectInstallSymlinks(root, []string{"codex"}, false, installSymlinkOptions{AssumeYes: true})
		if results["./AGENTS.md"].Action != "created" {
			t.Fatalf("results = %#v, want root AGENTS symlink", results)
		}
		if _, ok := results[".claude/CLAUDE.md"]; ok {
			t.Fatalf("results = %#v, want no Claude symlink", results)
		}
	})

	t.Run("claude only creates claude link", func(t *testing.T) {
		root := realpath(t, t.TempDir())
		results := ensureProjectInstallSymlinks(root, nil, true, installSymlinkOptions{AssumeYes: true})
		if results[".claude/CLAUDE.md"].Action != "created" {
			t.Fatalf("results = %#v, want Claude symlink", results)
		}
		if _, ok := results["./AGENTS.md"]; ok {
			t.Fatalf("results = %#v, want no root AGENTS symlink", results)
		}
	})
}

func TestEnsureProjectInstallSymlinksMigratesBothLegacyFiles(t *testing.T) {
	root := realpath(t, t.TempDir())
	writeInstallFile(t, filepath.Join(root, "AGENTS.md"), "# Root Agents\n")
	writeInstallFile(t, filepath.Join(root, ".claude", "CLAUDE.md"), "# Claude Notes\n")

	results := ensureProjectInstallSymlinks(root, []string{"cursor"}, true, installSymlinkOptions{AssumeYes: true})
	if results[".claude/CLAUDE.md"].Action != "replaced-file" || !results[".claude/CLAUDE.md"].Merged {
		t.Fatalf("Claude result = %#v, want replaced-file with merge", results[".claude/CLAUDE.md"])
	}
	if results["./AGENTS.md"].Action != "replaced-file" || !results["./AGENTS.md"].Merged {
		t.Fatalf("AGENTS result = %#v, want replaced-file with merge", results["./AGENTS.md"])
	}

	canonical := string(readFileBytes(t, filepath.Join(root, ".agents", "AGENTS.md")))
	if !strings.Contains(canonical, "## Migrated from .claude/CLAUDE.md") || !strings.Contains(canonical, "# Claude Notes") {
		t.Fatalf("canonical body = %q, want migrated Claude notes", canonical)
	}
	if !strings.Contains(canonical, "## Migrated from AGENTS.md") || !strings.Contains(canonical, "# Root Agents") {
		t.Fatalf("canonical body = %q, want migrated root AGENTS notes", canonical)
	}
	assertInstallFile(t, filepath.Join(root, ".claude", "CLAUDE.md.bak"), "# Claude Notes\n")
	assertInstallFile(t, filepath.Join(root, "AGENTS.md.bak"), "# Root Agents\n")
	assertInstallSymlinkTarget(t, filepath.Join(root, ".claude", "CLAUDE.md"), filepath.Join(root, ".agents", "AGENTS.md"))
	assertInstallSymlinkTarget(t, filepath.Join(root, "AGENTS.md"), filepath.Join(root, ".agents", "AGENTS.md"))
}

func TestRelativeInstallLinkTargetForProjectInstructionFiles(t *testing.T) {
	root := realpath(t, t.TempDir())
	canonical := filepath.Join(root, ".agents", "AGENTS.md")
	if got := relativeInstallLinkTarget(filepath.Join(root, "AGENTS.md"), canonical); got != ".agents/AGENTS.md" {
		t.Fatalf("root link target = %q, want .agents/AGENTS.md", got)
	}
	if got := relativeInstallLinkTarget(filepath.Join(root, ".claude", "CLAUDE.md"), canonical); got != "../.agents/AGENTS.md" {
		t.Fatalf("Claude link target = %q, want ../.agents/AGENTS.md", got)
	}
}

func assertInstallSymlinkTarget(t *testing.T, linkPath string, wantAbs string) {
	t.Helper()
	if !installIsSymlink(linkPath) {
		t.Fatalf("%s is not a symlink", linkPath)
	}
	if !installSymlinkPointsTo(linkPath, wantAbs) {
		t.Fatalf("%s resolves to %q, want %q", linkPath, resolveInstallSymlinkTarget(linkPath), filepath.Clean(wantAbs))
	}
}

func assertRawSymlink(t *testing.T, linkPath string, wantTarget string) {
	t.Helper()
	target, err := os.Readlink(linkPath)
	if err != nil {
		t.Fatalf("Readlink(%s) error = %v", linkPath, err)
	}
	if target != wantTarget {
		t.Fatalf("Readlink(%s) = %q, want %q", linkPath, target, wantTarget)
	}
}
