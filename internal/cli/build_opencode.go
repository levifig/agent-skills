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
		transformMd:   substituteNativeBuildCursorCommands,
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
	return copyNativeBuildDir(filepath.Join(srcDir, "hooks"), filepath.Join(dist, "plugins", "hooks"), nil, false)
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
		sidecarPath := filepath.Join(sidecarsSrc, skill, "SKILL.opencode.yaml")
		sidecarFields, err := readNativeBuildAgentSidecar(sidecarPath, false)
		if err != nil {
			return err
		}
		if len(sidecarFields) == 0 {
			continue
		}
		skillPath := filepath.Join(skillsSrc, skill, "SKILL.md")
		body, err := os.ReadFile(skillPath)
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
		content = strings.ReplaceAll(content, "](templates/", "](../skills/"+skill+"/templates/")
		content = strings.ReplaceAll(content, "](references/", "](../skills/"+skill+"/references/")
		output := "---\n" + renderNativeBuildYAMLFieldValues(fields) + "---\n" + substituteNativeBuildCursorCommands(content)
		if err := os.MkdirAll(commandsDest, 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(commandsDest, skill+".md"), []byte(output), 0o644); err != nil {
			return err
		}
	}
	return nil
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
		nativeAmpHookData(hooks) + "\n\n" +
		"export default async function AgentSkillsPlugin({ client, $ }) {\n  return {\n" +
		nativeOpenCodePluginBody() + "\n  };\n}"
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

func nativeOpenCodePluginBody() string {
	body := `    // Pre-tool hook handler
    'tool.execute.before': async (input) => {
      const toolName = input?.tool?.name;
      const toolInput = input?.tool?.input;
      if (!toolName) return;

      const hookPayload = serializeHookPayload(toolName, toolInput, input);

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
    'tool.execute.after': async (input) => {
      const toolName = input?.tool?.name;
      const toolInput = input?.tool?.input;
      if (!toolName) return;

      const hookPayload = serializeHookPayload(toolName, toolInput, input);

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
    // Session lifecycle event handler
    'event': async ({ event }) => {
      if (event.type === 'session.created' && sessionHooks.sessionstart) {
        for (const hook of sessionHooks.sessionstart) {
          await runHook('session', 'session', hook.id, hook.command, hook.script, undefined, hook.timeout, hook.failClosed);
        }
      }
      if (event.type === 'session.ended' && sessionHooks.sessionend) {
        for (const hook of sessionHooks.sessionend) {
          await runHook('session', 'session', hook.id, hook.command, hook.script, undefined, hook.timeout, hook.failClosed);
        }
      }
      if (event.type === 'context.compacting' && sessionHooks.precompact) {
        for (const hook of sessionHooks.precompact) {
          await runHook('session', 'session', hook.id, hook.command, hook.script, undefined, hook.timeout, hook.failClosed);
        }
      }
    }`
	return strings.ReplaceAll(body, "%%BT%%", "`")
}
