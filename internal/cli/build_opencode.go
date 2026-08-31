package cli

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func runNativeBuildOpenCode(root string, out io.Writer) error {
	start := time.Now()
	fmt.Fprintf(out, "\n%s\n\n", ansiBold("loaf build"))

	sharedStart := time.Now()
	fmt.Fprintf(out, "  %s shared skills intermediate...", ansiCyan("building"))
	if err := buildNativeSharedSkillsIntermediate(root); err != nil {
		fmt.Fprintf(out, "\r  %s shared skills intermediate\n", ansiRed("✗"))
		return err
	}
	fmt.Fprintf(out, "\r  %s shared skills intermediate %s\n", ansiGreen("✓"), ansiGray("("+elapsedSeconds(sharedStart)+")"))

	targetStart := time.Now()
	fmt.Fprintf(out, "  %s opencode...", ansiCyan("building"))
	if err := buildNativeOpenCodeTarget(root); err != nil {
		fmt.Fprintf(out, "\r  %s opencode\n", ansiRed("✗"))
		return err
	}
	fmt.Fprintf(out, "\r  %s opencode %s\n", ansiGreen("✓"), ansiGray("("+elapsedSeconds(targetStart)+")"))
	fmt.Fprintln(out)
	fmt.Fprintf(out, "%s %s\n", ansiGreen("Build complete"), ansiGray("("+elapsedSeconds(start)+")"))
	return nil
}

func buildNativeOpenCodeTarget(root string) error {
	version, err := nativeBuildPackageVersion(root)
	if err != nil {
		return err
	}
	targetsConfig, err := readNativeBuildTargetsConfig(root)
	if err != nil {
		return err
	}
	dist := filepath.Join(root, "dist", "opencode")
	if err := os.RemoveAll(dist); err != nil {
		return err
	}
	srcDir := filepath.Join(root, "content")
	if err := copyNativeBuildSkills(nativeBuildSkillCopyOptions{
		srcDir:        filepath.Join(root, "dist"),
		destDir:       filepath.Join(dist, "skills"),
		sidecarSrcDir: srcDir,
		targetName:    "opencode",
		version:       version,
		targetsConfig: targetsConfig,
	}); err != nil {
		return err
	}
	if err := copyNativeBuildAgents(srcDir, filepath.Join(dist, "agents"), "opencode", version, nil, false); err != nil {
		return err
	}
	if err := generateNativeOpenCodeCommands(root, version); err != nil {
		return err
	}
	if err := generateNativeOpenCodePlugin(filepath.Join(root, "config", "hooks.yaml"), dist, version); err != nil {
		return err
	}
	if err := copyNativeBuildDir(filepath.Join(srcDir, "hooks"), filepath.Join(dist, "plugins", "hooks"), nil, false); err != nil {
		return err
	}
	retired := filepath.Join(dist, "plugins", "hooks", "pre-tool", "orchestration-detect-linear-magic.py")
	if err := os.Remove(retired); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove retired hook %s: %w", retired, err)
	}
	return nil
}

func generateNativeOpenCodeCommands(root string, version string) error {
	skillsSrc := filepath.Join(root, "dist", "skills")
	sidecarsSrc := filepath.Join(root, "content", "skills")
	commandsDest := filepath.Join(root, "dist", "opencode", "commands")
	entries, err := os.ReadDir(skillsSrc)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		skill := entry.Name()
		invocable, err := isNativeOpenCodeCommandSkill(sidecarsSrc, skill)
		if err != nil {
			return err
		}
		if !invocable {
			continue
		}
		sidecarPath := filepath.Join(sidecarsSrc, skill, "SKILL.opencode.yaml")
		sidecarFields, err := readNativeBuildAgentSidecar(sidecarPath, false)
		if err != nil {
			return err
		}
		skillPath := filepath.Join(skillsSrc, skill, "SKILL.md")
		body, err := readRegularFileNoFollow(skillPath, projectFileReadLimit)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return err
		}
		frontmatter, content := splitNativeBuildFrontmatter(string(body))
		sourceFields := parseNativeBuildYAMLFieldValues(frontmatter)
		fields := []nativeBuildYAMLFieldValue{
			{key: "description", value: nativeBuildStringValue(firstNativeBuildFieldString(sourceFields, "description", ""))},
		}
		for _, field := range sidecarFields {
			fields = setNativeBuildYAMLFieldValue(fields, field.key, field.value)
		}
		fields = setNativeBuildYAMLFieldValue(fields, "version", nativeBuildStringValue(version))
		// Keep skill-local ](templates/ and ](references/ links as stable
		// placeholders. Build-time ../skills/<name>/ assumes skills live under
		// the OpenCode config dir (false since ADR-018 moved them to
		// ~/.agents/skills). Install rewrites these once the real commands
		// directory and skills store are known (see rewriteOpenCodeCommandSkillLinks).
		output := "---\n" + renderNativeBuildYAMLFieldValues(fields) + "---\n" + content
		if err := os.MkdirAll(commandsDest, 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(commandsDest, skill+".md"), []byte(output), 0o644); err != nil {
			return err
		}
	}
	return nil
}

func isNativeOpenCodeCommandSkill(sidecarsSrc string, skill string) (bool, error) {
	fields, err := readNativeBuildAgentSidecar(filepath.Join(sidecarsSrc, skill, "SKILL.claude-code.yaml"), false)
	if err != nil {
		return false, err
	}
	for _, field := range fields {
		if field.key == "user-invocable" && field.value.kind == "bool" {
			return field.value.scalar == "true", nil
		}
	}
	return false, nil
}

func generateNativeOpenCodePlugin(hooksPath string, dist string, version string) error {
	hooks, err := readNativeBuildHooks(hooksPath)
	if err != nil {
		return err
	}
	pluginDir := filepath.Join(dist, "plugins")
	if err := os.MkdirAll(pluginDir, 0o755); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(pluginDir, "hooks.ts"), []byte(renderNativeOpenCodePlugin(hooks, version)), 0o644)
}

func renderNativeOpenCodePlugin(hooks []nativeBuildHook, version string) string {
	return nativeOpenCodeHeader(version) + "\n\n" +
		nativeAmpCoreFunctions() + "\n\n" +
		nativeAmpHookData(retargetNativeOpenCodeSessionStart(hooks)) + "\n\n" +
		nativeOpenCodeSessionHelpers() + "\n\n" +
		"export default async function AgentSkillsPlugin({ client, $ }: { client: OpenCodeClient; $?: unknown }) {\n  void $;\n  return {\n" +
		nativeOpenCodePluginBody() + "\n  };\n}"
}

// retargetNativeOpenCodeSessionStart points the shared SessionStart hook at
// OpenCode's own dispatch variant, the way the Claude Code, Cursor, and Codex
// builders point it at theirs. config/hooks.yaml keeps the neutral command as
// the declaration; only the harness that has an adapter renders the adapter.
func retargetNativeOpenCodeSessionStart(hooks []nativeBuildHook) []nativeBuildHook {
	retargeted := make([]nativeBuildHook, 0, len(hooks))
	for _, hook := range hooks {
		if hook.section == "session" && hook.id == "session-start-loaf" && hook.event == "SessionStart" {
			hook.command = "loaf journal context --from-hook --opencode-hook"
		}
		retargeted = append(retargeted, hook)
	}
	return retargeted
}

func nativeOpenCodeHeader(version string) string {
	return `/**
 * OpenCode Plugin - Agent Skills Hooks
 * Auto-generated by loaf build system
 * @version ` + version + `
 */

import { execFile } from 'child_process';
import { promisify } from 'util';
import { join, dirname } from 'path';
import { fileURLToPath } from 'url';

const __dirname = dirname(fileURLToPath(import.meta.url));
const execFileAsync = promisify(execFile);`
}

func nativeOpenCodeSessionHelpers() string {
	return `type OpenCodeClient = {
  session: {
    get(input: { path: { id: string } }): Promise<{ data?: { parentID?: string } }>;
  };
};

const openCodeSessionLookupWarning = '[loaf] OpenCode session lookup unavailable; context delivery suppressed';

function normalizeOpenCodeToolName(toolName: string): string {
  switch (toolName) {
    case 'bash':
      return 'Bash';
    case 'edit':
      return 'Edit';
    case 'write':
      return 'Write';
    default:
      return toolName;
  }
}

async function isOpenCodeRootSession(client: OpenCodeClient, sessionID: string): Promise<boolean> {
  if (!sessionID) return false;
  try {
    const response = await client.session.get({ path: { id: sessionID } });
    if (!response || typeof response !== 'object' || !response.data || typeof response.data !== 'object' || Array.isArray(response.data)) {
      console.warn(openCodeSessionLookupWarning);
      return false;
    }
    const data = response.data as { parentID?: unknown };
    if ('parentID' in data && data.parentID !== undefined) {
      if (typeof data.parentID !== 'string') {
        console.warn(openCodeSessionLookupWarning);
        return false;
      }
      return false;
    }
    return true;
  } catch {
    console.warn(openCodeSessionLookupWarning);
    return false;
  }
}

function serializeOpenCodeLifecyclePayload(sessionID: string, lifecycleEvent: string): string {
  return JSON.stringify({
    target: 'opencode',
    session_id: sessionID,
    lifecycle_event: lifecycleEvent,
  });
}

async function runOpenCodeSessionHooks(hooks: HookEntry[] | undefined, sessionID: string, lifecycleEvent: string, output: string[]): Promise<void> {
  if (!hooks) return;
  const hookPayload = serializeOpenCodeLifecyclePayload(sessionID, lifecycleEvent);
  for (const hook of hooks) {
    const result = await runHook('session', 'session', hook.id, hook.command, hook.script, hookPayload, hook.timeout, hook.failClosed);
    if (result.exitCode === 0) {
      const stdout = result.stdout.trim();
      if (stdout) output.push(stdout);
      continue;
    }
    console.warn('[loaf] OpenCode ' + lifecycleEvent + ' hook ' + hook.id + ' failed (exit ' + result.exitCode + '); context delivery continued');
  }
}`
}

func nativeOpenCodePluginBody() string {
	body := `    // Pre-tool hook handler
    'tool.execute.before': async (input: { tool: string; sessionID: string; callID: string }, output: { args: unknown }) => {
      const toolName = normalizeOpenCodeToolName(input.tool);
      const toolInput = output.args;
      if (!toolName) return;

      const hookPayload = serializeHookPayload(toolName, toolInput, { input, output });

      for (const [matcher, hookList] of Object.entries(preToolHooks)) {
        if (matchesTool(toolName, matcher)) {
          for (const hook of hookList) {
            if (!matchesIfCondition(toolName, toolInput, hook.if)) continue;
            const result = await runHook('pre-tool', toolName, hook.id, hook.command, hook.script, hookPayload, hook.timeout, hook.failClosed);

            // Exit code 2 = block the action
            if (result.exitCode === 2) {
              throw new Error(result.stderr);
            }

            // Log errors for debugging
            if (result.exitCode === 1) {
              console.warn(%%BT%%[loaf] Hook ${hook.id} error: ${result.stderr}%%BT%%);
            }
          }
        }
      }
    },

    // Post-tool hook handler
    'tool.execute.after': async (input: { tool: string; sessionID: string; callID: string; args: unknown }, output: { title?: string; output?: string; metadata?: unknown }) => {
      const toolName = normalizeOpenCodeToolName(input.tool);
      const toolInput = input.args;
      if (!toolName) return;

      const hookPayload = serializeHookPayload(toolName, toolInput, { input, output });

      for (const [matcher, hookList] of Object.entries(postToolHooks)) {
        if (matchesTool(toolName, matcher)) {
          for (const hook of hookList) {
            if (!matchesIfCondition(toolName, toolInput, hook.if)) continue;
            const result = await runHook('post-tool', toolName, hook.id, hook.command, hook.script, hookPayload, hook.timeout, hook.failClosed);

            if (result.exitCode !== 0) {
              console.warn(%%BT%%[loaf] Post-hook ${hook.id} error (exit ${result.exitCode}): ${result.stderr}%%BT%%);
            }
          }
        }
      }
    },

    // Request and compaction context handlers
    'experimental.chat.system.transform': async (input: { sessionID?: string; model?: unknown }, output: { system: string[] }) => {
      const sessionID = input?.sessionID;
      if (!sessionID || !(await isOpenCodeRootSession(client, sessionID))) return;
      await runOpenCodeSessionHooks(sessionHooks.sessionstart, sessionID, 'system.transform', output.system);
    },

    'experimental.session.compacting': async (input: { sessionID: string }, output: { context: string[]; prompt?: string }) => {
      const sessionID = input?.sessionID;
      if (!sessionID || !(await isOpenCodeRootSession(client, sessionID))) return;
      await runOpenCodeSessionHooks(sessionHooks.postcompact, sessionID, 'session.compacting', output.context);
    }`
	return strings.ReplaceAll(body, "%%BT%%", "`")
}
