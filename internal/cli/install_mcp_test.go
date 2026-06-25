package cli

import (
	"path/filepath"
	"testing"
)

func TestInstallMcpDetectionFindsProjectConfiguredTargets(t *testing.T) {
	root := realpath(t, t.TempDir())
	home := filepath.Join(root, "home")
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	t.Setenv("CODEX_HOME", filepath.Join(home, ".codex"))

	writeInstallFile(t, filepath.Join(root, ".cursor", "mcp.json"), `{"mcpServers":{"linear":{"command":"npx","args":["-y","mcp-remote","https://mcp.linear.app/mcp"]}}}`+"\n")
	writeInstallFile(t, filepath.Join(root, "opencode.json"), `{"mcp":{"serena":{"type":"local","command":["uvx","serena","start-mcp-server"]}}}`+"\n")
	writeInstallFile(t, filepath.Join(root, ".amp", "settings.json"), `{"amp":{"mcpServers":{"linear":{"command":"npx","args":["-y","mcp-remote","https://mcp.linear.app/mcp"]}}}}`+"\n")
	writeInstallFile(t, filepath.Join(root, ".codex", "config.toml"), "[mcp_servers.linear]\ncommand = \"npx\"\nargs = [\"-y\", \"mcp-remote\", \"https://mcp.linear.app/mcp\"]\n")

	for _, tc := range []struct {
		target string
		mcpID  string
	}{
		{target: "cursor", mcpID: "linear"},
		{target: "opencode", mcpID: "serena"},
		{target: "amp", mcpID: "linear"},
		{target: "codex", mcpID: "linear"},
	} {
		t.Run(tc.target+"/"+tc.mcpID, func(t *testing.T) {
			status := detectInstallMcpForTarget(root, tc.target, tc.mcpID)
			if !status.configured || status.scope != "project" {
				t.Fatalf("status = %#v, want project configured", status)
			}
		})
	}
}

func TestInstallMcpDoneForTargetsRequiresEveryRequestedTarget(t *testing.T) {
	statuses := map[string]installMcpTargetStatus{
		"claude-code": {configured: true, scope: "global"},
		"cursor":      {configured: false},
	}

	if installMcpDoneForTargets(statuses, []string{"claude-code", "cursor"}) {
		t.Fatalf("installMcpDoneForTargets returned true with cursor unconfigured")
	}
	if !installMcpDoneForTargets(statuses, []string{"claude-code"}) {
		t.Fatalf("installMcpDoneForTargets returned false with all requested targets configured")
	}
	if installMcpDoneForTargets(statuses, nil) {
		t.Fatalf("installMcpDoneForTargets returned true with no targets")
	}
}

func TestInstallMcpSerenaUsesNativeTargetArgs(t *testing.T) {
	serena := installMcpDefinitions[1]
	if serena.id != "serena" {
		t.Fatalf("definition = %#v, want serena definition", serena)
	}

	for _, tc := range []struct {
		target string
		want   []string
	}{
		{target: "claude-code", want: []string{"serena", "start-mcp-server", "--context", "claude-code", "--project-from-cwd"}},
		{target: "codex", want: []string{"serena", "start-mcp-server", "--context", "codex", "--project-from-cwd"}},
		{target: "opencode", want: []string{"serena", "start-mcp-server", "--project-from-cwd"}},
	} {
		t.Run(tc.target, func(t *testing.T) {
			got := installMcpArgs(serena, tc.target)
			if len(got) != len(tc.want) {
				t.Fatalf("args = %#v, want %#v", got, tc.want)
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Fatalf("args = %#v, want %#v", got, tc.want)
				}
			}
		})
	}
}
