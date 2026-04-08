/**
 * loaf report command
 *
 * Durable report management for research findings, audits, and investigations.
 */

import { Command } from "commander";
import {
  existsSync,
  mkdirSync,
  readFileSync,
  writeFileSync,
  readdirSync,
  renameSync,
  unlinkSync,
} from "fs";
import { join, basename, resolve } from "path";
import matter from "gray-matter";

import { findAgentsDir } from "../lib/tasks/resolve.js";

// ANSI color helpers
const bold = (s: string) => `\x1b[1m${s}\x1b[0m`;
const green = (s: string) => `\x1b[32m${s}\x1b[0m`;
const red = (s: string) => `\x1b[31m${s}\x1b[0m`;
const yellow = (s: string) => `\x1b[33m${s}\x1b[0m`;
const cyan = (s: string) => `\x1b[36m${s}\x1b[0m`;
const gray = (s: string) => `\x1b[90m${s}\x1b[0m`;

// ─────────────────────────────────────────────────────────────────────────────
// Types
// ─────────────────────────────────────────────────────────────────────────────

interface ReportFrontmatter {
  title: string;
  type: string;
  created: string;
  status: "draft" | "final" | "archived";
  source: string;
  tags: string[];
  finalized_at?: string;
  archived_at?: string;
  archived_by?: string;
}

// ─────────────────────────────────────────────────────────────────────────────
// Helpers
// ─────────────────────────────────────────────────────────────────────────────

/** Convert a slug like "my-cool-report" to "My Cool Report" */
function slugToTitleCase(slug: string): string {
  return slug
    .split("-")
    .map((word) => word.charAt(0).toUpperCase() + word.slice(1))
    .join(" ");
}

/** Get timestamp in filename format: YYYYMMDD-HHMMSS */
function getTimestampForFilename(): string {
  const now = new Date();
  const year = now.getFullYear();
  const month = String(now.getMonth() + 1).padStart(2, "0");
  const day = String(now.getDate()).padStart(2, "0");
  const hour = String(now.getHours()).padStart(2, "0");
  const minute = String(now.getMinutes()).padStart(2, "0");
  const second = String(now.getSeconds()).padStart(2, "0");
  return `${year}${month}${day}-${hour}${minute}${second}`;
}

/** Sanitize a user-provided slug/type: strip path separators and special chars */
function sanitizePathSegment(input: string): string {
  return input.replace(/[/\\:*?"<>|]/g, "-").replace(/^\.+/, "");
}

/** Check that a resolved path is inside the allowed directory */
function isInsideDir(filePath: string, dir: string): boolean {
  const resolved = resolve(filePath);
  const resolvedDir = resolve(dir);
  return resolved.startsWith(resolvedDir + "/") || resolved === resolvedDir;
}

/** Resolve a file argument to a path in .agents/reports/ */
function resolveReportFile(reportsDir: string, fileArg: string): string | null {
  // Try exact path first — but only if inside reportsDir
  if (existsSync(fileArg) && isInsideDir(fileArg, reportsDir)) {
    return fileArg;
  }

  // Try as filename in reports dir — reject path traversal
  const directPath = join(reportsDir, fileArg);
  if (existsSync(directPath) && isInsideDir(directPath, reportsDir)) {
    return directPath;
  }

  // Search for a file containing the argument string
  if (existsSync(reportsDir)) {
    const files = readdirSync(reportsDir).filter(
      (f) => f.endsWith(".md") && f !== "archive"
    );
    const matches = files.filter((f) => f.includes(fileArg));
    if (matches.length === 1) {
      return join(reportsDir, matches[0]);
    }
    if (matches.length > 1) {
      console.error(`  ${red("error:")} Ambiguous match for "${fileArg}" — ${matches.length} reports match:`);
      for (const m of matches) {
        console.error(`    ${gray(m)}`);
      }
      console.error(`  Provide a more specific name or the full filename.`);
      process.exit(1);
    }
  }

  return null;
}

// ─────────────────────────────────────────────────────────────────────────────
// Command
// ─────────────────────────────────────────────────────────────────────────────

export function registerReportCommand(program: Command): void {
  const report = program
    .command("report")
    .description("Manage durable reports (research, audits, investigations)");

  // ── loaf report list ─────────────────────────────────────────────────────

  report
    .command("list")
    .description("List reports")
    .option("--type <type>", "Filter by report type")
    .option("--status <status>", "Filter by status (draft, final, archived)")
    .option("--json", "Output as JSON")
    .action(
      (options: { type?: string; status?: string; json?: boolean }) => {
        const agentsDir = findAgentsDir();
        if (!agentsDir) {
          console.error(`  ${red("error:")} Could not find .agents/ directory`);
          process.exit(1);
        }

        const reportsDir = join(agentsDir, "reports");
        if (!existsSync(reportsDir)) {
          if (options.json) {
            console.log(JSON.stringify([]));
            return;
          }
          console.log(`\n  ${gray("No reports found.")}`);
          console.log(
            `  Run ${cyan("loaf report create <slug>")} to create your first report.\n`
          );
          return;
        }

        // Collect reports from main dir and archive/
        const reports: Array<{
          file: string;
          data: ReportFrontmatter;
        }> = [];

        const files = readdirSync(reportsDir).filter(
          (f) => f.endsWith(".md")
        );
        for (const file of files) {
          try {
            const raw = readFileSync(join(reportsDir, file), "utf-8");
            const parsed = matter(raw);
            const data = parsed.data as unknown as ReportFrontmatter;

            if (options.type && data.type !== options.type) continue;
            if (options.status && data.status !== options.status) continue;

            reports.push({ file, data });
          } catch {
            continue;
          }
        }

        // Also scan archive/ directory
        const archiveDir = join(reportsDir, "archive");
        if (existsSync(archiveDir)) {
          const archivedFiles = readdirSync(archiveDir).filter(
            (f) => f.endsWith(".md")
          );
          for (const file of archivedFiles) {
            try {
              const raw = readFileSync(join(archiveDir, file), "utf-8");
              const parsed = matter(raw);
              const data = parsed.data as unknown as ReportFrontmatter;

              if (options.type && data.type !== options.type) continue;
              if (options.status && data.status !== options.status) continue;

              reports.push({ file: `archive/${file}`, data });
            } catch {
              continue;
            }
          }
        }

        if (options.json) {
          console.log(JSON.stringify(reports, null, 2));
          return;
        }

        if (reports.length === 0) {
          console.log(`\n  ${gray("No reports found.")}`);
          if (options.type || options.status) {
            console.log(`  ${gray("Try removing filters to see all reports.")}`);
          }
          console.log();
          return;
        }

        console.log(`\n  ${bold("loaf report list")}\n`);

        // Group by status
        const drafts = reports.filter((r) => r.data.status === "draft");
        const finals = reports.filter((r) => r.data.status === "final");
        const archived = reports.filter((r) => r.data.status === "archived");

        if (drafts.length > 0) {
          console.log(`  ${yellow("Drafts")}:`);
          for (const r of drafts) {
            const created = r.data.created
              ? gray(` -- ${new Date(r.data.created).toLocaleDateString()}`)
              : "";
            console.log(
              `    ${yellow("*")} ${r.data.title} ${gray(`[${r.data.type}]`)} ${gray(`(${r.data.source})`)}${created}`
            );
            console.log(`      ${gray(r.file)}`);
          }
          console.log();
        }

        if (finals.length > 0) {
          console.log(`  ${green("Final")}:`);
          for (const r of finals) {
            const created = r.data.created
              ? gray(` -- ${new Date(r.data.created).toLocaleDateString()}`)
              : "";
            console.log(
              `    ${green("*")} ${r.data.title} ${gray(`[${r.data.type}]`)} ${gray(`(${r.data.source})`)}${created}`
            );
            console.log(`      ${gray(r.file)}`);
          }
          console.log();
        }

        if (archived.length > 0) {
          console.log(`  ${gray("Archived")}:`);
          for (const r of archived) {
            console.log(
              `    ${gray("*")} ${r.data.title} ${gray(`[${r.data.type}]`)} ${gray(`(${r.data.source})`)}`
            );
            console.log(`      ${gray(r.file)}`);
          }
          console.log();
        }

        console.log(
          `  ${reports.length} report(s) total\n`
        );
      }
    );

  // ── loaf report create ───────────────────────────────────────────────────

  report
    .command("create <slug>")
    .description("Create a new report")
    .option("--type <type>", "Report type", "research")
    .option("--source <source>", "Report source", "ad-hoc")
    .action((slug: string, options: { type: string; source: string }) => {
      const agentsDir = findAgentsDir();
      if (!agentsDir) {
        console.error(`  ${red("error:")} Could not find .agents/ directory`);
        process.exit(1);
      }

      const reportsDir = join(agentsDir, "reports");
      if (!existsSync(reportsDir)) {
        mkdirSync(reportsDir, { recursive: true });
      }

      const timestamp = getTimestampForFilename();
      const safeType = sanitizePathSegment(options.type);
      const safeSlug = sanitizePathSegment(slug);
      const filename = `${timestamp}-${safeType}-${safeSlug}.md`;
      const filePath = join(reportsDir, filename);

      const title = slugToTitleCase(slug);
      const now = new Date().toISOString();

      const frontmatter: ReportFrontmatter = {
        title,
        type: options.type,
        created: now,
        status: "draft",
        source: options.source,
        tags: [],
      };

      const body = `# ${title}

## Question

_What question does this report answer?_

## Summary

_Executive summary of findings._

## Key Findings

- _Finding 1_
- _Finding 2_
- _Finding 3_

## Methodology

_How was this research conducted?_

## Detailed Analysis

_Full analysis goes here._

## Recommendations

- _Recommendation 1_
- _Recommendation 2_

## Sources

- _Source 1_

## Open Questions

- _Question 1_
`;

      const content = matter.stringify(
        body,
        frontmatter as unknown as Record<string, unknown>
      );
      writeFileSync(filePath, content, "utf-8");

      console.log(`\n  ${bold("loaf report create")}\n`);
      console.log(`  ${green("+")} Created: ${cyan(filename)}`);
      console.log(`  Type: ${options.type}`);
      console.log(`  Source: ${options.source}`);
      console.log(`  Path: ${gray(filePath)}`);
      console.log();
    });

  // ── loaf report finalize ─────────────────────────────────────────────────

  report
    .command("finalize <file>")
    .description("Mark a draft report as final")
    .action((fileArg: string) => {
      const agentsDir = findAgentsDir();
      if (!agentsDir) {
        console.error(`  ${red("error:")} Could not find .agents/ directory`);
        process.exit(1);
      }

      const reportsDir = join(agentsDir, "reports");
      const filePath = resolveReportFile(reportsDir, fileArg);

      if (!filePath) {
        console.error(
          `  ${red("error:")} Report not found: ${fileArg}`
        );
        process.exit(1);
      }

      const raw = readFileSync(filePath, "utf-8");
      const parsed = matter(raw);
      const data = parsed.data as unknown as ReportFrontmatter;

      if (data.status !== "draft") {
        console.error(
          `  ${red("error:")} Report is not a draft (status: ${data.status}). Only draft reports can be finalized.`
        );
        process.exit(1);
      }

      data.status = "final";
      data.finalized_at = new Date().toISOString();

      const updated = matter.stringify(
        parsed.content,
        data as unknown as Record<string, unknown>
      );
      writeFileSync(filePath, updated, "utf-8");

      console.log(`\n  ${bold("loaf report finalize")}\n`);
      console.log(
        `  ${green("+")} Finalized: ${cyan(basename(filePath))}`
      );
      console.log();
    });

  // ── loaf report archive ──────────────────────────────────────────────────

  report
    .command("archive <file>")
    .description("Archive a finalized report")
    .action((fileArg: string) => {
      const agentsDir = findAgentsDir();
      if (!agentsDir) {
        console.error(`  ${red("error:")} Could not find .agents/ directory`);
        process.exit(1);
      }

      const reportsDir = join(agentsDir, "reports");
      const filePath = resolveReportFile(reportsDir, fileArg);

      if (!filePath) {
        console.error(
          `  ${red("error:")} Report not found: ${fileArg}`
        );
        process.exit(1);
      }

      const raw = readFileSync(filePath, "utf-8");
      const parsed = matter(raw);
      const data = parsed.data as unknown as ReportFrontmatter;

      if (data.status !== "final") {
        console.error(
          `  ${red("error:")} Report is not finalized (status: ${data.status}). Only final reports can be archived.`
        );
        process.exit(1);
      }

      data.status = "archived";
      data.archived_at = new Date().toISOString();
      data.archived_by = "cli";

      const archiveDir = join(reportsDir, "archive");
      if (!existsSync(archiveDir)) {
        mkdirSync(archiveDir, { recursive: true });
      }

      const updated = matter.stringify(
        parsed.content,
        data as unknown as Record<string, unknown>
      );
      const archivePath = join(archiveDir, basename(filePath));
      writeFileSync(archivePath, updated, "utf-8");
      unlinkSync(filePath);

      console.log(`\n  ${bold("loaf report archive")}\n`);
      console.log(
        `  ${green("+")} Archived: ${cyan(basename(filePath))}`
      );
      console.log(`  Path: ${gray(archivePath)}`);
      console.log();
    });
}
