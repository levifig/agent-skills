/**
 * Gemini Build Target
 *
 * Generates Gemini-compatible skills:
 * dist/gemini/
 * └── skills/
 *     ├── python/
 *     │   ├── SKILL.md         # version + Gemini frontmatter
 *     │   ├── references/
 *     │   └── scripts/
 *     └── ...
 *
 * Gemini only supports skills - no commands, agents, or hooks.
 * Note: Gemini doesn't support XDG conventions yet, uses ~/.gemini/ directly.
 */

import {
  mkdirSync,
  cpSync,
  writeFileSync,
  readFileSync,
  existsSync,
  readdirSync,
  rmSync,
} from "fs";
import matter from "gray-matter";
import { join } from "path";
import { parse as parseYaml } from "yaml";
import { loadSkillFrontmatter } from "../lib/sidecar.js";
import { getVersion, injectVersion } from "../lib/version.js";

const TARGET_NAME = "gemini";

/**
 * Build Gemini distribution
 */
export async function build({ rootDir, srcDir, distDir }) {
  const version = getVersion(rootDir);

  const skillsDir = join(distDir, "skills");

  // Clean existing directory
  if (existsSync(skillsDir)) {
    rmSync(skillsDir, { recursive: true });
  }
  mkdirSync(skillsDir, { recursive: true });

  // Copy all skills with version injection
  copySkills(srcDir, skillsDir, version);
}

/**
 * Copy all skills with version injection and Gemini-specific frontmatter
 */
function copySkills(srcDir, destDir, version) {
  const src = join(srcDir, "skills");

  if (!existsSync(src)) {
    return;
  }

  const skills = readdirSync(src, { withFileTypes: true })
    .filter((d) => d.isDirectory())
    .map((d) => d.name);

  for (const skill of skills) {
    const skillSrc = join(src, skill);
    const skillDest = join(destDir, skill);
    mkdirSync(skillDest, { recursive: true });

    // Load frontmatter from SKILL.md
    const baseFrontmatter = loadSkillFrontmatter(skillSrc);

    // Load optional Gemini sidecar
    const sidecarFrontmatter = loadGeminiSkillSidecar(skillSrc);

    // Merge and inject version
    const frontmatter = injectVersion(
      { ...baseFrontmatter, ...sidecarFrontmatter },
      version
    );

    // Read SKILL.md body (strip existing frontmatter if any)
    const skillMdPath = join(skillSrc, "SKILL.md");
    if (existsSync(skillMdPath)) {
      const content = readFileSync(skillMdPath, "utf-8");
      const { content: body } = matter(content);

      // Write SKILL.md with merged frontmatter + version
      const transformed = matter.stringify(body, frontmatter);
      writeFileSync(join(skillDest, "SKILL.md"), transformed);
    }

    // Copy references directory
    const refSrc = join(skillSrc, "references");
    const refDest = join(skillDest, "references");
    if (existsSync(refSrc)) {
      cpSync(refSrc, refDest, { recursive: true });
    }

    // Copy scripts directory
    const scriptsSrc = join(skillSrc, "scripts");
    const scriptsDest = join(skillDest, "scripts");
    if (existsSync(scriptsSrc)) {
      cpSync(scriptsSrc, scriptsDest, { recursive: true });
    }
  }
}

/**
 * Load optional Gemini sidecar for skill
 */
function loadGeminiSkillSidecar(skillDir) {
  const sidecarPath = join(skillDir, `SKILL.${TARGET_NAME}.yaml`);

  if (!existsSync(sidecarPath)) {
    return {};
  }

  const content = readFileSync(sidecarPath, "utf-8");
  return parseYaml(content) || {};
}
