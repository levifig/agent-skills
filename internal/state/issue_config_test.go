package state

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeIssueProjectConfigFile(t *testing.T, root string, authority, prefix string) {
	t.Helper()
	if err := WriteIssueProjectConfig(root, authority, prefix); err != nil {
		t.Fatalf("WriteIssueProjectConfig() error = %v", err)
	}
}

func TestInitializeAppliesIssueProjectConfig(t *testing.T) {
	root := namedProjectRoot(t, "vcam")
	writeIssueProjectConfigFile(t, root.Path(), IssueAuthorityLocal, "CAM")
	resolver := PathResolver{StateHome: t.TempDir()}
	if _, err := Initialize(context.Background(), root, resolver); err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}
	identity, ok, err := LookupIssueIdentity(context.Background(), root, resolver)
	if err != nil || !ok {
		t.Fatalf("LookupIssueIdentity() = %#v ok=%t err=%v, want applied CAM", identity, ok, err)
	}
	if identity.Prefix != "CAM" || identity.Authority != IssueAuthorityLocal {
		t.Fatalf("identity = %#v, want local CAM", identity)
	}
	created, err := CreateIssue(context.Background(), root, resolver, IssueCreateOptions{Title: "Camera work"})
	if err != nil {
		t.Fatalf("CreateIssue() error = %v", err)
	}
	if created.Alias != "CAM-1" {
		t.Fatalf("alias = %q, want CAM-1 from loaf.json", created.Alias)
	}
}

func TestInitializeDoesNotOverwriteExistingIssueIdentity(t *testing.T) {
	root := namedProjectRoot(t, "vcam")
	resolver := PathResolver{StateHome: t.TempDir()}
	if _, err := Initialize(context.Background(), root, resolver); err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}
	if _, err := CreateIssue(context.Background(), root, resolver, IssueCreateOptions{Title: "Derived"}); err != nil {
		t.Fatalf("CreateIssue() error = %v", err)
	}
	writeIssueProjectConfigFile(t, root.Path(), IssueAuthorityLocal, "CAM")
	if _, err := Initialize(context.Background(), root, resolver); err != nil {
		t.Fatalf("second Initialize() error = %v", err)
	}
	identity, err := GetIssueIdentity(context.Background(), root, resolver)
	if err != nil || identity.Prefix != "VCAM" {
		t.Fatalf("identity = %#v err=%v, want derived VCAM left in place", identity, err)
	}
	status, err := Inspect(root, resolver)
	if err != nil {
		t.Fatalf("Inspect() error = %v", err)
	}
	assertDiagnosticPolicy(t, status.Diagnostics, IssuePrefixConfigDriftCode, RepairCategoryIssueIdentity, DiagnosticPolicyWarningDrift, false)
}

func TestCreateIssuePrefersLoafJSONPrefixBeforeIdentityExists(t *testing.T) {
	root := namedProjectRoot(t, "vcam")
	resolver := PathResolver{StateHome: t.TempDir()}
	if _, err := Initialize(context.Background(), root, resolver); err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}
	writeIssueProjectConfigFile(t, root.Path(), IssueAuthorityLocal, "CAM")
	created, err := CreateIssue(context.Background(), root, resolver, IssueCreateOptions{Title: "From config"})
	if err != nil {
		t.Fatalf("CreateIssue() error = %v", err)
	}
	if created.Alias != "CAM-1" {
		t.Fatalf("alias = %q, want CAM-1 from loaf.json on first mint", created.Alias)
	}
}

func TestLoadLinearAdapterConfigFallsBackToIssuePrefix(t *testing.T) {
	t.Setenv("LINEAR_TEAM_KEY", "")
	root := namedProjectRoot(t, "vcam")
	resolver := PathResolver{StateHome: t.TempDir()}
	if _, err := Initialize(context.Background(), root, resolver); err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}
	writeIssueProjectConfigFile(t, root.Path(), IssueAuthorityLinear, "ENG")
	cfg, err := LoadLinearAdapterConfig(context.Background(), root, resolver)
	if err != nil {
		t.Fatalf("LoadLinearAdapterConfig() error = %v", err)
	}
	if cfg.TeamKey != "ENG" {
		t.Fatalf("TeamKey = %q, want ENG from loaf.json", cfg.TeamKey)
	}
}

func TestInspectNamesMissingIssuePrefixConfig(t *testing.T) {
	root := namedProjectRoot(t, "vcam")
	resolver := PathResolver{StateHome: t.TempDir()}
	if _, err := Initialize(context.Background(), root, resolver); err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}
	if _, err := CreateIssue(context.Background(), root, resolver, IssueCreateOptions{Title: "Derived"}); err != nil {
		t.Fatalf("CreateIssue() error = %v", err)
	}
	status, err := Inspect(root, resolver)
	if err != nil {
		t.Fatalf("Inspect() error = %v", err)
	}
	assertDiagnosticPolicy(t, status.Diagnostics, IssuePrefixConfigMissingCode, RepairCategoryIssueIdentity, DiagnosticPolicyWarningDrift, false)
	action := findRepairAction(t, RepairPlanForStatus(status), "persist-issue-prefix")
	if action.Safe || action.Command != IssuePrefixPersistCommand("VCAM") {
		t.Fatalf("repair action = %#v, want persist VCAM", action)
	}
}

func TestIssuePrefixLeakIgnoresExplicitConfigPrefix(t *testing.T) {
	root := namedProjectRoot(t, "vcam")
	resolver := PathResolver{StateHome: t.TempDir()}
	if _, err := Initialize(context.Background(), root, resolver); err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}
	if _, err := SetIssueIdentity(context.Background(), root, resolver, IssueIdentityOptions{Prefix: "LOAF"}); err != nil {
		t.Fatalf("SetIssueIdentity() error = %v", err)
	}
	writeIssueProjectConfigFile(t, root.Path(), IssueAuthorityLocal, "LOAF")
	_, _, leaked, err := IssuePrefixLeak(context.Background(), root, resolver)
	if err != nil {
		t.Fatalf("IssuePrefixLeak() error = %v", err)
	}
	if leaked {
		t.Fatal("explicit loaf.json LOAF prefix reported as a leak")
	}
}

func TestIssueProjectConfigFindingsWarnsWithoutInventing(t *testing.T) {
	warnings, errors := IssueProjectConfigFindings(map[string]any{
		"version": "1.0.0",
	})
	if len(errors) != 0 {
		t.Fatalf("errors = %v, want none", errors)
	}
	if len(warnings) != 1 || !strings.Contains(warnings[0], "issue.prefix is not configured") {
		t.Fatalf("warnings = %v, want missing prefix", warnings)
	}
}

func TestWriteIssueProjectConfigMergesExistingFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".agents", "loaf.json")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(path, []byte(`{"version":"1.0.0","integrations":{"linear":{"enabled":false}}}`+"\n"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if err := WriteIssueProjectConfig(dir, IssueAuthorityLinear, "eng"); err != nil {
		t.Fatalf("WriteIssueProjectConfig() error = %v", err)
	}
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	var parsed map[string]any
	if err := json.Unmarshal(body, &parsed); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if parsed["version"] != "1.0.0" {
		t.Fatalf("version = %v, want preserved", parsed["version"])
	}
	issue := parsed["issue"].(map[string]any)
	if issue["authority"] != IssueAuthorityLinear || issue["prefix"] != "ENG" {
		t.Fatalf("issue = %#v, want linear ENG", issue)
	}
}

func TestAlignLeakedIssuePrefixRespectsExplicitConfig(t *testing.T) {
	root := namedProjectRoot(t, "vcam")
	resolver := PathResolver{StateHome: t.TempDir()}
	if _, err := Initialize(context.Background(), root, resolver); err != nil {
		t.Fatalf("Initialize() error = %v", err)
	}
	if _, err := SetIssueIdentity(context.Background(), root, resolver, IssueIdentityOptions{Prefix: "LOAF"}); err != nil {
		t.Fatalf("SetIssueIdentity() error = %v", err)
	}
	if _, err := CreateIssue(context.Background(), root, resolver, IssueCreateOptions{Title: "Explicit"}); err != nil {
		t.Fatalf("CreateIssue() error = %v", err)
	}
	writeIssueProjectConfigFile(t, root.Path(), IssueAuthorityLocal, "LOAF")
	result, err := AlignLeakedIssuePrefix(context.Background(), root, resolver, false)
	if err != nil {
		t.Fatalf("AlignLeakedIssuePrefix() error = %v", err)
	}
	if result.Rewritten != 0 {
		t.Fatalf("align = %#v, want no rewrite of explicit loaf.json LOAF", result)
	}
	if _, err := GetIssue(context.Background(), root, resolver, "LOAF-1"); err != nil {
		t.Fatalf("GetIssue(LOAF-1) error = %v, want explicit prefix kept", err)
	}
}
