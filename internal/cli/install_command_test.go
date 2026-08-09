package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunnerInstallExplicitCursorTargetRunsNatively(t *testing.T) {
	root, home := setupInstallCommandFixture(t)
	writeInstallFile(t, filepath.Join(root, "dist", "cursor", "skills", "foundations", "SKILL.md"), "# Foundations\n")
	installTestHookDistribution(t, root, "cursor")

	var stdout bytes.Buffer
	err := Runner{
		Stdout:     &stdout,
		WorkingDir: root,
		Executable: distributionFixtureExecutable(root),
	}.Run([]string{"install", "--to", "cursor", "--yes"})
	if err != nil {
		t.Fatalf("install --to cursor error = %v\n%s", err, stdout.String())
	}

	if strings.Contains(stdout.String(), "args=install") {
		t.Fatalf("stdout = %q, want native install without legacy delegation", stdout.String())
	}
	if !strings.Contains(stdout.String(), "Cursor installed") {
		t.Fatalf("stdout = %q, want Cursor install summary", stdout.String())
	}
	assertInstallFile(t, filepath.Join(home, ".cursor", loafInstallMarkerFile), "9.8.7-test.1\n")
	assertInstallFile(t, filepath.Join(home, ".agents", "skills", "foundations", "SKILL.md"), "# Foundations\n")
	if info, statErr := os.Lstat(filepath.Join(root, "AGENTS.md")); statErr != nil || info.Mode()&os.ModeSymlink != 0 {
		t.Fatalf("root AGENTS.md must be a real file: info=%v err=%v", info, statErr)
	}
	canonical := string(readFileBytes(t, filepath.Join(root, "AGENTS.md")))
	if !strings.Contains(canonical, "## Loaf Framework") || !strings.Contains(canonical, "<!-- loaf:managed:start sha256=") || strings.Contains(canonical, "<!-- loaf:managed:start v") {
		t.Fatalf("canonical AGENTS.md = %q, want native fenced section with sha256-only marker", canonical)
	}
	config := readInstallCommandJSON(t, filepath.Join(root, ".agents", "loaf.json"))
	integrations := config["integrations"].(map[string]any)
	if integrations["linear"].(map[string]any)["enabled"] != false || integrations["serena"].(map[string]any)["enabled"] != false {
		t.Fatalf("integrations = %#v, want non-interactive MCP defaults disabled", integrations)
	}
}

func TestRunnerInstallUsesAgentsHomeSkillDestinations(t *testing.T) {
	for _, target := range []string{"opencode", "cursor", "codex", "amp"} {
		t.Run(target, func(t *testing.T) {
			root, home := setupInstallCommandFixture(t)
			writeInstallFile(t, filepath.Join(root, "dist", target, "skills", "foundations", "SKILL.md"), "# Foundations\n")
			if target == "cursor" || target == "codex" {
				installTestHookDistribution(t, root, target)
			}

			var stdout bytes.Buffer
			err := Runner{Stdout: &stdout, WorkingDir: root, Executable: distributionFixtureExecutable(root)}.Run([]string{"install", "--to", target, "--yes"})
			if err != nil {
				t.Fatalf("install --to %s error = %v\n%s", target, err, stdout.String())
			}

			assertInstallFile(t, filepath.Join(home, ".agents", "skills", "foundations", "SKILL.md"), "# Foundations\n")
			record := readInstallCommandJSON(t, installRecordPath(home, target))
			if record["target"] != target || record["skills_dir"] != filepath.Join(home, ".agents", "skills") {
				t.Fatalf("record = %#v, want target and shared skills dir", record)
			}
			switch target {
			case "opencode":
				assertInstallPathMissing(t, filepath.Join(home, ".config", "opencode", "skills", "foundations"))
			case "amp":
				assertInstallPathMissing(t, filepath.Join(home, ".config", "agents", "skills", "foundations"))
			}
		})
	}
}

func TestRunnerInstallSharedSkillsPreservesForeignEntries(t *testing.T) {
	root, home := setupInstallCommandFixture(t)
	sharedSkills := filepath.Join(home, ".agents", "skills")
	writeInstallFile(t, filepath.Join(sharedSkills, "foreign-skill", "SKILL.md"), "# Mine\n")
	writeInstallFile(t, filepath.Join(root, "dist", "cursor", "skills", "foundations", "SKILL.md"), "# Foundations\n")
	installTestHookDistribution(t, root, "cursor")

	var stdout bytes.Buffer
	err := Runner{Stdout: &stdout, WorkingDir: root, Executable: distributionFixtureExecutable(root)}.Run([]string{"install", "--to", "cursor", "--yes"})
	if err != nil {
		t.Fatalf("install --to cursor error = %v\n%s", err, stdout.String())
	}
	assertInstallFile(t, filepath.Join(sharedSkills, "foreign-skill", "SKILL.md"), "# Mine\n")
	assertInstallFile(t, filepath.Join(sharedSkills, "foundations", "SKILL.md"), "# Foundations\n")

	if err := os.RemoveAll(filepath.Join(root, "dist", "cursor", "skills", "foundations")); err != nil {
		t.Fatalf("RemoveAll(foundations) error = %v", err)
	}
	writeInstallFile(t, filepath.Join(root, "dist", "cursor", "skills", "go-development", "SKILL.md"), "# Go\n")
	stdout.Reset()
	err = Runner{Stdout: &stdout, WorkingDir: root, Executable: distributionFixtureExecutable(root)}.Run([]string{"install", "--to", "cursor", "--yes"})
	if err != nil {
		t.Fatalf("second install --to cursor error = %v\n%s", err, stdout.String())
	}
	assertInstallPathMissing(t, filepath.Join(sharedSkills, "foundations"))
	assertInstallFile(t, filepath.Join(sharedSkills, "foreign-skill", "SKILL.md"), "# Mine\n")
	assertInstallFile(t, filepath.Join(sharedSkills, "go-development", "SKILL.md"), "# Go\n")
}

func TestRunnerInstallRecordKeepsRelocatedTargetDetectable(t *testing.T) {
	root, home := setupInstallCommandFixture(t)
	writeInstallFile(t, filepath.Join(root, "dist", "cursor", "skills", "foundations", "SKILL.md"), "# Foundations\n")
	installTestHookDistribution(t, root, "cursor")

	var stdout bytes.Buffer
	err := Runner{Stdout: &stdout, WorkingDir: root, Executable: distributionFixtureExecutable(root)}.Run([]string{"install", "--to", "cursor", "--yes"})
	if err != nil {
		t.Fatalf("install --to cursor error = %v\n%s", err, stdout.String())
	}
	if err := os.Remove(filepath.Join(home, ".cursor", loafInstallMarkerFile)); err != nil {
		t.Fatalf("Remove(marker) error = %v", err)
	}
	if !isLoafInstalledForTargetInstall("cursor", filepath.Join(home, ".cursor")) {
		t.Fatal("cursor install not detected from shared install record")
	}
}

func TestRunnerInstallCodexUsesCodeXHomeNatively(t *testing.T) {
	root, home := setupInstallCommandFixture(t)
	codexHome := filepath.Join(home, "custom-codex")
	t.Setenv("CODEX_HOME", codexHome)
	writeInstallFile(t, filepath.Join(root, "dist", "codex", "skills", "go-development", "SKILL.md"), "# Go\n")
	installTestHookDistribution(t, root, "codex")

	var stdout bytes.Buffer
	err := Runner{Stdout: &stdout, WorkingDir: root, Executable: distributionFixtureExecutable(root)}.Run([]string{"install", "--to", "codex", "--yes"})
	if err != nil {
		t.Fatalf("install --to codex error = %v\n%s", err, stdout.String())
	}
	assertInstallFile(t, filepath.Join(codexHome, loafInstallMarkerFile), "9.8.7-test.1\n")
	assertInstallFile(t, filepath.Join(home, ".agents", "skills", "go-development", "SKILL.md"), "# Go\n")
	// CODEX_HOME is where the reconciler converges too, not just where the
	// marker lands: the entry it projects has to reach the same relocated root.
	hooks := readInstallHooks(t, filepath.Join(codexHome, "hooks.json"))
	if len(hooks.Hooks["SessionStart"]) != 1 {
		t.Fatalf("codex hooks = %#v, want the SessionStart entry projected under CODEX_HOME", hooks)
	}
}

func TestRunnerInstallCodexBasicCommandsFailsWhenCapabilityCannotBeInstalled(t *testing.T) {
	root, home := setupInstallCommandFixture(t)
	codexHome := filepath.Join(home, "custom-codex")
	t.Setenv("CODEX_HOME", codexHome)
	writeInstallFile(t, filepath.Join(root, "dist", "codex", "skills", "go-development", "SKILL.md"), "# Go\n")
	installTestHookDistribution(t, root, "codex")
	writeInstallFile(t, filepath.Join(root, "dist", "codex", ".codex", "rules", "loaf.rules.tmpl"), "# Loaf Codex policy\n{{LOAF_BASIC_RULES}}\n")

	var stdout bytes.Buffer
	err := Runner{Stdout: &stdout, WorkingDir: root, Executable: distributionFixtureExecutable(root)}.Run([]string{"install", "--to", "codex", "--codex-basic-commands", "--yes"})
	if err == nil || !strings.Contains(err.Error(), "not on PATH") {
		t.Fatalf("Codex basic command policy install error = %v, want visible executable trust failure\n%s", err, stdout.String())
	}
	assertInstallPathMissing(t, filepath.Join(codexHome, "rules", "loaf.rules"))
	assertInstallPathMissing(t, filepath.Join(codexHome, loafInstallMarkerFile))
}

func TestRunnerInstallOffersBinarySelfInstall(t *testing.T) {
	root, home := setupInstallCommandFixture(t)
	pathBin := filepath.Join(root, "path-bin")
	mkdirAll(t, pathBin)
	t.Setenv("PATH", pathBin)
	writeInstallFile(t, filepath.Join(root, "bin", "loaf"), "#!/bin/sh\nprintf 'loaf %s\\n' \"$*\"\n")
	if err := os.Chmod(filepath.Join(root, "bin", "loaf"), 0o755); err != nil {
		t.Fatalf("Chmod(source loaf) error = %v", err)
	}
	writeInstallFile(t, filepath.Join(root, "bin", "native", "darwin-arm64", "loaf"), "native\n")
	writeInstallFile(t, filepath.Join(root, "dist", "cursor", "skills", "foundations", "SKILL.md"), "# Foundations\n")
	installTestHookDistribution(t, root, "cursor")
	mkdirAll(t, filepath.Join(home, ".cursor"))

	var stdout bytes.Buffer
	err := Runner{
		Stdout:     &stdout,
		Stdin:      strings.NewReader("y\n\n\n"),
		WorkingDir: root,
		Executable: distributionFixtureExecutable(root),
	}.Run([]string{"install", "--to", "cursor", "--yes"})
	if err != nil {
		t.Fatalf("install --to cursor with binary prompt error = %v\n%s", err, stdout.String())
	}
	if !strings.Contains(stdout.String(), "Install 'loaf' binary to ~/.local/bin?") || !strings.Contains(stdout.String(), "Installed loaf binary") {
		t.Fatalf("stdout = %q, want binary install prompt and success", stdout.String())
	}
	assertInstallFile(t, filepath.Join(home, ".local", "bin", "loaf"), "#!/bin/sh\nprintf 'loaf %s\\n' \"$*\"\n")
	assertInstallFile(t, filepath.Join(home, ".local", "bin", "native", "darwin-arm64", "loaf"), "native\n")
	assertInstallPathMissing(t, filepath.Join(home, ".local", "share", "loaf", "dist-cli", "index.js"))
}

func TestRunnerInstallInteractiveSelectionRunsNatively(t *testing.T) {
	root, home := setupInstallCommandFixture(t)
	writeInstallFile(t, filepath.Join(root, "dist", "cursor", "skills", "foundations", "SKILL.md"), "# Foundations\n")
	installTestHookDistribution(t, root, "cursor")
	mkdirAll(t, filepath.Join(home, ".cursor"))

	var stdout bytes.Buffer
	err := Runner{
		Stdout:     &stdout,
		Stdin:      strings.NewReader("y\n"),
		WorkingDir: root,
		Executable: distributionFixtureExecutable(root),
	}.Run([]string{"install"})
	if err != nil {
		t.Fatalf("interactive install error = %v\n%s", err, stdout.String())
	}
	if strings.Contains(stdout.String(), "args=install") {
		t.Fatalf("stdout = %q, want native interactive install without legacy delegation", stdout.String())
	}
	if !strings.Contains(stdout.String(), "Install to") || !strings.Contains(stdout.String(), "Cursor installed") {
		t.Fatalf("stdout = %q, want prompts and cursor install", stdout.String())
	}
	assertInstallFile(t, filepath.Join(home, ".cursor", loafInstallMarkerFile), "9.8.7-test.1\n")
}

func TestRunnerInstallInteractiveNoTargetsStillUpdatesClaudeProjectFile(t *testing.T) {
	root, home := setupInstallCommandFixture(t)
	writeInstallFile(t, filepath.Join(root, "dist", "cursor", "skills", "foundations", "SKILL.md"), "# Foundations\n")
	installTestHookDistribution(t, root, "cursor")
	mkdirAll(t, filepath.Join(home, ".cursor"))
	writeInstallFile(t, filepath.Join(root, "bin", "claude"), "#!/bin/sh\nexit 0\n")
	if err := os.Chmod(filepath.Join(root, "bin", "claude"), 0o755); err != nil {
		t.Fatalf("Chmod(fake claude) error = %v", err)
	}

	var stdout bytes.Buffer
	err := Runner{
		Stdout:     &stdout,
		Stdin:      strings.NewReader("n\n"),
		WorkingDir: root,
		Executable: distributionFixtureExecutable(root),
	}.Run([]string{"install", "--yes"})
	if err != nil {
		t.Fatalf("interactive no-target install error = %v\n%s", err, stdout.String())
	}
	if !strings.Contains(stdout.String(), "No targets selected") {
		t.Fatalf("stdout = %q, want no targets selected", stdout.String())
	}
	canonical := filepath.Join(root, "AGENTS.md")
	assertInstallSymlinkTarget(t, filepath.Join(root, ".claude", "CLAUDE.md"), canonical)
	body := string(readFileBytes(t, canonical))
	if !strings.Contains(body, "## Loaf Framework") {
		t.Fatalf("canonical body = %q, want Claude project fenced section", body)
	}
}

func TestRunnerInstallMcpRecommendationWritesCursorProjectConfig(t *testing.T) {
	root, home := setupInstallCommandFixture(t)
	writeInstallFile(t, filepath.Join(root, "bin", "loaf"), "#!/bin/sh\nexit 0\n")
	if err := os.Chmod(filepath.Join(root, "bin", "loaf"), 0o755); err != nil {
		t.Fatalf("Chmod(fake loaf) error = %v", err)
	}
	writeInstallFile(t, filepath.Join(root, "dist", "cursor", "skills", "foundations", "SKILL.md"), "# Foundations\n")
	installTestHookDistribution(t, root, "cursor")
	writeInstallFile(t, filepath.Join(root, ".cursor", "mcp.json"), `{"mcpServers":{"existing":{"command":"keep","args":[]}},"theme":"dark"}`+"\n")
	mkdirAll(t, filepath.Join(home, ".cursor"))

	var stdout bytes.Buffer
	err := Runner{
		Stdout:     &stdout,
		Stdin:      strings.NewReader("p\nn\n"),
		WorkingDir: root,
		Executable: distributionFixtureExecutable(root),
	}.Run([]string{"install", "--to", "cursor", "--yes"})
	if err != nil {
		t.Fatalf("install --to cursor with MCP prompt error = %v\n%s", err, stdout.String())
	}
	if !strings.Contains(stdout.String(), "Recommended MCP servers") || !strings.Contains(stdout.String(), "cursor: merged into .cursor/mcp.json") {
		t.Fatalf("stdout = %q, want MCP recommendation output", stdout.String())
	}
	mcp := readInstallCommandJSON(t, filepath.Join(root, ".cursor", "mcp.json"))
	if mcp["theme"] != "dark" {
		t.Fatalf("mcp config = %#v, want existing sibling key preserved", mcp)
	}
	servers := mcp["mcpServers"].(map[string]any)
	if _, ok := servers["existing"]; !ok {
		t.Fatalf("mcp servers = %#v, want existing server preserved", servers)
	}
	linear := servers["linear"].(map[string]any)
	if linear["command"] != "npx" {
		t.Fatalf("linear server = %#v, want command npx", linear)
	}
	args := linear["args"].([]any)
	if len(args) != 3 || args[2] != "https://mcp.linear.app/mcp" {
		t.Fatalf("linear args = %#v, want mcp-remote URL", args)
	}
	config := readInstallCommandJSON(t, filepath.Join(root, ".agents", "loaf.json"))
	integrations := config["integrations"].(map[string]any)
	if integrations["linear"].(map[string]any)["enabled"] != true || integrations["serena"].(map[string]any)["enabled"] != false {
		t.Fatalf("integrations = %#v, want linear enabled and serena disabled", integrations)
	}
}

func TestRunnerInstallMcpRecommendationOffersSerenaNativeInstall(t *testing.T) {
	root, home := setupInstallCommandFixture(t)
	bin := filepath.Join(root, "bin")
	logPath := filepath.Join(root, "serena-install.log")
	writeInstallFile(t, filepath.Join(bin, "loaf"), "#!/bin/sh\nexit 0\n")
	writeInstallFile(t, filepath.Join(bin, "uv"), fmt.Sprintf(`#!/bin/sh
echo "uv $*" >> %q
/bin/cat > %q <<'EOS'
#!/bin/sh
echo "serena $*" >> %q
exit 0
EOS
/bin/chmod +x %q
`, logPath, filepath.Join(bin, "serena"), logPath, filepath.Join(bin, "serena")))
	if err := os.Chmod(filepath.Join(bin, "loaf"), 0o755); err != nil {
		t.Fatalf("Chmod(fake loaf) error = %v", err)
	}
	if err := os.Chmod(filepath.Join(bin, "uv"), 0o755); err != nil {
		t.Fatalf("Chmod(fake uv) error = %v", err)
	}
	writeInstallFile(t, filepath.Join(root, "dist", "cursor", "skills", "foundations", "SKILL.md"), "# Foundations\n")
	installTestHookDistribution(t, root, "cursor")
	mkdirAll(t, filepath.Join(home, ".cursor"))

	var stdout bytes.Buffer
	err := Runner{
		Stdout:     &stdout,
		Stdin:      strings.NewReader("n\np\ny\n"),
		WorkingDir: root,
		Executable: distributionFixtureExecutable(root),
	}.Run([]string{"install", "--to", "cursor", "--yes"})
	if err != nil {
		t.Fatalf("install --to cursor with Serena prompt error = %v\n%s", err, stdout.String())
	}
	if !strings.Contains(stdout.String(), "Serena must be installed natively") || !strings.Contains(stdout.String(), "Serena native CLI installed") {
		t.Fatalf("stdout = %q, want Serena prerequisite install output", stdout.String())
	}
	log := string(readFileBytes(t, logPath))
	if !strings.Contains(log, "uv tool install -p 3.13 serena-agent@latest --prerelease=allow") || !strings.Contains(log, "serena init") {
		t.Fatalf("serena install log = %q, want uv install and serena init", log)
	}
	mcp := readInstallCommandJSON(t, filepath.Join(root, ".cursor", "mcp.json"))
	servers := mcp["mcpServers"].(map[string]any)
	serena := servers["serena"].(map[string]any)
	if serena["command"] != "serena" {
		t.Fatalf("serena server = %#v, want serena command", serena)
	}
	args := serena["args"].([]any)
	if len(args) != 4 || args[1] != "--context" || args[2] != "ide" {
		t.Fatalf("serena args = %#v, want Cursor Serena context", args)
	}
	config := readInstallCommandJSON(t, filepath.Join(root, ".agents", "loaf.json"))
	integrations := config["integrations"].(map[string]any)
	if integrations["linear"].(map[string]any)["enabled"] != false || integrations["serena"].(map[string]any)["enabled"] != true {
		t.Fatalf("integrations = %#v, want linear disabled and serena enabled", integrations)
	}
}

func TestInstallMcpConfigWritersHandleOpenCodeAndNestedAmp(t *testing.T) {
	root := realpath(t, t.TempDir())
	opencodePath := filepath.Join(root, "opencode.json")
	if err := mergeOpenCodeMcpConfig(opencodePath, "linear", []string{"npx", "-y", "mcp-remote", "https://mcp.linear.app/mcp"}); err != nil {
		t.Fatalf("mergeOpenCodeMcpConfig error = %v", err)
	}
	opencode := readInstallCommandJSON(t, opencodePath)
	openServers := opencode["mcp"].(map[string]any)
	openLinear := openServers["linear"].(map[string]any)
	if openLinear["type"] != "local" || openLinear["enabled"] != true {
		t.Fatalf("opencode linear = %#v, want local enabled server", openLinear)
	}
	command := openLinear["command"].([]any)
	if len(command) != 4 || command[0] != "npx" {
		t.Fatalf("opencode command = %#v, want command array", command)
	}

	ampPath := filepath.Join(root, ".amp", "settings.json")
	writeInstallFile(t, ampPath, `{"amp":{"theme":"quiet"}}`+"\n")
	if err := mergeJSONMcpConfig(ampPath, "amp.mcpServers", "linear", []string{"npx", "-y", "mcp-remote", "https://mcp.linear.app/mcp"}); err != nil {
		t.Fatalf("mergeJSONMcpConfig(amp) error = %v", err)
	}
	amp := readInstallCommandJSON(t, ampPath)
	ampSection := amp["amp"].(map[string]any)
	if ampSection["theme"] != "quiet" {
		t.Fatalf("amp section = %#v, want existing nested key preserved", ampSection)
	}
	ampServers := ampSection["mcpServers"].(map[string]any)
	if ampServers["linear"].(map[string]any)["command"] != "npx" {
		t.Fatalf("amp servers = %#v, want nested linear server", ampServers)
	}
}

func TestRunnerInstallFromLinkedWorktreeWritesMainLoafConfig(t *testing.T) {
	requireCLIGit(t)
	main := initCLIGitRepo(t)
	home := filepath.Join(main, "home")
	bin := filepath.Join(main, "bin")
	originalPath := os.Getenv("PATH")
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	t.Setenv("PATH", bin+string(os.PathListSeparator)+originalPath)
	mkdirAll(t, bin)
	mkdirAll(t, filepath.Join(home, ".cursor"))
	writeInstallFile(t, filepath.Join(main, "package.json"), `{"name":"loaf","version":"9.8.7-test.1"}`+"\n")
	writeInstallFile(t, filepath.Join(main, "dist", "cursor", "skills", "foundations", "SKILL.md"), "# Foundations\n")
	installTestHookDistribution(t, main, "cursor")
	gitCLI(t, main, "add", "package.json", "dist/cursor/skills/foundations/SKILL.md")
	gitCLI(t, main, "-c", "user.name=Loaf Test", "-c", "user.email=loaf@example.test", "-c", "commit.gpgsign=false", "commit", "-m", "add install fixture")
	linked := addCLILinkedWorktree(t, main, "install-config")

	var stdout bytes.Buffer
	err := Runner{
		Stdout:     &stdout,
		WorkingDir: linked,
		Executable: distributionFixtureExecutable(main),
	}.Run([]string{"install", "--to", "cursor", "--yes"})
	if err != nil {
		t.Fatalf("install from linked worktree error = %v\n%s", err, stdout.String())
	}

	config := readInstallCommandJSON(t, filepath.Join(main, ".agents", "loaf.json"))
	integrations := config["integrations"].(map[string]any)
	if integrations["linear"].(map[string]any)["enabled"] != false || integrations["serena"].(map[string]any)["enabled"] != false {
		t.Fatalf("integrations = %#v, want defaults recorded in main worktree", integrations)
	}
	if _, err := os.Stat(filepath.Join(linked, ".agents", "loaf.json")); !os.IsNotExist(err) {
		t.Fatalf("linked loaf.json stat = %v, want no shadow config in linked worktree", err)
	}
}

func TestRunnerInstallHelpAndInvalidTargetAreNative(t *testing.T) {
	var helpOut bytes.Buffer
	if err := (Runner{Stdout: &helpOut, WorkingDir: t.TempDir()}).Run([]string{"install", "--help"}); err != nil {
		t.Fatalf("install --help error = %v", err)
	}
	if !strings.Contains(helpOut.String(), "Usage: loaf install") {
		t.Fatalf("help output = %q, want native install help", helpOut.String())
	}

	root, _ := setupInstallCommandFixture(t)
	err := (Runner{Stdout: &bytes.Buffer{}, WorkingDir: root, Executable: distributionFixtureExecutable(root)}).Run([]string{"install", "--to", "wat"})
	if err == nil || !strings.Contains(err.Error(), "unknown install target") {
		t.Fatalf("invalid target error = %v, want native unknown target error", err)
	}
}

func setupInstallCommandFixture(t *testing.T) (string, string) {
	t.Helper()
	root := realpath(t, t.TempDir())
	home := filepath.Join(root, "home")
	bin := filepath.Join(root, "bin")
	mkdirAll(t, home)
	mkdirAll(t, bin)
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	t.Setenv("CODEX_HOME", "")
	t.Setenv("PATH", bin)
	// The Loaf-repo detector reads the state database. Point it at a path that
	// will never exist so no fixture can reach the real global one.
	t.Setenv("LOAF_DB", filepath.Join(t.TempDir(), "loaf.sqlite"))
	writeInstallFile(t, filepath.Join(root, "package.json"), `{"name":"loaf","version":"9.8.7-test.1"}`+"\n")
	return root, home
}

func writeInstallDeprecationManifest(t *testing.T, root string, body string) {
	t.Helper()
	writeInstallFile(t, filepath.Join(root, "config", "deprecations.json"), body+"\n")
}

func readInstallCommandJSON(t *testing.T, path string) map[string]any {
	t.Helper()
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%s) error = %v", path, err)
	}
	var data map[string]any
	if err := json.Unmarshal(body, &data); err != nil {
		t.Fatalf("Unmarshal(%s) error = %v\n%s", path, err, body)
	}
	return data
}
