/**
 * Read/write `.agents/loaf.json` for project configuration and integration toggles.
 */

import { existsSync, mkdirSync, readFileSync, writeFileSync } from "fs";
import { join } from "path";

export interface LoafConfig {
  knowledge?: {
    local?: string[];
    staleness_threshold_days?: number;
    imports?: string[];
  };
  integrations?: {
    linear?: { enabled: boolean };
    serena?: { enabled: boolean };
  };
  [key: string]: unknown;
}

export function readLoafConfig(projectRoot: string): LoafConfig {
  const p = join(projectRoot, ".agents", "loaf.json");
  if (!existsSync(p)) return {};
  try {
    const raw = readFileSync(p, "utf-8");
    return JSON.parse(raw) as LoafConfig;
  } catch {
    return {};
  }
}

export function mergeLoafConfigIntegrations(
  projectRoot: string,
  updates: Partial<{ linear: { enabled: boolean }; serena: { enabled: boolean } }>,
): void {
  const agentsDir = join(projectRoot, ".agents");
  const p = join(agentsDir, "loaf.json");
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
  if (!existsSync(agentsDir)) {
    mkdirSync(agentsDir, { recursive: true });
  }
  writeFileSync(p, `${JSON.stringify(next, null, 2)}\n`, "utf-8");
}
