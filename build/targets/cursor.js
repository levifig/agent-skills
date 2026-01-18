/**
 * Cursor Build Target
 *
 * Generates Cursor rules structure:
 * dist/cursor/
 * └── .cursor/
 *     └── rules/
 *         ├── python.mdc
 *         ├── typescript.mdc
 *         ├── rails.mdc
 *         └── ...
 *
 * Note: Cursor doesn't support hooks, so we include manual check instructions.
 */

import { mkdirSync, readFileSync, writeFileSync, existsSync, readdirSync } from "fs";
import { join } from "path";

/**
 * Build Cursor distribution
 */
export async function build({ config, rootDir, srcDir, distDir }) {
  const rulesDir = join(distDir, ".cursor", "rules");
  mkdirSync(rulesDir, { recursive: true });

  // Generate a rule file for each skill
  const skillsDir = join(srcDir, "skills");
  if (existsSync(skillsDir)) {
    const skills = readdirSync(skillsDir, { withFileTypes: true })
      .filter((d) => d.isDirectory())
      .map((d) => d.name);

    for (const skill of skills) {
      generateSkillRule(skill, srcDir, rulesDir, config);
    }
  }

  // Generate a combined agents rule
  generateAgentsRule(srcDir, rulesDir);
}

/**
 * Generate .mdc rule file for a skill
 */
function generateSkillRule(skillName, srcDir, rulesDir, config) {
  const skillDir = join(srcDir, "skills", skillName);
  const skillMdPath = join(skillDir, "SKILL.md");

  if (!existsSync(skillMdPath)) {
    return;
  }

  const skillContent = readFileSync(skillMdPath, "utf-8");

  // Find associated hooks for manual check instructions
  const pluginGroup = Object.entries(config["plugin-groups"]).find(
    ([, cfg]) => cfg.skills?.includes(skillName)
  );

  let hookInstructions = "";
  if (pluginGroup) {
    const [, cfg] = pluginGroup;
    const preHookIds = cfg.hooks?.["pre-tool"] || [];
    const postHookIds = cfg.hooks?.["post-tool"] || [];

    if (preHookIds.length > 0 || postHookIds.length > 0) {
      hookInstructions = `
## Quality Checks (Manual)

Since Cursor doesn't support automated hooks, run these checks manually:

### Before Committing
${preHookIds
  .map((id) => {
    const hook = config.hooks["pre-tool"].find((h) => h.id === id);
    return hook ? `- ${hook.description}` : null;
  })
  .filter(Boolean)
  .join("\n")}

### After Changes
${postHookIds
  .map((id) => {
    const hook = config.hooks["post-tool"].find((h) => h.id === id);
    return hook ? `- ${hook.description}` : null;
  })
  .filter(Boolean)
  .join("\n")}
`;
    }
  }

  // Read reference files
  const referenceDir = join(skillDir, "reference");
  let referenceContent = "";
  if (existsSync(referenceDir)) {
    const refFiles = readdirSync(referenceDir)
      .filter((f) => f.endsWith(".md"))
      .slice(0, 5); // Limit to avoid too large files

    for (const refFile of refFiles) {
      const refPath = join(referenceDir, refFile);
      const content = readFileSync(refPath, "utf-8");
      referenceContent += `\n\n---\n\n${content}`;
    }
  }

  // Build MDC content
  const mdcContent = `---
description: ${skillName} development patterns and best practices
globs:
${getGlobsForSkill(skillName)}
---

${skillContent}
${hookInstructions}
${referenceContent}
`;

  writeFileSync(join(rulesDir, `${skillName}.mdc`), mdcContent);
}

/**
 * Generate agents rule file
 */
function generateAgentsRule(srcDir, rulesDir) {
  const agentsDir = join(srcDir, "agents");
  if (!existsSync(agentsDir)) {
    return;
  }

  const agentFiles = readdirSync(agentsDir).filter((f) => f.endsWith(".md"));

  let content = `---
description: Agent roles and responsibilities
globs:
  - "**/*"
---

# Available Agents

These are the specialized agents and their responsibilities:

`;

  for (const agentFile of agentFiles) {
    const agentPath = join(agentsDir, agentFile);
    const agentContent = readFileSync(agentPath, "utf-8");

    // Extract frontmatter
    const frontmatterMatch = agentContent.match(/^---\n([\s\S]*?)\n---/);
    if (frontmatterMatch) {
      const frontmatter = frontmatterMatch[1];
      const nameMatch = frontmatter.match(/name:\s*(.+)/);
      const descMatch = frontmatter.match(/description:\s*(.+)/);

      if (nameMatch && descMatch) {
        content += `## ${nameMatch[1]}\n\n${descMatch[1]}\n\n`;
      }
    }
  }

  writeFileSync(join(rulesDir, "agents.mdc"), content);
}

/**
 * Get glob patterns for a skill
 */
function getGlobsForSkill(skillName) {
  const globs = {
    python: `  - "**/*.py"
  - "**/pyproject.toml"
  - "**/requirements*.txt"`,
    typescript: `  - "**/*.ts"
  - "**/*.tsx"
  - "**/tsconfig*.json"
  - "**/package.json"`,
    rails: `  - "**/*.rb"
  - "**/Gemfile"
  - "app/**/*"
  - "config/**/*"`,
    infrastructure: `  - "**/Dockerfile*"
  - "**/*.yaml"
  - "**/*.yml"
  - "**/terraform/**/*"`,
    design: `  - "**/*.css"
  - "**/*.scss"
  - "**/*.tsx"
  - "**/tailwind.config.*"`,
    foundations: `  - "**/*"`,
    orchestration: `  - ".agents/**/*"
  - "**/*.md"`,
  };

  return globs[skillName] || `  - "**/*"`;
}
