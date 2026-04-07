/**
 * loaf spec command
 *
 * Subcommands for managing project specs. Currently supports `list`
 * to display specs grouped by status with task counts.
 */

import { Command } from "commander";
import { existsSync } from "fs";
import { join } from "path";

import { findAgentsDir } from "../lib/tasks/resolve.js";
import { loadIndex, buildIndexFromFiles, saveIndex, syncFrontmatterFromIndex, archiveSpecs } from "../lib/tasks/migrate.js";
import type { TaskIndex, SpecStatus } from "../lib/tasks/types.js";

// ANSI color helpers
const bold = (s: string) => `\x1b[1m${s}\x1b[0m`;
const green = (s: string) => `\x1b[32m${s}\x1b[0m`;
const red = (s: string) => `\x1b[31m${s}\x1b[0m`;
const yellow = (s: string) => `\x1b[33m${s}\x1b[0m`;
const cyan = (s: string) => `\x1b[36m${s}\x1b[0m`;
const gray = (s: string) => `\x1b[90m${s}\x1b[0m`;

// ─────────────────────────────────────────────────────────────────────────────
// Helpers
// ─────────────────────────────────────────────────────────────────────────────

/** Display order and color mapping for spec statuses */
const STATUS_ORDER: SpecStatus[] = [
  "implementing",
  "approved",
  "drafting",
  "complete",
];

const STATUS_COLORS: Record<SpecStatus, (s: string) => string> = {
  implementing: yellow,
  approved: cyan,
  drafting: gray,
  complete: green,
};

const STATUS_LABELS: Record<SpecStatus, string> = {
  implementing: "Implementing",
  approved: "Approved",
  drafting: "Drafting",
  complete: "Complete",
};

/**
 * Load or build the task index, exiting on error.
 * If TASKS.json doesn't exist, builds from .md files and saves.
 */
function resolveIndex(agentsDir: string): TaskIndex {
  const indexPath = join(agentsDir, "TASKS.json");

  if (existsSync(indexPath)) {
    const index = loadIndex(indexPath);
    if (index) return index;

    console.error(`  ${red("error:")} TASKS.json exists but is invalid`);
    process.exit(1);
  }

  // Build from .md files
  const index = buildIndexFromFiles(agentsDir);
  saveIndex(indexPath, index);
  return index;
}

interface TaskCounts {
  todo: number;
  in_progress: number;
  done: number;
}

/**
 * Compute task counts per spec from the index.
 * Returns a map of specId -> { todo, in_progress, done }.
 */
function computeTaskCounts(index: TaskIndex): Record<string, TaskCounts> {
  const counts: Record<string, TaskCounts> = {};

  for (const [, task] of Object.entries(index.tasks)) {
    if (!task.spec) continue;

    if (!counts[task.spec]) {
      counts[task.spec] = { todo: 0, in_progress: 0, done: 0 };
    }

    const c = counts[task.spec];
    if (task.status === "done") {
      c.done++;
    } else if (task.status === "in_progress") {
      c.in_progress++;
    } else {
      // todo, blocked, review all count as "todo" for spec summary
      c.todo++;
    }
  }

  return counts;
}

/**
 * Format task counts line for display.
 */
function formatTaskCounts(counts: TaskCounts | undefined): string {
  if (!counts || (counts.todo === 0 && counts.in_progress === 0 && counts.done === 0)) {
    return gray("(none)");
  }

  const parts = [
    counts.todo > 0 ? yellow(String(counts.todo)) : gray("0"),
    " todo · ",
    counts.in_progress > 0 ? cyan(String(counts.in_progress)) : gray("0"),
    " in_progress · ",
    counts.done > 0 ? green(String(counts.done)) : gray("0"),
    " done",
  ];

  return parts.join("");
}

// ─────────────────────────────────────────────────────────────────────────────
// Command
// ─────────────────────────────────────────────────────────────────────────────

export function registerSpecCommand(program: Command): void {
  const spec = program
    .command("spec")
    .description("Manage project specs");

  // loaf spec list
  spec
    .command("list")
    .description("Show specs with status and task counts")
    .option("--json", "Output raw JSON")
    .action(async (options: { json?: boolean }) => {
      const agentsDir = findAgentsDir();

      if (!agentsDir) {
        console.error(`  ${red("error:")} No .agents/ directory found`);
        process.exit(1);
      }

      const index = resolveIndex(agentsDir);

      // --json: dump spec entries and exit
      if (options.json) {
        console.log(JSON.stringify(index.specs, null, 2));
        return;
      }

      console.log(`\n${bold("  loaf spec list")}\n`);

      const specEntries = Object.entries(index.specs);

      if (specEntries.length === 0) {
        console.log(`  ${gray("No specs found.")}\n`);
        return;
      }

      const taskCounts = computeTaskCounts(index);

      // Group specs by status
      const grouped: Record<SpecStatus, Array<[string, typeof index.specs[string]]>> = {
        implementing: [],
        approved: [],
        drafting: [],
        complete: [],
      };

      for (const [id, entry] of specEntries) {
        const status = entry.status as SpecStatus;
        if (grouped[status]) {
          grouped[status].push([id, entry]);
        } else {
          // Unknown status — put in drafting as fallback
          grouped.drafting.push([id, entry]);
        }
      }

      // Sort specs within each group by ID
      for (const status of STATUS_ORDER) {
        grouped[status].sort((a, b) => a[0].localeCompare(b[0]));
      }

      // Display
      for (const status of STATUS_ORDER) {
        const entries = grouped[status];
        if (entries.length === 0) continue;

        const colorFn = STATUS_COLORS[status];
        const label = STATUS_LABELS[status];

        console.log(`  ${bold(colorFn(`${label} (${entries.length})`))}`)

        for (const [id, entry] of entries) {
          console.log(`    ${bold(id)}  ${entry.title}`);
          console.log(`              Tasks: ${formatTaskCounts(taskCounts[id])}`);
        }

        console.log();
      }

      console.log(`  Total: ${bold(String(specEntries.length))} specs\n`);
    });

  // loaf spec archive
  spec
    .command("archive")
    .description("Move completed specs to archive and update TASKS.json")
    .argument("<ids...>", "Spec IDs to archive (e.g., SPEC-009 SPEC-013)")
    .action(async (ids: string[]) => {
      const agentsDir = findAgentsDir();
      if (!agentsDir) {
        console.error(`  ${red("error:")} No .agents/ directory found`);
        process.exit(1);
      }

      const index = resolveIndex(agentsDir);
      const indexPath = join(agentsDir, "TASKS.json");

      console.log(`\n${bold("  loaf spec archive")}\n`);

      const results = archiveSpecs(agentsDir, index, ids);

      let archived = 0;
      let skipped = 0;

      for (const r of results) {
        if (r.status === "archived") {
          const entry = index.specs[r.id];
          console.log(`  ${green("✓")} ${bold(r.id)}: ${entry.title}`);
          archived++;
        } else {
          const icon = r.reason === "already archived" ? gray("⊘") : yellow("⊘");
          console.log(`  ${icon} ${bold(r.id)}: ${r.reason}`);
          skipped++;
        }
      }

      if (archived > 0) {
        saveIndex(indexPath, index);
        syncFrontmatterFromIndex(agentsDir, index);
      }

      console.log();
      if (archived > 0) {
        console.log(`  Archived ${bold(String(archived))} spec(s)`);
      }
      if (skipped > 0) {
        console.log(`  Skipped ${bold(String(skipped))} spec(s)`);
      }
      console.log();
    });
}
