/**
 * Read/write `.agents/config.json` for integration toggles (TASK-088).
 */

import { existsSync, mkdirSync, readFileSync, writeFileSync } from "fs";
import { join } from "path";

export interface AgentsConfig {
  integrations?: {
    linear?: { enabled: boolean };
    serena?: { enabled: boolean };
  };
  [key: string]: unknown;
}

export function readAgentsConfig(projectRoot: string): AgentsConfig {
  const p = join(projectRoot, ".agents", "config.json");
  if (!existsSync(p)) return {};
  try {
    const raw = readFileSync(p, "utf-8");
    return JSON.parse(raw) as AgentsConfig;
  } catch {
    return {};
  }
}

export function mergeAgentsConfigIntegrations(
  projectRoot: string,
  updates: Partial<{ linear: { enabled: boolean }; serena: { enabled: boolean } }>,
): void {
  const agentsDir = join(projectRoot, ".agents");
  const p = join(agentsDir, "config.json");
  const existing = readAgentsConfig(projectRoot);
  const integrations = {
    ...existing.integrations,
  };
  if (updates.linear !== undefined) {
    integrations.linear = updates.linear;
  }
  if (updates.serena !== undefined) {
    integrations.serena = updates.serena;
  }
  const next: AgentsConfig = {
    ...existing,
    integrations,
  };
  if (!existsSync(agentsDir)) {
    mkdirSync(agentsDir, { recursive: true });
  }
  writeFileSync(p, `${JSON.stringify(next, null, 2)}\n`, "utf-8");
}
