/**
 * Amp Build Target
 *
 * Amp experimental target:
 * - Skills only (no commands, agents)
 * - Skills read from shared intermediate at dist/skills/
 * - NO sidecar merge (Amp reads standard SKILL.md)
 * - Generates runtime plugin at dist/amp/plugins/loaf.js
 */

import { join } from "path";
import { resetTargetOutput } from "../lib/target-output.js";
import { copySkills } from "../lib/skills.js";
import { getVersion } from "../lib/version.js";
import {
  generateRuntimePlugin,
  AmpPlatform,
} from "../lib/hooks/runtime-plugin.js";
import type { BuildContext, HooksConfig } from "../types.js";

const TARGET_NAME = "amp";

export async function build({
  config,
  rootDir,
  srcDir,
  distDir,
  targetsConfig,
}: BuildContext): Promise<void> {
  const version = getVersion(rootDir);
  resetTargetOutput(distDir);

  // Identity transform - commands already substituted in intermediate
  const transformMd = (content: string) => content;

  // Copy skills from shared intermediate - NO sidecar merge
  copySkills({
    srcDir: join(rootDir, "dist"), // Read from dist/skills/ intermediate
    destDir: join(distDir, "skills"),
    targetName: TARGET_NAME,
    version,
    targetsConfig,
    transformMd,
    // No mergeFrontmatter - Amp reads standard SKILL.md
  });

  // Generate runtime plugin
  generateRuntimePlugin(
    config as HooksConfig,
    join(rootDir, "content"),
    distDir,
    AmpPlatform,
    version,
  );
}
