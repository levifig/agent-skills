package state

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/levifig/loaf/internal/project"
)

func TestDeriveIssuePrefixFromProjectSlug(t *testing.T) {
	cases := []struct {
		name, path, want string
	}{
		{name: "vcam", path: "/code/vcam", want: "VCAM"},
		{name: "crossfade", path: "/code/crossfade", want: "CROSSFADE"},
		{name: "gridsight-core-gds", path: "/code/gridsight-core-gds", want: "GRIDSIGHTCOREGDS"},
		{name: "loaf", path: "/code/loaf", want: "LOAF"},
		{name: "001", path: "/tmp/001", want: ""},
		{name: "", path: "/tmp/001", want: ""},
	}
	for _, tc := range cases {
		if got := DeriveIssuePrefix(tc.name, tc.path); got != tc.want {
			t.Fatalf("DeriveIssuePrefix(%q, %q) = %q, want %q", tc.name, tc.path, got, tc.want)
		}
	}
}

func namedProjectRoot(t *testing.T, name string) project.Root {
	t.Helper()
	dir := filepath.Join(t.TempDir(), name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	root, err := project.ResolveRoot(dir)
	if err != nil {
		t.Fatalf("ResolveRoot() error = %v", err)
	}
	return root
}

func TestIssueCreateMintsDerivedProjectPrefix(t *testing.T) {
	root := namedProjectRoot(t, "vcam")
	resolver := PathResolver{StateHome: t.TempDir()}
	if _, err := Initialize(context.Background(), root, resolver); err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}
	created, err := CreateIssue(context.Background(), root, resolver, IssueCreateOptions{Title: "Camera work"})
	if err != nil {
		t.Fatalf("CreateIssue() error = %v", err)
	}
	if created.Alias != "VCAM-1" {
		t.Fatalf("alias = %q, want VCAM-1", created.Alias)
	}
	identity, err := GetIssueIdentity(context.Background(), root, resolver)
	if err != nil {
		t.Fatalf("GetIssueIdentity() error = %v", err)
	}
	if identity.Prefix != "VCAM" || identity.NextNumber != 2 {
		t.Fatalf("identity = %#v, want VCAM next 2", identity)
	}
}

func TestAlignLeakedIssuePrefixRewritesAliasesAndNotes(t *testing.T) {
	root := namedProjectRoot(t, "vcam")
	resolver := PathResolver{StateHome: t.TempDir()}
	if _, err := Initialize(context.Background(), root, resolver); err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}
	if _, err := SetIssueIdentity(context.Background(), root, resolver, IssueIdentityOptions{Prefix: "LOAF"}); err != nil {
		t.Fatalf("SetIssueIdentity() error = %v", err)
	}
	created, err := CreateIssue(context.Background(), root, resolver, IssueCreateOptions{Title: "Leaked"})
	if err != nil {
		t.Fatalf("CreateIssue() error = %v", err)
	}
	if created.Alias != "LOAF-1" {
		t.Fatalf("alias = %q, want LOAF-1 before align", created.Alias)
	}
	task, err := CreateTask(context.Background(), root, resolver, TaskCreateOptions{Title: "Open leftover"})
	if err != nil {
		t.Fatalf("CreateTask() error = %v", err)
	}
	if _, err := Absorb(context.Background(), root, resolver, AbsorbOptions{Ref: task.Task.Alias}); err != nil {
		t.Fatalf("Absorb() error = %v", err)
	}

	dry, err := AlignLeakedIssuePrefix(context.Background(), root, resolver, true)
	if err != nil {
		t.Fatalf("AlignLeakedIssuePrefix(dry-run) error = %v", err)
	}
	if !dry.DryRun || dry.Rewritten != 1 || dry.Items[0].ToPrefix != "VCAM" || dry.Items[0].Aliases != 2 {
		t.Fatalf("dry-run = %#v, want VCAM rewrite of 2 aliases", dry)
	}
	still, err := GetIssue(context.Background(), root, resolver, "LOAF-1")
	if err != nil || still.Alias != "LOAF-1" {
		t.Fatalf("dry-run mutated alias: %#v err=%v", still, err)
	}

	result, err := AlignLeakedIssuePrefix(context.Background(), root, resolver, false)
	if err != nil {
		t.Fatalf("AlignLeakedIssuePrefix() error = %v", err)
	}
	if result.DryRun || result.Rewritten != 1 || result.Items[0].Notes == 0 {
		t.Fatalf("result = %#v, want applied rewrite with absorb notes", result)
	}
	if _, err := GetIssue(context.Background(), root, resolver, "LOAF-1"); err == nil {
		t.Fatal("GetIssue(LOAF-1) error = nil, want missing after align")
	}
	aligned, err := GetIssue(context.Background(), root, resolver, "VCAM-1")
	if err != nil {
		t.Fatalf("GetIssue(VCAM-1) error = %v", err)
	}
	if aligned.Title != "Leaked" {
		t.Fatalf("VCAM-1 = %#v, want leaked issue", aligned)
	}
	shown, err := ShowTask(context.Background(), root, resolver, task.Task.Alias)
	if err != nil {
		t.Fatalf("ShowTask() error = %v", err)
	}
	if shown.Task.Status != LifecycleStatusArchived {
		t.Fatalf("task status = %q, want archived", shown.Task.Status)
	}
	from, to, leaked, err := IssuePrefixLeak(context.Background(), root, resolver)
	if err != nil || leaked {
		t.Fatalf("leak after align from=%q to=%q leaked=%v err=%v", from, to, leaked, err)
	}
}

func TestIssuePrefixLeakIgnoresLoafProject(t *testing.T) {
	root := namedProjectRoot(t, "loaf")
	resolver := PathResolver{StateHome: t.TempDir()}
	if _, err := Initialize(context.Background(), root, resolver); err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}
	if _, err := CreateIssue(context.Background(), root, resolver, IssueCreateOptions{Title: "Keep LOAF"}); err != nil {
		t.Fatalf("CreateIssue() error = %v", err)
	}
	_, _, leaked, err := IssuePrefixLeak(context.Background(), root, resolver)
	if err != nil || leaked {
		t.Fatalf("loaf project leaked=%v err=%v, want none", leaked, err)
	}
	result, err := AlignLeakedIssuePrefix(context.Background(), root, resolver, false)
	if err != nil {
		t.Fatalf("AlignLeakedIssuePrefix() error = %v", err)
	}
	if result.Rewritten != 0 {
		t.Fatalf("result = %#v, want no rewrite on loaf", result)
	}
}

func TestAlignLeakedIssuePrefixLeavesCustomPrefix(t *testing.T) {
	root := namedProjectRoot(t, "vcam")
	resolver := PathResolver{StateHome: t.TempDir()}
	if _, err := Initialize(context.Background(), root, resolver); err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}
	if _, err := SetIssueIdentity(context.Background(), root, resolver, IssueIdentityOptions{Prefix: "CAM"}); err != nil {
		t.Fatalf("SetIssueIdentity() error = %v", err)
	}
	created, err := CreateIssue(context.Background(), root, resolver, IssueCreateOptions{Title: "Custom"})
	if err != nil {
		t.Fatalf("CreateIssue() error = %v", err)
	}
	if created.Alias != "CAM-1" {
		t.Fatalf("alias = %q, want CAM-1", created.Alias)
	}
	result, err := AlignLeakedIssuePrefix(context.Background(), root, resolver, false)
	if err != nil {
		t.Fatalf("AlignLeakedIssuePrefix() error = %v", err)
	}
	if result.Rewritten != 0 {
		t.Fatalf("result = %#v, want no rewrite of a hand-set prefix", result)
	}
	still, err := GetIssue(context.Background(), root, resolver, "CAM-1")
	if err != nil || still.Alias != "CAM-1" {
		t.Fatalf("custom alias mutated: %#v err=%v", still, err)
	}
}

func TestAlignLeakedIssuePrefixLeavesStartedBranch(t *testing.T) {
	root := namedProjectRoot(t, "infra")
	resolver := PathResolver{StateHome: t.TempDir()}
	status, err := Initialize(context.Background(), root, resolver)
	if err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}
	if _, err := SetIssueIdentity(context.Background(), root, resolver, IssueIdentityOptions{Prefix: "LOAF"}); err != nil {
		t.Fatalf("SetIssueIdentity() error = %v", err)
	}
	created, err := CreateIssue(context.Background(), root, resolver, IssueCreateOptions{Title: "Live worktree"})
	if err != nil {
		t.Fatalf("CreateIssue() error = %v", err)
	}
	store, err := OpenStore(status.DatabasePath)
	if err != nil {
		t.Fatalf("OpenStore() error = %v", err)
	}
	defer store.Close()
	if _, err := store.db.Exec(`UPDATE issues SET started_branch = ?, started_worktree = ? WHERE id = ?`, "issue/loaf-1", "/tmp/infra-loaf-1", created.ID); err != nil {
		t.Fatalf("set started workspace: %v", err)
	}
	if _, err := AlignLeakedIssuePrefix(context.Background(), root, resolver, false); err != nil {
		t.Fatalf("AlignLeakedIssuePrefix() error = %v", err)
	}
	aligned, err := GetIssue(context.Background(), root, resolver, "INFRA-1")
	if err != nil {
		t.Fatalf("GetIssue(INFRA-1) error = %v", err)
	}
	if aligned.StartedBranch != "issue/loaf-1" || aligned.StartedWorktree != "/tmp/infra-loaf-1" {
		t.Fatalf("started workspace = %#v, want issue/loaf-1 left on disk", aligned)
	}
}

func TestAlignLeakedIssuePrefixesAllWalksGlobalDatabase(t *testing.T) {
	resolver := PathResolver{StateHome: t.TempDir()}
	vcam := namedProjectRoot(t, "vcam")
	dots := namedProjectRoot(t, "dots")
	loaf := namedProjectRoot(t, "loaf")
	for _, root := range []project.Root{vcam, dots, loaf} {
		if _, err := Initialize(context.Background(), root, resolver); err != nil {
			t.Fatalf("Initialize(%s) error = %v", root.Path(), err)
		}
	}
	if _, err := SetIssueIdentity(context.Background(), vcam, resolver, IssueIdentityOptions{Prefix: "LOAF"}); err != nil {
		t.Fatalf("SetIssueIdentity(vcam) error = %v", err)
	}
	if _, err := SetIssueIdentity(context.Background(), dots, resolver, IssueIdentityOptions{Prefix: "LOAF"}); err != nil {
		t.Fatalf("SetIssueIdentity(dots) error = %v", err)
	}
	if _, err := CreateIssue(context.Background(), vcam, resolver, IssueCreateOptions{Title: "Camera"}); err != nil {
		t.Fatalf("CreateIssue(vcam) error = %v", err)
	}
	if _, err := CreateIssue(context.Background(), dots, resolver, IssueCreateOptions{Title: "Dotfiles"}); err != nil {
		t.Fatalf("CreateIssue(dots) error = %v", err)
	}
	if _, err := CreateIssue(context.Background(), loaf, resolver, IssueCreateOptions{Title: "Keep"}); err != nil {
		t.Fatalf("CreateIssue(loaf) error = %v", err)
	}

	result, err := AlignLeakedIssuePrefixes(context.Background(), vcam, resolver, true, false)
	if err != nil {
		t.Fatalf("AlignLeakedIssuePrefixes() error = %v", err)
	}
	if result.Rewritten != 2 {
		t.Fatalf("result = %#v, want 2 leaked projects rewritten", result)
	}
	if _, err := GetIssue(context.Background(), vcam, resolver, "VCAM-1"); err != nil {
		t.Fatalf("GetIssue(VCAM-1) error = %v", err)
	}
	if _, err := GetIssue(context.Background(), dots, resolver, "DOTS-1"); err != nil {
		t.Fatalf("GetIssue(DOTS-1) error = %v", err)
	}
	kept, err := GetIssue(context.Background(), loaf, resolver, "LOAF-1")
	if err != nil || kept.Alias != "LOAF-1" {
		t.Fatalf("loaf alias = %#v err=%v, want LOAF-1", kept, err)
	}
}

func TestInspectNamesIssuePrefixLeak(t *testing.T) {
	root := namedProjectRoot(t, "vcam")
	resolver := PathResolver{StateHome: t.TempDir()}
	if _, err := Initialize(context.Background(), root, resolver); err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}
	if _, err := SetIssueIdentity(context.Background(), root, resolver, IssueIdentityOptions{Prefix: "LOAF"}); err != nil {
		t.Fatalf("SetIssueIdentity() error = %v", err)
	}
	if _, err := CreateIssue(context.Background(), root, resolver, IssueCreateOptions{Title: "Leaked"}); err != nil {
		t.Fatalf("CreateIssue() error = %v", err)
	}

	status, err := Inspect(root, resolver)
	if err != nil {
		t.Fatalf("Inspect() error = %v", err)
	}
	assertDiagnosticPolicy(t, status.Diagnostics, IssuePrefixLeakCode, RepairCategoryIssueIdentity, DiagnosticPolicyWarningDrift, false)
	assertDiagnosticDetail(t, status.Diagnostics, IssuePrefixLeakCode, "preview_command", IssuePrefixAlignCommand)
	action := findRepairAction(t, RepairPlanForStatus(status), "align-issue-prefix")
	if action.Safe || action.Applied || action.Command != IssuePrefixAlignCommand {
		t.Fatalf("repair action = %#v, want manual prefix align preview", action)
	}
}

func TestRewritePrefixTokensPrefersLongerAliases(t *testing.T) {
	got := rewritePrefixTokens("absorbed into LOAF-10 after LOAF-1", map[string]string{
		"LOAF-1":  "VCAM-1",
		"LOAF-10": "VCAM-10",
	})
	if got != "absorbed into VCAM-10 after VCAM-1" {
		t.Fatalf("rewrite = %q, want longest-first alias rewrite", got)
	}
}

func TestDefineIssuePrefixRewritesExistingAliases(t *testing.T) {
	root := namedProjectRoot(t, "gridsight-core-gds")
	resolver := PathResolver{StateHome: t.TempDir()}
	if _, err := Initialize(context.Background(), root, resolver); err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}
	created, err := CreateIssue(context.Background(), root, resolver, IssueCreateOptions{Title: "Core work"})
	if err != nil {
		t.Fatalf("CreateIssue() error = %v", err)
	}
	if created.Alias != "GRIDSIGHTCOREGDS-1" {
		t.Fatalf("alias = %q, want derived GRIDSIGHTCOREGDS-1", created.Alias)
	}

	dry, err := DefineIssuePrefix(context.Background(), root, resolver, "gds", true)
	if err != nil {
		t.Fatalf("DefineIssuePrefix(dry-run) error = %v", err)
	}
	if !dry.DryRun || dry.Rewritten != 1 || dry.Items[0].ToPrefix != "GDS" || dry.Items[0].Aliases != 1 {
		t.Fatalf("dry-run = %#v, want GDS rewrite of 1 alias", dry)
	}
	if _, err := GetIssue(context.Background(), root, resolver, "GRIDSIGHTCOREGDS-1"); err != nil {
		t.Fatalf("dry-run mutated alias: %v", err)
	}

	result, err := DefineIssuePrefix(context.Background(), root, resolver, "gds", false)
	if err != nil {
		t.Fatalf("DefineIssuePrefix() error = %v", err)
	}
	if result.DryRun || result.Items[0].FromPrefix != "GRIDSIGHTCOREGDS" || result.Items[0].ToPrefix != "GDS" {
		t.Fatalf("result = %#v, want applied GDS define", result)
	}
	if _, err := GetIssue(context.Background(), root, resolver, "GRIDSIGHTCOREGDS-1"); err == nil {
		t.Fatal("old alias still resolves after define")
	}
	defined, err := GetIssue(context.Background(), root, resolver, "GDS-1")
	if err != nil || defined.Title != "Core work" {
		t.Fatalf("GDS-1 = %#v err=%v, want retitled alias", defined, err)
	}
	identity, err := GetIssueIdentity(context.Background(), root, resolver)
	if err != nil || identity.Prefix != "GDS" || identity.NextNumber != 2 {
		t.Fatalf("identity = %#v, want GDS next 2", identity)
	}
	again, err := DefineIssuePrefix(context.Background(), root, resolver, "GDS", false)
	if err != nil {
		t.Fatalf("DefineIssuePrefix(same) error = %v", err)
	}
	if again.Rewritten != 0 {
		t.Fatalf("same prefix = %#v, want no rewrite", again)
	}
}

func TestDefineIssuePrefixSetsLinearTeamWithoutRewrite(t *testing.T) {
	root := namedProjectRoot(t, "vcam")
	resolver := PathResolver{StateHome: t.TempDir()}
	if _, err := Initialize(context.Background(), root, resolver); err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}
	created, err := CreateIssue(context.Background(), root, resolver, IssueCreateOptions{Title: "Local first"})
	if err != nil {
		t.Fatalf("CreateIssue() error = %v", err)
	}
	if created.Alias != "VCAM-1" {
		t.Fatalf("alias = %q, want VCAM-1 before tracker define", created.Alias)
	}
	if _, err := SetIssueIdentity(context.Background(), root, resolver, IssueIdentityOptions{Authority: IssueAuthorityLinear, Prefix: "ENG"}); err != nil {
		t.Fatalf("SetIssueIdentity() error = %v", err)
	}
	result, err := DefineIssuePrefix(context.Background(), root, resolver, "OPS", false)
	if err != nil {
		t.Fatalf("DefineIssuePrefix() error = %v", err)
	}
	if !result.ConfigWritten || !result.TeamKeyWritten || result.Prefix != "OPS" {
		t.Fatalf("result = %#v, want OPS team persist without alias rewrite", result)
	}
	if _, err := GetIssue(context.Background(), root, resolver, "VCAM-1"); err != nil {
		t.Fatalf("GetIssue(VCAM-1) error = %v, want tracker define to leave local aliases", err)
	}
	cfg, err := LoadLinearAdapterConfig(context.Background(), root, resolver)
	if err != nil {
		t.Fatalf("LoadLinearAdapterConfig() error = %v", err)
	}
	if cfg.TeamKey != "OPS" {
		t.Fatalf("TeamKey = %q, want OPS", cfg.TeamKey)
	}
	issueCfg, err := LoadIssueProjectConfig(root.Path())
	if err != nil || issueCfg.Authority != IssueAuthorityLinear || issueCfg.Prefix != "OPS" {
		t.Fatalf("loaf.json issue = %#v err=%v, want linear OPS", issueCfg, err)
	}
}
