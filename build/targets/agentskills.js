/**
 * Agent Skills Build Target
 *
 * Generates standard skills format used by Codex, Cursor, and Copilot:
 * dist/agentskills/
 * └── skills/
 *     ├── python/
 *     │   ├── SKILL.md (with frontmatter)
 *     │   ├── references/
 *     │   └── scripts/
 *     └── ...
 *
 * Copies skill directories directly - frontmatter is in SKILL.md source.
 * Excludes sidecar files (*.yaml).
 */

import {
  mkdirSync,
  cpSync,
  readdirSync,
  existsSync,
  rmSync,
  statSync,
} from "fs";
import { join, extname } from "path";

/**
 * Build agentskills distribution
 */
export async function build({ srcDir, distDir }) {
  const skillsDir = join(distDir, "skills");
  if (existsSync(skillsDir)) {
    rmSync(skillsDir, { recursive: true });
  }
  mkdirSync(skillsDir, { recursive: true });

  copySkills(srcDir, skillsDir);
}

/**
 * Copy all skills, excluding sidecar files
 */
function copySkills(srcDir, destDir) {
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

    // Copy skill contents, excluding sidecar files
    copyDirExcludingSidecars(skillSrc, skillDest);
  }
}

/**
 * Recursively copy directory, excluding *.yaml sidecar files
 */
function copyDirExcludingSidecars(src, dest) {
  const entries = readdirSync(src, { withFileTypes: true });

  for (const entry of entries) {
    const srcPath = join(src, entry.name);
    const destPath = join(dest, entry.name);

    if (entry.isDirectory()) {
      mkdirSync(destPath, { recursive: true });
      copyDirExcludingSidecars(srcPath, destPath);
    } else if (entry.isFile()) {
      // Skip sidecar files (*.yaml in skill root)
      if (extname(entry.name) === ".yaml") {
        continue;
      }
      cpSync(srcPath, destPath);
    }
  }
}
