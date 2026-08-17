package cli

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/levifig/loaf/internal/project"
	"github.com/levifig/loaf/internal/state"
)

func namedIssueCLIFixture(t *testing.T, name string) (string, string) {
	t.Helper()
	dir := filepath.Join(realpath(t, t.TempDir()), name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	stateHome := t.TempDir()
	if err := (Runner{WorkingDir: dir, StateHome: stateHome}).Run([]string{"state", "init"}); err != nil {
		t.Fatalf("state init error = %v", err)
	}
	return dir, stateHome
}

func TestRunnerIssueIdentityShowsDerivedPrefix(t *testing.T) {
	workingDir, stateHome := namedIssueCLIFixture(t, "vcam")
	out, err := runIssue(t, workingDir, stateHome, "identity")
	if err != nil {
		t.Fatalf("issue identity error = %v\n%s", err, out)
	}
	if !strings.Contains(out, "prefix: VCAM") || !strings.Contains(out, "next number: 1") {
		t.Fatalf("identity = %q, want derived VCAM", out)
	}

	created, err := runIssue(t, workingDir, stateHome, "new", "Camera work", "--json")
	if err != nil {
		t.Fatalf("issue new error = %v\n%s", err, created)
	}
	result := decodeIssueResult(t, created)
	if result.Issue.Alias != "VCAM-1" {
		t.Fatalf("created alias = %q, want VCAM-1", result.Issue.Alias)
	}
}

func TestRunnerIssueIdentityAlignRewritesLeakedPrefix(t *testing.T) {
	workingDir, stateHome := namedIssueCLIFixture(t, "vcam")
	root, err := project.ResolveRoot(workingDir)
	if err != nil {
		t.Fatalf("ResolveRoot() error = %v", err)
	}
	resolver := state.PathResolver{StateHome: stateHome}
	if _, err := state.SetIssueIdentity(context.Background(), root, resolver, state.IssueIdentityOptions{Prefix: "LOAF"}); err != nil {
		t.Fatalf("SetIssueIdentity() error = %v", err)
	}
	if _, err := runIssue(t, workingDir, stateHome, "new", "Leaked"); err != nil {
		t.Fatalf("issue new error = %v", err)
	}

	dry, err := runIssue(t, workingDir, stateHome, "identity", "--align", "--dry-run")
	if err != nil {
		t.Fatalf("issue identity --align --dry-run error = %v\n%s", err, dry)
	}
	if !strings.Contains(dry, "would rewrite") || !strings.Contains(dry, "VCAM-*") {
		t.Fatalf("dry-run = %q, want VCAM preview", dry)
	}
	if _, err := runIssue(t, workingDir, stateHome, "show", "LOAF-1"); err != nil {
		t.Fatalf("dry-run mutated LOAF-1: %v", err)
	}

	applied, err := runIssue(t, workingDir, stateHome, "identity", "--align", "--json")
	if err != nil {
		t.Fatalf("issue identity --align error = %v\n%s", err, applied)
	}
	var result state.IssuePrefixAlignResult
	if err := json.Unmarshal([]byte(applied), &result); err != nil {
		t.Fatalf("align JSON error = %v\n%s", err, applied)
	}
	if result.DryRun || result.Rewritten != 1 || result.Items[0].ToPrefix != "VCAM" {
		t.Fatalf("align = %#v, want applied VCAM rewrite", result)
	}
	if _, err := runIssue(t, workingDir, stateHome, "show", "LOAF-1"); err == nil {
		t.Fatal("show LOAF-1 error = nil, want missing after align")
	}
	shown, err := runIssue(t, workingDir, stateHome, "show", "VCAM-1")
	if err != nil {
		t.Fatalf("show VCAM-1 error = %v\n%s", err, shown)
	}
	if !strings.Contains(shown, "Leaked") {
		t.Fatalf("show VCAM-1 = %q, want leaked title", shown)
	}
}

func TestRunnerIssueIdentityHelpListsAlign(t *testing.T) {
	workingDir, stateHome := issueCLIFixture(t)
	helpOut, err := runIssue(t, workingDir, stateHome, "--help")
	if err != nil {
		t.Fatalf("issue --help error = %v", err)
	}
	if !strings.Contains(helpOut, "identity") {
		t.Fatalf("issue help missing identity:\n%s", helpOut)
	}
	identityHelp, err := runIssue(t, workingDir, stateHome, "identity", "--help")
	if err != nil {
		t.Fatalf("issue identity --help error = %v", err)
	}
	for _, want := range []string{"--prefix", "--authority", "--align", "--all", "--dry-run", "--json"} {
		if !strings.Contains(identityHelp, want) {
			t.Fatalf("identity help missing %s:\n%s", want, identityHelp)
		}
	}
}

func TestRunnerIssueIdentityDefinesPrefix(t *testing.T) {
	workingDir, stateHome := namedIssueCLIFixture(t, "gridsight-core-gds")
	if _, err := runIssue(t, workingDir, stateHome, "new", "Core work"); err != nil {
		t.Fatalf("issue new error = %v", err)
	}
	dry, err := runIssue(t, workingDir, stateHome, "identity", "--prefix", "gds", "--dry-run")
	if err != nil {
		t.Fatalf("issue identity --prefix --dry-run error = %v\n%s", err, dry)
	}
	if !strings.Contains(dry, "would define prefix GDS") || !strings.Contains(dry, "GRIDSIGHTCOREGDS-*") {
		t.Fatalf("dry-run = %q, want GDS preview", dry)
	}
	if _, err := runIssue(t, workingDir, stateHome, "show", "GRIDSIGHTCOREGDS-1"); err != nil {
		t.Fatalf("dry-run mutated alias: %v", err)
	}
	out, err := runIssue(t, workingDir, stateHome, "identity", "--prefix", "gds")
	if err != nil {
		t.Fatalf("issue identity --prefix error = %v\n%s", err, out)
	}
	if !strings.Contains(out, "defined prefix GDS") {
		t.Fatalf("define = %q, want defined GDS", out)
	}
	shown, err := runIssue(t, workingDir, stateHome, "show", "GDS-1")
	if err != nil {
		t.Fatalf("show GDS-1 error = %v\n%s", err, shown)
	}
	if !strings.Contains(shown, "Core work") {
		t.Fatalf("show GDS-1 = %q, want core work", shown)
	}
}

func TestRunnerIssueIdentityPrefixRejectsAlign(t *testing.T) {
	workingDir, stateHome := issueCLIFixture(t)
	_, err := runIssue(t, workingDir, stateHome, "identity", "--prefix", "GDS", "--align")
	if err == nil || !strings.Contains(err.Error(), "--prefix and --authority cannot be combined with --align") {
		t.Fatalf("error = %v, want prefix/align refusal", err)
	}
}

func TestRunnerIssueIdentityPrefixWritesProjectConfig(t *testing.T) {
	workingDir, stateHome := namedIssueCLIFixture(t, "gridsight-core-gds")
	out, err := runIssue(t, workingDir, stateHome, "identity", "--prefix", "gds")
	if err != nil {
		t.Fatalf("issue identity --prefix error = %v\n%s", err, out)
	}
	if !strings.Contains(out, "wrote .agents/loaf.json") {
		t.Fatalf("define = %q, want loaf.json persist", out)
	}
	body, err := os.ReadFile(filepath.Join(workingDir, ".agents", "loaf.json"))
	if err != nil {
		t.Fatalf("ReadFile(loaf.json) error = %v", err)
	}
	var config struct {
		Issue struct {
			Authority string `json:"authority"`
			Prefix    string `json:"prefix"`
		} `json:"issue"`
	}
	if err := json.Unmarshal(body, &config); err != nil {
		t.Fatalf("loaf.json = %s parse error = %v", body, err)
	}
	if config.Issue.Authority != state.IssueAuthorityLocal || config.Issue.Prefix != "GDS" {
		t.Fatalf("loaf.json issue = %#v, want local GDS", config.Issue)
	}
}

func TestRunnerIssueIdentityAuthorityLinearWritesTeamKey(t *testing.T) {
	workingDir, stateHome := namedIssueCLIFixture(t, "vcam")
	out, err := runIssue(t, workingDir, stateHome, "identity", "--authority", "linear", "--prefix", "ENG")
	if err != nil {
		t.Fatalf("issue identity --authority linear error = %v\n%s", err, out)
	}
	if !strings.Contains(out, "defined authority linear") || !strings.Contains(out, "tracker owns aliases") {
		t.Fatalf("define = %q, want linear authority without rewrite", out)
	}
	shown, err := runIssue(t, workingDir, stateHome, "identity")
	if err != nil {
		t.Fatalf("issue identity error = %v\n%s", err, shown)
	}
	if !strings.Contains(shown, "authority: linear") || !strings.Contains(shown, "prefix: ENG") {
		t.Fatalf("identity = %q, want linear ENG", shown)
	}
	root, err := project.ResolveRoot(workingDir)
	if err != nil {
		t.Fatalf("ResolveRoot() error = %v", err)
	}
	cfg, err := state.LoadLinearAdapterConfig(context.Background(), root, state.PathResolver{StateHome: stateHome})
	if err != nil {
		t.Fatalf("LoadLinearAdapterConfig() error = %v", err)
	}
	if cfg.TeamKey != "ENG" {
		t.Fatalf("TeamKey = %q, want ENG", cfg.TeamKey)
	}
}
