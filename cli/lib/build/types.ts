/**
 * Build System Types
 *
 * Shared type definitions for the Loaf build system.
 */

// ─────────────────────────────────────────────────────────────────────────────
// Build Context (passed to each target builder)
// ─────────────────────────────────────────────────────────────────────────────

export interface BuildContext {
  /** hooks.yaml parsed content */
  config: HooksConfig;
  /** Target-specific config from targets.yaml */
  targetConfig: TargetConfig;
  /** Full targets.yaml content */
  targetsConfig: TargetsConfig;
  /** Repository root directory */
  rootDir: string;
  /** Content source directory (skills, agents, hooks, templates) */
  srcDir: string;
  /** Output directory for this target */
  distDir: string;
  /** Target name (e.g., 'claude-code', 'opencode') */
  targetName: string;
}

// ─────────────────────────────────────────────────────────────────────────────
// Target Transformer Interface
// ─────────────────────────────────────────────────────────────────────────────

export interface TargetModule {
  build(ctx: BuildContext): Promise<void>;
}

// ─────────────────────────────────────────────────────────────────────────────
// hooks.yaml Types
// ─────────────────────────────────────────────────────────────────────────────

export interface HookDefinition {
  id: string;
  skill: string;
  /** Shell script path (required for command hooks that use scripts) */
  script?: string;
  /** Direct command to execute (alternative to script, for simple commands) */
  command?: string;
  /** Path to an instruction file relative to hooks output dir (e.g., "instructions/pre-merge.md") */
  instruction?: string;
  /** Hook type: "command" (default) or "prompt" */
  type?: "command" | "prompt";
  /** Prompt text (required for prompt hooks) */
  prompt?: string;
  /** Permission rule filter (e.g., "Bash(gh pr merge:*)") */
  if?: string;
  matcher?: string;
  blocking?: boolean;
  timeout?: number;
  description?: string;
  event?: string;
  /** When true, hook failure/crash blocks the action (fail-closed). Default: false (fail-open) */
  failClosed?: boolean;
}

export interface HooksConfig {
  hooks: {
    "pre-tool"?: HookDefinition[];
    "post-tool"?: HookDefinition[];
    session?: HookDefinition[];
  };
}

// ─────────────────────────────────────────────────────────────────────────────
// targets.yaml Types
// ─────────────────────────────────────────────────────────────────────────────

export interface TargetConfig {
  output?: string;
  defaults?: {
    agents?: {
      frontmatter?: Record<string, unknown>;
    };
  };
}

export interface TargetsConfig {
  "shared-templates"?: Record<string, string[]>;
  targets?: Record<string, TargetConfig>;
}

// ─────────────────────────────────────────────────────────────────────────────
// Frontmatter Types
// ─────────────────────────────────────────────────────────────────────────────

export interface SkillFrontmatter {
  name: string;
  description: string;
  version?: string;
  [key: string]: unknown;
}

export interface AgentFrontmatter {
  name?: string;
  description?: string;
  model?: string;
  skills?: string[];
  tools?: string[];
  [key: string]: unknown;
}
