/**
 * Read/write `.agents/loaf.json` for project configuration and integration toggles.
 *
 * This is the single typed surface for `loaf.json`. Format conventions
 * (2-space indent, trailing newline, key preservation) live here so every
 * writer agrees.
 */

import { existsSync, mkdirSync, readFileSync, writeFileSync } from "fs";
import { join } from "path";

export interface LoafConfig {
  knowledge?: {
    local?: string[];
    staleness_threshold_days?: number;
    imports?: string[];
  };
  integrations?: Record<string, { enabled: boolean }>;
  [key: string]: unknown;
}

/** Absolute path to `.agents/loaf.json` for a project root. */
export function loafConfigPath(projectRoot: string): string {
  return join(projectRoot, ".agents", "loaf.json");
}

export function readLoafConfig(projectRoot: string): LoafConfig {
  const p = loafConfigPath(projectRoot);
  if (!existsSync(p)) return {};
  try {
    const raw = readFileSync(p, "utf-8");
    return JSON.parse(raw) as LoafConfig;
  } catch {
    return {};
  }
}

/**
 * Write `loaf.json`, ensuring the `.agents/` directory exists. Format:
 * 2-space indent + trailing newline. Single source of truth for the file
 * format — every writer in the codebase delegates here.
 */
function writeLoafConfigRaw(
  projectRoot: string,
  next: Record<string, unknown>,
): void {
  const agentsDir = join(projectRoot, ".agents");
  if (!existsSync(agentsDir)) {
    mkdirSync(agentsDir, { recursive: true });
  }
  writeFileSync(
    loafConfigPath(projectRoot),
    `${JSON.stringify(next, null, 2)}\n`,
    "utf-8",
  );
}

export function mergeLoafConfigIntegrations(
  projectRoot: string,
  updates: Partial<{ linear: { enabled: boolean }; serena: { enabled: boolean } }>,
): void {
  const existing = readLoafConfig(projectRoot);
  const integrations = {
    ...existing.integrations,
  };
  if (updates.linear !== undefined) {
    integrations.linear = updates.linear;
  }
  if (updates.serena !== undefined) {
    integrations.serena = updates.serena;
  }
  const next: LoafConfig = {
    ...existing,
    integrations,
  };
  writeLoafConfigRaw(projectRoot, next as Record<string, unknown>);
}
