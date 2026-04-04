/**
 * loaf housekeeping — scan .agents/, recommend actions (dry-run, filters, pipe-safe).
 */

import { Command } from "commander";
import { existsSync, mkdirSync, readFileSync, renameSync, rmSync, writeFileSync } from "fs";
import { join, dirname } from "path";
import matter from "gray-matter";

import { isTTY, askYesNo } from "../lib/prompts.js";
import { findAgentsDir, getOrBuildIndex } from "../lib/tasks/resolve.js";
import { archiveTasks, archiveSpecs, saveIndex } from "../lib/tasks/migrate.js";
import { scanArtifacts } from "../lib/housekeeping/scanner.js";
import type { ArtifactType, CleanupRecommendation } from "../lib/housekeeping/types.js";

const bold = (s: string) => `\x1b[1m${s}\x1b[0m`;
const green = (s: string) => `\x1b[32m${s}\x1b[0m`;
const red = (s: string) => `\x1b[31m${s}\x1b[0m`;
const yellow = (s: string) => `\x1b[33m${s}\x1b[0m`;
const gray = (s: string) => `\x1b[90m${s}\x1b[0m`;
const cyan = (s: string) => `\x1b[36m${s}\x1b[0m`;

const TYPE_LABELS: Record<ArtifactType, string> = {
  session: "SESSIONS",
  task: "TASKS",
  spec: "SPECS",
  plan: "PLANS",
  draft: "DRAFTS",
  council: "COUNCILS",
  report: "REPORTS",
};

// ─────────────────────────────────────────────────────────────────────────────
// Helpers
// ─────────────────────────────────────────────────────────────────────────────

function formatPreview(fm: Record<string, unknown> | undefined): string {
  if (!fm) return gray("  (no frontmatter)");
  const keys = Object.keys(fm).slice(0, 3);
  return keys
    .map((k) => {
      const v = fm[k];
      const display = typeof v === "string" ? v : JSON.stringify(v);
      return gray(`  ${k}: ${display}`);
    })
    .join("\n");
}

const NESTED_ARCHIVE_TYPES = new Set<ArtifactType>(["session", "council", "report"]);

/** Move session/council/report to archive/ and set archived metadata in frontmatter. */
function archiveGenericArtifact(filePath: string, artifactType: ArtifactType): void {
  const dir = dirname(filePath);
  const archiveDir = join(dir, "archive");
  const filename = filePath.split("/").pop()!;
  const destPath = join(archiveDir, filename);

  mkdirSync(archiveDir, { recursive: true });

  const now = new Date().toISOString();
  const raw = readFileSync(filePath, "utf-8");
  const { data, content } = matter(raw);

  const blockKey = NESTED_ARCHIVE_TYPES.has(artifactType) ? artifactType : null;

  if (blockKey && data[blockKey] && typeof data[blockKey] === "object") {
    const block = data[blockKey] as Record<string, unknown>;
    block.status = "archived";
    block.archived_at = now;
    block.archived_by = "loaf housekeeping";
  } else {
    data.archived_at = now;
    data.archived_by = "loaf housekeeping";
  }

  const updated = matter.stringify(content, data);
  writeFileSync(filePath, updated, "utf-8");

  renameSync(filePath, destPath);
}

// ─────────────────────────────────────────────────────────────────────────────
// Command
// ─────────────────────────────────────────────────────────────────────────────

export function registerHousekeepingCommand(program: Command): void {
  program
    .command("housekeeping")
    .description("Scan .agents/ artifacts and recommend housekeeping actions")
    .option("--dry-run", "Show recommendations without prompting for actions")
    .option("--sessions", "Only review sessions")
    .option("--specs", "Only review specs")
    .option("--plans", "Only review plans")
    .option("--drafts", "Only review drafts")
    .action(async (options: {
      dryRun?: boolean;
      sessions?: boolean;
      specs?: boolean;
      plans?: boolean;
      drafts?: boolean;
    }) => {
      const agentsDir = findAgentsDir();
      if (!agentsDir) {
        console.error(`  ${red("error:")} No .agents/ directory found`);
        process.exit(1);
      }

      console.log(`\n${bold("loaf housekeeping")}\n`);

      const filter: ArtifactType[] | undefined = (() => {
        const types: ArtifactType[] = [];
        if (options.sessions) types.push("session");
        if (options.specs) types.push("spec");
        if (options.plans) types.push("plan");
        if (options.drafts) types.push("draft");
        return types.length > 0 ? types : undefined;
      })();

      const result = scanArtifacts({ agentsDir, filter });

      for (const warning of result.warnings) {
        console.log(`  ${yellow("warn:")} ${warning}`);
      }
      if (result.warnings.length > 0) console.log();

      for (const typeSummary of result.summary) {
        const label = TYPE_LABELS[typeSummary.type];
        console.log(`  ${bold(label)} ${gray(`(${typeSummary.total} total)`)}`);

        const actionableRecs = result.recommendations.filter(
          (r) => r.type === typeSummary.type && r.action !== "skip",
        );

        if (typeSummary.archive > 0) {
          const items = actionableRecs
            .filter((r) => r.action === "archive")
            .map((r) => r.filename);
          console.log(`    ${green("Ready to archive:")}  ${typeSummary.archive}  ${gray(items.join(", "))}`);
        }
        if (typeSummary.delete > 0) {
          const items = actionableRecs
            .filter((r) => r.action === "delete")
            .map((r) => r.filename);
          console.log(`    ${red("Ready to delete:")}   ${typeSummary.delete}  ${gray(items.join(", "))}`);
        }
        if (typeSummary.flag > 0) {
          const items = actionableRecs
            .filter((r) => r.action === "flag")
            .map((r) => r.filename);
          console.log(`    ${yellow("Needs review:")}      ${typeSummary.flag}  ${gray(items.join(", "))}`);
        }
        if (typeSummary.skip > 0) {
          console.log(`    ${gray(`Active/current:     ${typeSummary.skip}`)}`);
        }

        console.log();
      }

      const actionable = result.recommendations.filter((r) => r.action !== "skip");

      if (actionable.length === 0) {
        console.log(`  ${green("✓")} Nothing to clean up.\n`);
        return;
      }

      if (options.dryRun || !isTTY()) {
        const mode = options.dryRun ? "--dry-run" : "non-TTY";
        console.log(`  ${cyan(`(${mode})`)} ${actionable.length} actionable item(s). Run interactively to take action.\n`);
        return;
      }

      console.log(`  ${bold("Actions")} — ${actionable.length} item(s)\n`);

      let index: ReturnType<typeof getOrBuildIndex> | null = null;
      const getIndex = () => {
        if (!index) index = getOrBuildIndex(agentsDir);
        return index;
      };
      const tasksToArchive: string[] = [];
      const specsToArchive: string[] = [];
      let actionsPerformed = 0;

      for (const rec of actionable) {
        console.log(`  ${cyan("→")} ${bold(rec.filename)}`);
        console.log(`    ${rec.reason}`);
        if (rec.hint) console.log(`    ${yellow("hint:")} ${rec.hint}`);

        if (rec.action === "archive") {
          if (rec.type === "task") {
            const match = rec.filename.match(/^(TASK-\d+)/);
            if (match) {
              const confirmed = await askYesNo(`    Archive ${match[1]}? [y/N] `);
              if (confirmed) {
                tasksToArchive.push(match[1]);
                actionsPerformed++;
              }
            }
          } else if (rec.type === "spec") {
            const match = rec.filename.match(/^(SPEC-\d+)/);
            if (match) {
              const confirmed = await askYesNo(`    Archive ${match[1]}? [y/N] `);
              if (confirmed) {
                specsToArchive.push(match[1]);
                actionsPerformed++;
              }
            }
          } else {
            const confirmed = await askYesNo(`    Archive? [y/N] `);
            if (confirmed && existsSync(rec.path)) {
              archiveGenericArtifact(rec.path, rec.type);
              console.log(`    ${green("✓")} Archived`);
              actionsPerformed++;
            }
          }
        } else if (rec.action === "delete") {
          console.log(formatPreview(rec.frontmatter));
          const confirmed = await askYesNo(`    ${red("Delete")}? [y/N] `);
          if (confirmed && existsSync(rec.path)) {
            rmSync(rec.path);
            console.log(`    ${green("✓")} Deleted`);
            actionsPerformed++;
          }
        } else if (rec.action === "flag") {
          console.log(`    ${gray("(flagged for review — no automatic action)")}`);
        }

        console.log();
      }

      if (tasksToArchive.length > 0) {
        const idx = getIndex();
        const results = archiveTasks(agentsDir, idx, tasksToArchive);
        for (const r of results) {
          if (r.status === "archived") {
            console.log(`  ${green("✓")} Archived ${r.id}`);
          } else {
            console.log(`  ${yellow("⚠")} Skipped ${r.id}: ${r.reason}`);
          }
        }
      }

      if (specsToArchive.length > 0) {
        const idx = getIndex();
        const results = archiveSpecs(agentsDir, idx, specsToArchive);
        for (const r of results) {
          if (r.status === "archived") {
            console.log(`  ${green("✓")} Archived ${r.id}`);
          } else {
            console.log(`  ${yellow("⚠")} Skipped ${r.id}: ${r.reason}`);
          }
        }
      }

      if (index) {
        saveIndex(join(agentsDir, "TASKS.json"), index);
      }

      console.log(`  ${green("✓")} Housekeeping complete — ${actionsPerformed} action(s) performed.\n`);
    });
}
