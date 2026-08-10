package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Harness MCP configs and the shared hooks file share the loaf.json contract:
// absent → empty document; present-but-unusable → refuse, preserve, report.

func TestMergeHarnessConfigsRefuseMalformedJSON(t *testing.T) {
	for _, testCase := range []struct {
		name string
		body string
	}{
		{name: "truncated object", body: `{"mcpServers":`},
		{name: "json array", body: "[]\n"},
		{name: "json null", body: "null\n"},
		{name: "not json", body: "notes\n"},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			dir := t.TempDir()

			for _, kind := range []struct {
				name  string
				path  string
				merge func(path string) error
			}{
				{
					name: "cursor mcp.json",
					path: filepath.Join(dir, ".cursor", "mcp.json"),
					merge: func(path string) error {
						return mergeJSONMcpConfig(path, "mcpServers", "linear", []string{"npx", "-y", "mcp-remote"})
					},
				},
				{
					name: "opencode.json",
					path: filepath.Join(dir, "opencode.json"),
					merge: func(path string) error {
						return mergeOpenCodeMcpConfig(path, "linear", []string{"npx", "-y", "mcp-remote"})
					},
				},
				{
					name: "amp settings.json",
					path: filepath.Join(dir, ".amp", "settings.json"),
					merge: func(path string) error {
						return mergeJSONMcpConfig(path, "amp.mcpServers", "linear", []string{"npx", "-y", "mcp-remote"})
					},
				},
			} {
				t.Run(kind.name, func(t *testing.T) {
					writeInstallFile(t, kind.path, testCase.body)
					err := kind.merge(kind.path)
					if err == nil {
						t.Fatalf("merge error = nil, want a parse refusal")
					}
					if !strings.Contains(err.Error(), "does not parse as a JSON object") {
						t.Fatalf("merge error = %q, want the parse refusal", err)
					}
					if !strings.Contains(err.Error(), "preserving it as written") {
						t.Fatalf("merge error = %q, want preservation stated", err)
					}
					if got := string(readFileBytes(t, kind.path)); got != testCase.body {
						t.Fatalf("%s = %q, want it preserved byte-for-byte as %q", kind.path, got, testCase.body)
					}
				})
			}
		})
	}
}

func TestReadHookFileRefusesNonObject(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "hooks.json")
	writeInstallFile(t, path, "[]\n")

	_, err := readHookFile(path)
	if err == nil {
		t.Fatal("readHookFile error = nil, want a parse refusal")
	}
	if !strings.Contains(err.Error(), "parse hooks file") || !strings.Contains(err.Error(), "preserving it as written") {
		t.Fatalf("readHookFile error = %q, want a parse refusal that preserves the file", err)
	}
	if got := string(readFileBytes(t, path)); got != "[]\n" {
		t.Fatalf("hooks.json changed to %q; a refused read must not rewrite the file", got)
	}
}

func TestInstallMcpDetectionReportsPermissionDeniedConfig(t *testing.T) {
	skipWithoutEnforcedPermissions(t)

	root := t.TempDir()
	t.Setenv("HOME", filepath.Join(root, "home"))
	t.Setenv("USERPROFILE", filepath.Join(root, "home"))
	mcpPath := filepath.Join(root, ".cursor", "mcp.json")
	writeInstallFile(t, mcpPath, `{"mcpServers":{"linear":{"command":"npx"}}}`+"\n")
	chmodForTest(t, mcpPath, 0o000)

	status := detectInstallMcpForTarget(root, "cursor", "linear")
	if status.configured {
		t.Fatalf("status = %#v, want unconfigured with a notice", status)
	}
	if !strings.Contains(status.notice, mcpPath) || !strings.Contains(status.notice, "could not be inspected") {
		t.Fatalf("status.notice = %q, want the path and the read failure", status.notice)
	}

	// Dry-run plan surfaces the same notice rather than a silent not-configured.
	plan := planInstallMcp(root, []string{"cursor"})
	notices := installMcpPlanNotices(plan)
	if len(notices) == 0 {
		t.Fatalf("plan notices = %#v, want the permission failure named", plan)
	}
	joined := strings.Join(notices, "; ")
	if !strings.Contains(joined, mcpPath) {
		t.Fatalf("plan notices = %q, want the path named", joined)
	}
}

func TestInstallMcpDetectionReportsUnreadableParentDir(t *testing.T) {
	skipWithoutEnforcedPermissions(t)

	root := t.TempDir()
	t.Setenv("HOME", filepath.Join(root, "home"))
	t.Setenv("USERPROFILE", filepath.Join(root, "home"))
	mcpPath := filepath.Join(root, ".cursor", "mcp.json")
	writeInstallFile(t, mcpPath, `{"mcpServers":{}}`+"\n")
	chmodForTest(t, filepath.Join(root, ".cursor"), 0o000)

	status := detectInstallMcpForTarget(root, "cursor", "linear")
	if status.configured {
		t.Fatalf("status = %#v, want unconfigured with a notice", status)
	}
	if status.notice == "" {
		t.Fatal("status.notice is empty; an unreadable parent must produce a notice")
	}

	plan := planInstallMcp(root, []string{"cursor"})
	if len(installMcpPlanNotices(plan)) == 0 {
		t.Fatalf("plan notices empty for unreadable parent; plan = %#v", plan)
	}
}

func TestSymlinkPassRefusesUnreadableClaudeFile(t *testing.T) {
	skipWithoutEnforcedPermissions(t)

	root := t.TempDir()
	writeInstallFile(t, filepath.Join(root, "AGENTS.md"), "# Project\n")
	path := filepath.Join(root, ".claude", "CLAUDE.md")
	writeInstallFile(t, path, "# User claude instructions\n")
	chmodForTest(t, path, 0o000)

	result := ensureInstallSymlink(path, "../AGENTS.md", ".claude/CLAUDE.md", installSymlinkOptions{
		AssumeYes:     true,
		CanonicalPath: filepath.Join(root, "AGENTS.md"),
		ProjectRoot:   root,
	})
	if result.Action != "error" || !result.Refused {
		chmodForTest(t, path, 0o600)
		t.Fatalf("ensureInstallSymlink() = %#v, want a refused read that fails the project part", result)
	}
	if !anyInstallSymlinkRefusal(map[string]installSymlinkResult{"claude": result}) {
		chmodForTest(t, path, 0o600)
		t.Fatal("anyInstallSymlinkRefusal did not treat the permission failure as an abort")
	}

	// Plan agrees: error, not a promised replace/migration.
	action, detail := planInstallSymlink(path, "../AGENTS.md", ".claude/CLAUDE.md", filepath.Join(root, "AGENTS.md"), true)
	if action != "error" {
		chmodForTest(t, path, 0o600)
		t.Fatalf("planInstallSymlink() = %q, %q, want error", action, detail)
	}

	chmodForTest(t, path, 0o600)
}

func TestSymlinkPassRefusesUnreadableLegacyAgentsFile(t *testing.T) {
	skipWithoutEnforcedPermissions(t)

	root := t.TempDir()
	writeInstallFile(t, filepath.Join(root, "AGENTS.md"), "# Project\n")
	legacy := filepath.Join(root, ".agents", "AGENTS.md")
	writeInstallFile(t, legacy, "# Legacy agents\n")
	chmodForTest(t, legacy, 0o000)

	result := ensureRootInstallAgentsFile(root, installSymlinkOptions{AssumeYes: true})
	if result.Action != "error" || !result.Refused {
		chmodForTest(t, legacy, 0o600)
		t.Fatalf("ensureRootInstallAgentsFile() = %#v, want a refused read that fails the project part", result)
	}

	action, detail, isError := planRootInstallAgentsFile(root, true)
	if action != "error" || !isError {
		chmodForTest(t, legacy, 0o600)
		t.Fatalf("planRootInstallAgentsFile() = %q, %q, %v, want the same refusal in the plan", action, detail, isError)
	}

	chmodForTest(t, legacy, 0o600)
}

func TestAnyInstallSymlinkRefusalIncludesOrdinaryReadErrors(t *testing.T) {
	results := map[string]installSymlinkResult{
		"claude": installSymlinkReadRefusal("Failed to replace .claude/CLAUDE.md", os.ErrPermission),
	}
	if !results["claude"].Refused {
		t.Fatal("installSymlinkReadRefusal left Refused false for a permission error")
	}
	if !anyInstallSymlinkRefusal(results) {
		t.Fatal("anyInstallSymlinkRefusal returned false for a permission-denied migration read")
	}
}

// Ensure merge of a valid config still works after the refusal path was added.
func TestMergeHarnessConfigAcceptsValidObject(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "mcp.json")
	writeInstallFile(t, path, `{"mcpServers":{"other":{"command":"echo"}}}`+"\n")

	if err := mergeJSONMcpConfig(path, "mcpServers", "linear", []string{"npx", "-y", "mcp-remote"}); err != nil {
		t.Fatalf("mergeJSONMcpConfig(valid) error = %v", err)
	}
	var data map[string]any
	if err := json.Unmarshal(readFileBytes(t, path), &data); err != nil {
		t.Fatalf("rewritten config does not parse: %v", err)
	}
	servers, _ := data["mcpServers"].(map[string]any)
	if _, ok := servers["other"]; !ok {
		t.Fatalf("rewritten config lost user server: %#v", data)
	}
	if _, ok := servers["linear"]; !ok {
		t.Fatalf("rewritten config missing Loaf server: %#v", data)
	}
}

// A no-manifest Cursor distribution has no path left to promise: its entries
// are reconciled from a catalog that build output does not carry, so dry-run
// carries the staleness conflict, apply fails, and the file stays put.
func TestPlanLegacyHookNoManifestRefusesAStaleCursorDistribution(t *testing.T) {
	root, home := setupInstallCommandFixture(t)
	writeInstallFile(t, filepath.Join(root, "dist", "cursor", "skills", "foundations", "SKILL.md"), "# Foundations\n")
	// No .loaf-target-manifest.json — the legacy/no-manifest branch.
	writeInstallFile(t, filepath.Join(home, ".cursor", loafInstallMarkerFile), "old\n")
	malformed := `{"hooks":`
	hooksPath := filepath.Join(home, ".cursor", "hooks.json")
	writeInstallFile(t, hooksPath, malformed)

	plan := parseInstallPlanJSON(t, runInstallCapture(t, root, "upgrade", "--to", "cursor", "--dry-run", "--json"))
	cursor := findTargetPlan(t, plan, "cursor")
	if !cursor.Blocked {
		t.Fatalf("cursor plan Blocked = false, want true for a malformed legacy hooks.json")
	}
	var hooksDecision artifactPlanDecision
	found := false
	for _, artifact := range cursor.Artifacts {
		if artifact.ID == "hooks" && artifact.Kind == "hook-legacy" {
			hooksDecision = artifact
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("artifacts = %#v, want a hook-legacy decision", cursor.Artifacts)
	}
	if hooksDecision.Action != planActionConflict {
		t.Fatalf("hook-legacy action = %q, want conflict (not update)", hooksDecision.Action)
	}
	if !strings.Contains(hooksDecision.Detail, "stale") || !strings.Contains(hooksDecision.Detail, "loaf build") {
		t.Fatalf("hook-legacy detail = %q, want the staleness refusal that apply will raise", hooksDecision.Detail)
	}
	if !strings.Contains(hooksDecision.Destination, "hooks.json") {
		t.Fatalf("hook-legacy destination = %q, want the hooks.json path", hooksDecision.Destination)
	}

	// Apply must refuse and leave the truncated file byte-for-byte.
	output := runUpgradeExpectingExitError(t, root, "upgrade", "--to", "cursor", "--yes")
	if !strings.Contains(output, "hooks.json") && !strings.Contains(output, "Cursor") {
		t.Fatalf("upgrade output = %q, want the failed hooks merge named", output)
	}
	if got := string(readFileBytes(t, hooksPath)); got != malformed {
		t.Fatalf("hooks.json = %q after refused apply, want it preserved as %q", got, malformed)
	}
}
