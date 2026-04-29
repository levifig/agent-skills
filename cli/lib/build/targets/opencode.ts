/**
 * OpenCode Build Target
 *
 * Generates flat OpenCode structure (plural dirs per OpenCode config):
 * dist/opencode/
 * ├── skills/
 * ├── agents/
 * ├── commands/    (generated from skills with OpenCode sidecars)
 * └── plugins/
 *     └── hooks.ts
 */

import {
  mkdirSync,
  cpSync,
  writeFileSync,
  readFileSync,
  readdirSync,
  existsSync,
} from "fs";
import matter from "gray-matter";
import { join } from "path";
import { parse as parseYaml } from "yaml";
import { resetTargetOutput } from "../lib/target-output.js";
import { getVersion } from "../lib/version.js";
import { copySkills as copySharedSkills } from "../lib/skills.js";
import { copyAgents as copySharedAgents } from "../lib/agents.js";
import {
  generateRuntimePlugin,
  OpenCodePlatform,
} from "../lib/hooks/runtime-plugin.js";
import type { BuildContext, HooksConfig } from "../types.js";

const TARGET_NAME = "opencode";

function substituteCommands(content: string): string {
  return content
    .replace(/\{\{IMPLEMENT_CMD\}\}/g, "/implement")
    .replace(/\{\{RESUME_CMD\}\}/g, "/resume")
    .replace(/\{\{ORCHESTRATE_CMD\}\}/g, "/implement");
}

export async function build({
  config,
  targetsConfig,
  rootDir,
  srcDir,
  distDir,
}: BuildContext): Promise<void> {
  const version = getVersion(rootDir);
  const transformMd = (content: string) => substituteCommands(content);

  resetTargetOutput(distDir);

  copySkills(rootDir, srcDir, distDir, targetsConfig, transformMd, version);
  copyAgents(srcDir, distDir, version);
  generateCommandsFromSkills(rootDir, distDir, version);
  generateRuntimePlugin(
    config as HooksConfig,
    srcDir,
    distDir,
    OpenCodePlatform,
    version,
  );

  // Copy hooks directory to plugins/hooks/ for script-backed session hooks
  const hooksSrc = join(srcDir, "hooks");
  if (existsSync(hooksSrc)) {
    const hooksDest = join(distDir, "plugins", "hooks");
    mkdirSync(hooksDest, { recursive: true });
    cpSync(hooksSrc, hooksDest, { recursive: true });
  }
}

function copySkills(
  rootDir: string,
  srcDir: string,
  distDir: string,
  targetsConfig: BuildContext["targetsConfig"],
  transformMd: (content: string) => string,
  version: string,
): void {
  const destDir = join(distDir, "skills");

  // Ensure destination directory exists
  mkdirSync(destDir, { recursive: true });

  // Use shared copySkills to read from intermediate dist/skills/
  // but load OpenCode sidecars from content/skills/ for merging
  copySharedSkills({
    srcDir: join(rootDir, "dist"), // Reads from dist/skills/
    destDir,
    targetName: "opencode",
    version,
    targetsConfig,
    transformMd,
    mergeFrontmatter: (base, skillDir) => {
      // skillDir is from dist/skills/, extract skill name
      const skillName = skillDir.split("/").pop() || "";
      // Load OpenCode sidecar from content/skills/{skill}/
      const sidecarPath = join(srcDir, "skills", skillName, "SKILL.opencode.yaml");
      if (existsSync(sidecarPath)) {
        const sidecarContent = readFileSync(sidecarPath, "utf-8");
        const sidecar = (parseYaml(sidecarContent) as Record<string, unknown>) || {};
        return { ...base, ...sidecar };
      }
      return base;
    },
  });
}

function copyAgents(
  srcDir: string,
  distDir: string,
  version: string,
): void {
  const destDir = join(distDir, "agents");

  // Ensure destination directory exists before calling shared function
  mkdirSync(destDir, { recursive: true });

  copySharedAgents({
    srcDir,
    destDir,
    targetName: "opencode",
    version,
    sidecarRequired: false,
  });
}

function generateCommandsFromSkills(
  rootDir: string,
  distDir: string,
  version: string,
): void {
  // Read from intermediate dist/skills/
  const skillsSrc = join(rootDir, "dist", "skills");
  const sidecarsSrc = join(rootDir, "content", "skills");
  const commandsDest = join(distDir, "commands");

  if (!existsSync(skillsSrc)) return;

  mkdirSync(commandsDest, { recursive: true });

  const skills = readdirSync(skillsSrc, { withFileTypes: true })
    .filter((d) => d.isDirectory())
    .map((d) => d.name);

  for (const skill of skills) {
    // Sidecar is in content/skills/, skill content is in dist/skills/
    const sidecarPath = join(sidecarsSrc, skill, "SKILL.opencode.yaml");

    if (!existsSync(sidecarPath)) continue;

    const skillMdPath = join(skillsSrc, skill, "SKILL.md");
    if (!existsSync(skillMdPath)) continue;

    const content = readFileSync(skillMdPath, "utf-8");
    const { content: body, data: skillFrontmatter } = matter(content);

    const sidecarContent = readFileSync(sidecarPath, "utf-8");
    const sidecar = (parseYaml(sidecarContent) as Record<string, unknown>) || {};

    const mergedFrontmatter: Record<string, unknown> = {
      description: (skillFrontmatter as Record<string, unknown>).description || "",
      ...sidecar,
      version,
    };

    // Rewrite relative links for command files
    const relinked = body
      .replace(/\]\(templates\//g, `](../skills/${skill}/templates/`)
      .replace(/\]\(references\//g, `](../skills/${skill}/references/`);

    const transformed = substituteCommands(matter.stringify(relinked, mergedFrontmatter));
    writeFileSync(join(commandsDest, `${skill}.md`), transformed);
  }
}
