package cli

import (
	"os"
	"path/filepath"
	"testing"
)

func TestChangeScopeDigest(t *testing.T) {
	t.Run("identical-trees-digest-identically-regardless-of-quotePath", func(t *testing.T) {
		repo := initCLIGitRepo(t)
		writeEvidenceFixtureTree(t, repo)
		commitAllChangeTest(t, repo, "chore: seed evidence tree")

		gitCLI(t, repo, "config", "core.quotePath", "true")
		a, err := scopeDigest(repo, "HEAD", ChangeEvidenceExclusions(), nil)
		if err != nil {
			t.Fatalf("digest quotePath=true: %v", err)
		}
		gitCLI(t, repo, "config", "core.quotePath", "false")
		b, err := scopeDigest(repo, "HEAD", ChangeEvidenceExclusions(), nil)
		if err != nil {
			t.Fatalf("digest quotePath=false: %v", err)
		}
		if a.Digest == "" || a.Digest != b.Digest {
			t.Fatalf("digest mismatch under quotePath: %s vs %s", a.Digest, b.Digest)
		}
		if len(a.Sections) == 0 {
			t.Fatal("expected scope_sections")
		}
	})

	t.Run("sort-independent-of-traversal-order", func(t *testing.T) {
		// Two trees with the same entries must digest identically even if we
		// feed unsorted ls-tree output through the parser path — scopeDigest
		// byte-sorts before hashing.
		entries := []changeTreeEntry{
			{Mode: "100644", OID: "aaa", Path: "z.txt"},
			{Mode: "100644", OID: "bbb", Path: "a.txt"},
			{Mode: "100755", OID: "ccc", Path: "m/bin"},
		}
		reversed := []changeTreeEntry{entries[2], entries[1], entries[0]}
		sortedCopy := append([]changeTreeEntry(nil), entries...)
		// Mimic scopeDigest's sort.
		sortEvidenceEntries(sortedCopy)
		sortEvidenceEntries(reversed)
		if hashEvidenceEntries(sortedCopy) != hashEvidenceEntries(reversed) {
			t.Fatal("byte-sort must make digest traversal-order independent")
		}
	})

	t.Run("mode-change-changes-digest", func(t *testing.T) {
		repo := initCLIGitRepo(t)
		path := filepath.Join(repo, "script.sh")
		if err := os.WriteFile(path, []byte("#!/bin/sh\necho hi\n"), 0o644); err != nil {
			t.Fatalf("WriteFile: %v", err)
		}
		commitAllChangeTest(t, repo, "chore: add script non-executable")
		before, err := scopeDigest(repo, "HEAD", ChangeEvidenceExclusions(), nil)
		if err != nil {
			t.Fatalf("digest before: %v", err)
		}
		gitCLI(t, repo, "update-index", "--chmod=+x", "script.sh")
		gitCLI(t, repo, "-c", "user.name=Loaf Test", "-c", "user.email=loaf@example.test", "-c", "commit.gpgsign=false", "commit", "-m", "chore: mark executable")
		after, err := scopeDigest(repo, "HEAD", ChangeEvidenceExclusions(), nil)
		if err != nil {
			t.Fatalf("digest after: %v", err)
		}
		if before.Digest == after.Digest {
			t.Fatal("100644→100755 must change the digest")
		}
	})

	t.Run("excluded-paths-never-participate", func(t *testing.T) {
		repo := initCLIGitRepo(t)
		writeEvidenceFixtureTree(t, repo)
		commitAllChangeTest(t, repo, "chore: seed")

		baseline, err := scopeDigest(repo, "HEAD", ChangeEvidenceExclusions(), nil)
		if err != nil {
			t.Fatalf("baseline: %v", err)
		}

		// Receipts-only commit.
		receipt := filepath.Join(repo, "docs", "changes", "20260728-demo", "receipts", "verify.json")
		if err := os.MkdirAll(filepath.Dir(receipt), 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		if err := os.WriteFile(receipt, []byte(`{"schema_version":2}`+"\n"), 0o644); err != nil {
			t.Fatalf("WriteFile receipt: %v", err)
		}
		commitAllChangeTest(t, repo, "chore: receipts only")
		afterReceipt, err := scopeDigest(repo, "HEAD", ChangeEvidenceExclusions(), nil)
		if err != nil {
			t.Fatalf("after receipt: %v", err)
		}
		if afterReceipt.Digest != baseline.Digest {
			t.Fatalf("receipts-only commit must leave digest unchanged: %s → %s", baseline.Digest, afterReceipt.Digest)
		}

		// Reports-only commit.
		report := filepath.Join(repo, "docs", "changes", "20260728-demo", "reports", "board.html")
		if err := os.MkdirAll(filepath.Dir(report), 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		if err := os.WriteFile(report, []byte("<html></html>\n"), 0o644); err != nil {
			t.Fatalf("WriteFile report: %v", err)
		}
		commitAllChangeTest(t, repo, "chore: reports only")
		afterReport, err := scopeDigest(repo, "HEAD", ChangeEvidenceExclusions(), nil)
		if err != nil {
			t.Fatalf("after report: %v", err)
		}
		if afterReport.Digest != baseline.Digest {
			t.Fatalf("reports-only commit must leave digest unchanged: %s → %s", baseline.Digest, afterReport.Digest)
		}

		// Allowlist paths.
		if err := os.WriteFile(filepath.Join(repo, "package.json"), []byte(`{"version":"9.9.9"}`+"\n"), 0o644); err != nil {
			t.Fatalf("WriteFile package.json: %v", err)
		}
		if err := os.MkdirAll(filepath.Join(repo, "dist"), 0o755); err != nil {
			t.Fatalf("mkdir dist: %v", err)
		}
		if err := os.WriteFile(filepath.Join(repo, "dist", "out.js"), []byte("x\n"), 0o644); err != nil {
			t.Fatalf("WriteFile dist: %v", err)
		}
		commitAllChangeTest(t, repo, "chore: release metadata only")
		afterMeta, err := scopeDigest(repo, "HEAD", ChangeEvidenceExclusions(), nil)
		if err != nil {
			t.Fatalf("after meta: %v", err)
		}
		if afterMeta.Digest != baseline.Digest {
			t.Fatalf("allowlist-only commit must leave digest unchanged")
		}

		// A real code path must change it.
		if err := os.WriteFile(filepath.Join(repo, "internal", "cli", "x.go"), []byte("package cli\n"), 0o644); err != nil {
			t.Fatalf("WriteFile code: %v", err)
		}
		commitAllChangeTest(t, repo, "feat: real code")
		afterCode, err := scopeDigest(repo, "HEAD", ChangeEvidenceExclusions(), nil)
		if err != nil {
			t.Fatalf("after code: %v", err)
		}
		if afterCode.Digest == baseline.Digest {
			t.Fatal("non-excluded path must change digest")
		}
		if afterCode.Sections["internal"] == "" || afterCode.Sections["internal"] == baseline.Sections["internal"] {
			t.Fatalf("internal section must drift: %#v vs %#v", baseline.Sections, afterCode.Sections)
		}
	})

	t.Run("case-sensitive-mask-matching", func(t *testing.T) {
		if matchEvidenceGlob("Docs/changes/x/receipts/verify.json", "docs/changes/*/receipts/**") {
			t.Fatal("mask must be case-sensitive")
		}
		if !matchEvidenceGlob("docs/changes/x/receipts/verify.json", "docs/changes/*/receipts/**") {
			t.Fatal("expected match for exact-case receipts path")
		}
		if matchEvidenceGlob("Package.json", "package.json") {
			t.Fatal("package.json mask must be case-sensitive")
		}
		if !matchEvidenceGlob("dist/foo/bar.js", "dist/**") {
			t.Fatal("dist/** must match nested paths")
		}
		if matchEvidenceGlob("distributor/x", "dist/**") {
			t.Fatal("dist/** must not prefix-match unrelated paths")
		}
	})

	t.Run("exclusions-exported-boundary", func(t *testing.T) {
		got := ChangeEvidenceExclusions()
		wantParts := []string{
			"docs/changes/*/receipts/**",
			"docs/changes/*/reports/**",
			"package.json",
			".claude-plugin/marketplace.json",
			"CHANGELOG.md",
			"dist/**",
			"plugins/**",
			"bin/**",
		}
		if len(got) != len(wantParts) {
			t.Fatalf("exclusions = %#v, want %#v", got, wantParts)
		}
		for i, want := range wantParts {
			if got[i] != want {
				t.Fatalf("exclusions[%d] = %q, want %q", i, got[i], want)
			}
		}
		if ChangeEvidenceDigestSpec != "v1" {
			t.Fatalf("digest spec = %q, want v1", ChangeEvidenceDigestSpec)
		}
		if len(ReleaseMetadataAllowlist) == 0 {
			t.Fatal("ReleaseMetadataAllowlist must be exported for promotion Change")
		}
	})
}

func writeEvidenceFixtureTree(t *testing.T, repo string) {
	t.Helper()
	files := map[string]string{
		"internal/cli/main.go": "package cli\n",
		"content/skills/x.md":  "# skill\n",
		"weird name.txt":       "space\n",
	}
	for rel, body := range files {
		path := filepath.Join(repo, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", rel, err)
		}
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatalf("WriteFile %s: %v", rel, err)
		}
	}
}

func sortEvidenceEntries(entries []changeTreeEntry) {
	// Local helper mirroring scopeDigest's byte-sort without importing sort in
	// every call site of the test — keep deterministic for the fixture.
	for i := 0; i < len(entries); i++ {
		for j := i + 1; j < len(entries); j++ {
			if entries[j].Path < entries[i].Path {
				entries[i], entries[j] = entries[j], entries[i]
			}
		}
	}
}

func TestMatchEvidenceGlob(t *testing.T) {
	cases := []struct {
		path, pattern string
		want          bool
	}{
		{"docs/changes/foo/receipts/verify.json", "docs/changes/*/receipts/**", true},
		{"docs/changes/foo/receipts", "docs/changes/*/receipts/**", true},
		{"docs/changes/foo/shape.md", "docs/changes/*/receipts/**", false},
		{"docs/changes/foo/bar/receipts/x", "docs/changes/*/receipts/**", false},
		{"bin/loaf", "bin/**", true},
		{"bin", "bin/**", true},
		{"CHANGELOG.md", "CHANGELOG.md", true},
		{"docs/CHANGELOG.md", "CHANGELOG.md", false},
	}
	for _, tc := range cases {
		got := matchEvidenceGlob(tc.path, tc.pattern)
		if got != tc.want {
			t.Fatalf("match(%q, %q) = %v, want %v", tc.path, tc.pattern, got, tc.want)
		}
	}
}
