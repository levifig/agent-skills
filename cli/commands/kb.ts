/**
 * loaf kb command
 *
 * Subcommands for knowledge base validation, status overview, staleness
 * checking, and review tracking.
 */

import { Command } from "commander";
import { existsSync, mkdirSync, readFileSync, writeFileSync } from "fs";
import { join, basename, relative, isAbsolute } from "path";
import matter from "gray-matter";

import { findGitRoot, loadKbConfig } from "../lib/kb/resolve.js";
import { loadKnowledgeFiles } from "../lib/kb/loader.js";
import { validateLoadedFiles, findSkippedFiles } from "../lib/kb/validate.js";
import { checkAllStaleness, checkStaleness, findCoveringFiles } from "../lib/kb/staleness.js";
import { isQmdAvailable, registerCollection, listCollections } from "../lib/kb/qmd.js";
import type { StalenessResult, ValidationResult } from "../lib/kb/types.js";
import { registerKbGlossarySubcommand } from "./kb-glossary.js";

// ANSI color helpers (matching project conventions)
const bold = (s: string) => `\x1b[1m${s}\x1b[0m`;
const green = (s: string) => `\x1b[32m${s}\x1b[0m`;
const red = (s: string) => `\x1b[31m${s}\x1b[0m`;
const yellow = (s: string) => `\x1b[33m${s}\x1b[0m`;
const cyan = (s: string) => `\x1b[36m${s}\x1b[0m`;
const gray = (s: string) => `\x1b[90m${s}\x1b[0m`;

// ─────────────────────────────────────────────────────────────────────────────
// Command Registration
// ─────────────────────────────────────────────────────────────────────────────

export function registerKbCommand(program: Command): void {
  const kb = program
    .command("kb")
    .description("Knowledge base management");

  // ── loaf kb glossary ───────────────────────────────────────────────────
  registerKbGlossarySubcommand(kb);

  // ── loaf kb validate ───────────────────────────────────────────────────

  kb
    .command("validate")
    .description("Validate knowledge file frontmatter")
    .option("--json", "Output results as JSON")
    .action(async (options: { json?: boolean }) => {
      let gitRoot: string;
      try {
        gitRoot = findGitRoot();
      } catch {
        if (!options.json) {
          console.error(`  ${red("error:")} Not inside a git repository`);
        }
        process.exit(1);
      }

      const config = loadKbConfig(gitRoot);

      // Validate files the loader accepted (warnings)
      const loadedFiles = loadKnowledgeFiles(gitRoot, config);
      const loadedResults = validateLoadedFiles(gitRoot, loadedFiles);

      // Find files the loader skipped (errors)
      const skippedResults = findSkippedFiles(gitRoot, config);

      const allResults = [...loadedResults, ...skippedResults];

      // --json: output array of ValidationResult objects
      if (options.json) {
        const jsonOutput = allResults.map((r) => ({
          file: r.file.relativePath,
          errors: r.errors,
          warnings: r.warnings,
        }));
        process.stdout.write(JSON.stringify(jsonOutput, null, 2) + "\n");
        const hasErrors = allResults.some((r) => r.errors.length > 0);
        if (hasErrors) process.exit(1);
        return;
      }

      // Human-readable output
      console.log(`\n  ${bold("loaf kb validate")}\n`);

      let totalErrors = 0;
      let totalWarnings = 0;

      for (const result of allResults) {
        const hasIssues = result.errors.length > 0 || result.warnings.length > 0;

        if (!hasIssues) {
          console.log(`  ${green("\u2713")} ${result.file.relativePath}`);
        } else {
          console.log(`  ${result.file.relativePath}`);

          for (const error of result.errors) {
            console.log(`    ${red("error:")} ${error.field} \u2014 ${error.message}`);
            totalErrors++;
          }

          for (const warning of result.warnings) {
            console.log(`    ${yellow("warn:")} ${warning.field} \u2014 ${warning.message}`);
            totalWarnings++;
          }
        }
      }

      // Summary
      console.log();
      console.log(`  ${bold(String(allResults.length))} files, ${red(String(totalErrors))} errors, ${yellow(String(totalWarnings))} warnings`);
      console.log();

      if (totalErrors > 0) {
        process.exit(1);
      }
    });

  // ── loaf kb status ─────────────────────────────────────────────────────

  kb
    .command("status")
    .description("Show knowledge base overview")
    .option("--json", "Output status as JSON")
    .action(async (options: { json?: boolean }) => {
      let gitRoot: string;
      try {
        gitRoot = findGitRoot();
      } catch {
        if (!options.json) {
          console.error(`  ${red("error:")} Not inside a git repository`);
        }
        process.exit(1);
      }

      const config = loadKbConfig(gitRoot);
      const files = loadKnowledgeFiles(gitRoot, config);

      // Compute stats
      const totalFiles = files.length;
      const filesWithCovers = files.filter((f) => f.frontmatter.covers && f.frontmatter.covers.length > 0).length;
      const filesWithoutCovers = totalFiles - filesWithCovers;

      // Staleness check
      const stalenessResults = checkAllStaleness(gitRoot, files, config);
      const staleCount = stalenessResults.filter((r) => r.isStale).length;

      // Average review age
      const now = new Date();
      let totalAgeDays = 0;
      let reviewedCount = 0;

      for (const file of files) {
        const reviewed = new Date(file.frontmatter.last_reviewed);
        if (!isNaN(reviewed.getTime())) {
          const ageDays = Math.floor((now.getTime() - reviewed.getTime()) / (1000 * 60 * 60 * 24));
          totalAgeDays += ageDays;
          reviewedCount++;
        }
      }

      const avgReviewAgeDays = reviewedCount > 0 ? Math.round(totalAgeDays / reviewedCount) : 0;

      // Per-directory breakdown
      const dirCounts: Record<string, number> = {};
      for (const file of files) {
        // Extract directory portion of relativePath
        const parts = file.relativePath.split("/");
        const dir = parts.length > 1 ? parts.slice(0, -1).join("/") : ".";
        dirCounts[dir] = (dirCounts[dir] || 0) + 1;
      }

      // --json: output structured summary
      if (options.json) {
        const summary = {
          total_files: totalFiles,
          files_with_covers: filesWithCovers,
          files_without_covers: filesWithoutCovers,
          stale: staleCount,
          avg_review_age_days: avgReviewAgeDays,
          directories: dirCounts,
        };
        process.stdout.write(JSON.stringify(summary, null, 2) + "\n");
        return;
      }

      // Human-readable output
      console.log(`\n  ${bold("loaf kb status")}\n`);

      console.log(`  Files:    ${bold(String(totalFiles))}`);
      console.log(`  Covers:   ${green(String(filesWithCovers))} with ${gray(String(filesWithoutCovers))} without`);
      console.log(`  Stale:    ${staleCount > 0 ? red(String(staleCount)) : green("0")}`);
      console.log(`  Avg age:  ${bold(String(avgReviewAgeDays))} days since last review`);
      console.log();

      // Directory breakdown
      console.log(`  ${bold("Directories")}`);
      const sortedDirs = Object.entries(dirCounts).sort(([a], [b]) => a.localeCompare(b));
      for (const [dir, count] of sortedDirs) {
        console.log(`    ${cyan(dir)}: ${count} files`);
      }

      console.log();
    });

  // ── loaf kb check ──────────────────────────────────────────────────────

  kb
    .command("check")
    .description("Check knowledge file staleness against git history")
    .option("--file <path>", "Reverse lookup: find knowledge files covering this path")
    .option("--json", "Output results as JSON")
    .action(async (options: { file?: string; json?: boolean }) => {
      let gitRoot: string;
      try {
        gitRoot = findGitRoot();
      } catch {
        if (!options.json) {
          console.error(`  ${red("error:")} Not inside a git repository`);
        }
        process.exit(1);
      }

      const config = loadKbConfig(gitRoot);
      const files = loadKnowledgeFiles(gitRoot, config);

      // ── Reverse lookup mode: --file <path> ──────────────────────────────
      if (options.file) {
        // Normalize to a relative path from git root
        const filePath = isAbsolute(options.file)
          ? relative(gitRoot, options.file)
          : options.file;

        const coveringFiles = findCoveringFiles(files, filePath);

        // Get staleness for each covering file
        const results: StalenessResult[] = coveringFiles.map((f) =>
          checkStaleness(gitRoot, f, config),
        );

        if (options.json) {
          const jsonOutput = results.map((r) => ({
            file: r.file.relativePath,
            isStale: r.isStale,
            commitCount: r.commitCount,
            lastCommitAuthor: r.lastCommitAuthor,
            lastCommitDate: r.lastCommitDate,
            lastReviewed: r.file.frontmatter.last_reviewed,
          }));
          process.stdout.write(JSON.stringify(jsonOutput, null, 2) + "\n");
          return;
        }

        console.log(`\n  ${bold("loaf kb check")} --file ${filePath}\n`);

        if (results.length === 0) {
          console.log(`  ${gray("No knowledge files cover this path")}`);
          console.log();
          return;
        }

        for (const result of results) {
          const statusLabel = result.isStale
            ? red("stale")
            : green("fresh");
          console.log(`  ${statusLabel}  ${result.file.relativePath}`);
          console.log(`    last_reviewed: ${result.file.frontmatter.last_reviewed}`);
          if (result.isStale) {
            console.log(`    ${result.commitCount} commit${result.commitCount === 1 ? "" : "s"} since review`);
            if (result.lastCommitAuthor) {
              console.log(`    last by: ${result.lastCommitAuthor} (${result.lastCommitDate})`);
            }
          }
        }

        console.log();
        return;
      }

      // ── Default mode: check all files ───────────────────────────────────
      const results = checkAllStaleness(gitRoot, files, config);

      if (options.json) {
        const jsonOutput = results.map((r) => ({
          file: r.file.relativePath,
          isStale: r.isStale,
          hasCoverage: r.hasCoverage,
          commitCount: r.commitCount,
          lastCommitAuthor: r.lastCommitAuthor,
          lastCommitDate: r.lastCommitDate,
          lastReviewed: r.file.frontmatter.last_reviewed,
        }));
        process.stdout.write(JSON.stringify(jsonOutput, null, 2) + "\n");
        return;
      }

      console.log(`\n  ${bold("loaf kb check")}\n`);

      // Group results
      const stale = results.filter((r) => r.isStale);
      const fresh = results.filter((r) => !r.isStale && r.hasCoverage);
      const noCoverage = results.filter((r) => !r.hasCoverage);

      // Stale files
      if (stale.length > 0) {
        console.log(`  ${red(bold("Stale"))}`);
        for (const result of stale) {
          console.log(`    ${red("\u2717")} ${result.file.relativePath}`);
          console.log(`      ${result.commitCount} commit${result.commitCount === 1 ? "" : "s"} since ${result.file.frontmatter.last_reviewed}`);
          if (result.lastCommitAuthor) {
            console.log(`      last by: ${result.lastCommitAuthor} (${result.lastCommitDate})`);
          }
        }
        console.log();
      }

      // Fresh files
      if (fresh.length > 0) {
        console.log(`  ${green(bold("Fresh"))}`);
        for (const result of fresh) {
          console.log(`    ${green("\u2713")} ${result.file.relativePath}  ${gray("reviewed " + result.file.frontmatter.last_reviewed)}`);
        }
        console.log();
      }

      // No coverage files
      if (noCoverage.length > 0) {
        console.log(`  ${gray(bold("No coverage"))}`);
        for (const result of noCoverage) {
          console.log(`    ${gray("-")} ${gray(result.file.relativePath)}`);
        }
        console.log();
      }

      // Summary
      console.log(`  ${red(String(stale.length))} stale, ${green(String(fresh.length))} fresh, ${gray(String(noCoverage.length))} without coverage`);
      console.log();
    });

  // ── loaf kb review ─────────────────────────────────────────────────────

  kb
    .command("review")
    .description("Mark a knowledge file as reviewed today")
    .argument("<file>", "File path (relative to git root or absolute)")
    .option("--json", "Output updated frontmatter as JSON")
    .action(async (filePath: string, options: { json?: boolean }) => {
      let gitRoot: string;
      try {
        gitRoot = findGitRoot();
      } catch {
        if (!options.json) {
          console.error(`  ${red("error:")} Not inside a git repository`);
        }
        process.exit(1);
      }

      // Resolve the file path
      const absPath = isAbsolute(filePath) ? filePath : join(gitRoot, filePath);
      const relPath = relative(gitRoot, absPath);

      let raw: string;
      try {
        raw = readFileSync(absPath, "utf-8");
      } catch {
        if (!options.json) {
          console.error(`  ${red("error:")} File not found: ${relPath}`);
        }
        process.exit(1);
        return; // unreachable, but helps TS
      }

      const { data, content } = matter(raw);

      // Guard: only operate on files with knowledge frontmatter
      if (!data || !Array.isArray(data.topics)) {
        if (options.json) {
          process.stdout.write(
            JSON.stringify({ error: `Not a knowledge file (missing topics field): ${relPath}` }, null, 2) + "\n",
          );
        } else {
          console.error(`  ${red("error:")} Not a knowledge file (missing topics field): ${relPath}`);
        }
        process.exit(1);
        return;
      }

      // Update last_reviewed to today
      const today = new Date().toISOString().slice(0, 10);
      data.last_reviewed = today;

      // Serialize back using gray-matter
      const updated = matter.stringify(content, data);
      writeFileSync(absPath, updated, "utf-8");

      // --json: output the updated frontmatter
      if (options.json) {
        process.stdout.write(JSON.stringify(data, null, 2) + "\n");
        return;
      }

      console.log(`\n  ${green("\u2713")} Updated last_reviewed for ${bold(relPath)}`);
      console.log(`    last_reviewed: ${today}`);
      console.log();
    });

  // ── loaf kb init ──────────────────────────────────────────────────────

  kb
    .command("init")
    .description("Initialize knowledge base directories and QMD collections")
    .option("--json", "Output results as JSON")
    .action(async (options: { json?: boolean }) => {
      let gitRoot: string;
      try {
        gitRoot = findGitRoot();
      } catch {
        if (!options.json) {
          console.error(`  ${red("error:")} Not inside a git repository`);
        }
        process.exit(1);
      }

      const repoName = basename(gitRoot);
      const actions: Array<{ action: string; target: string; status: "created" | "exists" }> = [];

      // ── Create directories ──────────────────────────────────────────────

      const dirs = ["docs/knowledge", "docs/decisions"];

      for (const dir of dirs) {
        const fullPath = join(gitRoot, dir);
        if (existsSync(fullPath)) {
          actions.push({ action: "directory", target: dir, status: "exists" });
        } else {
          mkdirSync(fullPath, { recursive: true });
          actions.push({ action: "directory", target: dir, status: "created" });
        }
      }

      // ── Create/update .agents/loaf.json ─────────────────────────────────

      const configPath = join(gitRoot, ".agents", "loaf.json");
      let configStatus: "created" | "exists" | "updated" = "exists";

      if (!existsSync(configPath)) {
        // Ensure .agents/ exists
        const agentsDir = join(gitRoot, ".agents");
        if (!existsSync(agentsDir)) {
          mkdirSync(agentsDir, { recursive: true });
        }

        const config = {
          knowledge: {
            local: ["docs/knowledge", "docs/decisions"],
            staleness_threshold_days: 30,
            imports: [],
          },
        };
        writeFileSync(configPath, JSON.stringify(config, null, 2) + "\n", "utf-8");
        configStatus = "created";
      } else {
        // Read existing, add knowledge section if missing
        try {
          const raw = readFileSync(configPath, "utf-8");
          const parsed = JSON.parse(raw);

          if (!parsed || typeof parsed !== "object" || Array.isArray(parsed)) {
            // Malformed — leave it alone
            configStatus = "exists";
          } else if (!parsed.knowledge || typeof parsed.knowledge !== "object" || Array.isArray(parsed.knowledge)) {
            parsed.knowledge = {
              local: ["docs/knowledge", "docs/decisions"],
              staleness_threshold_days: 30,
              imports: [],
            };
            writeFileSync(configPath, JSON.stringify(parsed, null, 2) + "\n", "utf-8");
            configStatus = "updated";
          }
        } catch {
          // If JSON is malformed, leave it alone
          configStatus = "exists";
        }
      }

      actions.push({
        action: "config",
        target: ".agents/loaf.json",
        status: configStatus === "updated" ? "created" : configStatus,
      });

      // ── QMD integration ─────────────────────────────────────────────────

      const qmdAvailable = isQmdAvailable();
      const qmdActions: Array<{ collection: string; path: string; status: "registered" | "exists" }> = [];

      if (qmdAvailable) {
        const existing = listCollections();

        const collections = [
          { name: `${repoName}-knowledge`, path: join(gitRoot, "docs/knowledge") },
          { name: `${repoName}-decisions`, path: join(gitRoot, "docs/decisions") },
        ];

        for (const col of collections) {
          if (existing.includes(col.name)) {
            qmdActions.push({ collection: col.name, path: col.path, status: "exists" });
          } else {
            registerCollection(col.name, col.path);
            qmdActions.push({ collection: col.name, path: col.path, status: "registered" });
          }
        }
      }

      // ── Output ────────────────────────────────────────────────────────

      if (options.json) {
        const output: Record<string, unknown> = {
          directories: actions.filter((a) => a.action === "directory"),
          config: { path: ".agents/loaf.json", status: configStatus },
          qmd: {
            available: qmdAvailable,
            collections: qmdActions,
          },
        };
        process.stdout.write(JSON.stringify(output, null, 2) + "\n");
        return;
      }

      // Human-readable output
      console.log(`\n  ${bold("loaf kb init")}\n`);

      for (const a of actions) {
        if (a.status === "created") {
          console.log(`  ${green("+")} Created ${a.target}`);
        } else {
          console.log(`  ${gray("-")} Already exists: ${a.target}`);
        }
      }

      if (configStatus === "updated") {
        console.log(`  ${green("+")} Added knowledge section to .agents/loaf.json`);
      }

      console.log();

      if (qmdAvailable) {
        console.log(`  ${bold("QMD")}`);
        for (const qa of qmdActions) {
          if (qa.status === "registered") {
            console.log(`  ${green("+")} Registered collection: ${cyan(qa.collection)}`);
          } else {
            console.log(`  ${gray("-")} Collection exists: ${cyan(qa.collection)}`);
          }
        }
      } else {
        console.log(`  ${bold("QMD")}`);
        console.log(`  ${yellow("info:")} QMD not found. Install QMD for knowledge retrieval:`);
        console.log(`        ${cyan("https://github.com/tobi/qmd")}`);
      }

      console.log();
      console.log(`  ${green("\u2713")} Knowledge base initialized`);
      console.log();
    });

  // ── loaf kb import ────────────────────────────────────────────────────

  kb
    .command("import")
    .description("Import external project knowledge via QMD collection")
    .argument("<name>", "Name of the external project's knowledge collection")
    .option("--path <path>", "Path to the external project's knowledge directory")
    .option("--json", "Output results as JSON")
    .action(async (name: string, options: { json?: boolean; path?: string }) => {
      // ── Require QMD ────────────────────────────────────────────────────
      if (!isQmdAvailable()) {
        if (options.json) {
          process.stdout.write(
            JSON.stringify({ error: "QMD is required for importing external knowledge" }, null, 2) + "\n",
          );
        } else {
          console.error(
            `  ${red("error:")} QMD is required for importing external knowledge. Install QMD: ${cyan("https://github.com/tobi/qmd")}`,
          );
        }
        process.exit(1);
      }

      // ── Resolve git root ───────────────────────────────────────────────
      let gitRoot: string;
      try {
        gitRoot = findGitRoot();
      } catch {
        if (!options.json) {
          console.error(`  ${red("error:")} Not inside a git repository`);
        }
        process.exit(1);
      }

      const config = loadKbConfig(gitRoot);
      const collectionName = `${name}-knowledge`;

      // ── Check for duplicates ───────────────────────────────────────────

      // Already in config?
      if (config.imports.some((i) => i.name === name)) {
        if (options.json) {
          process.stdout.write(
            JSON.stringify({ name, collection: collectionName, status: "already_imported" }, null, 2) + "\n",
          );
        } else {
          console.log(`  Already imported: ${bold(name)}`);
        }
        process.exit(0);
      }

      // ── Validate .agents/loaf.json before any side effects ──────────
      const configPath = join(gitRoot, ".agents", "loaf.json");
      let parsed: Record<string, unknown> = {};

      if (existsSync(configPath)) {
        try {
          const raw = readFileSync(configPath, "utf-8");
          const result = JSON.parse(raw);
          if (!result || typeof result !== "object" || Array.isArray(result)) {
            throw new Error("Expected a JSON object");
          }
          parsed = result;
        } catch {
          if (options.json) {
            process.stdout.write(
              JSON.stringify({ error: "Cannot parse .agents/loaf.json — fix or remove it before importing" }, null, 2) + "\n",
            );
          } else {
            console.error(`  ${red("error:")} Cannot parse .agents/loaf.json — fix or remove it before importing`);
          }
          process.exit(1);
        }
      } else {
        // Ensure .agents/ exists
        const agentsDir = join(gitRoot, ".agents");
        if (!existsSync(agentsDir)) {
          mkdirSync(agentsDir, { recursive: true });
        }
      }

      // ── Register QMD collection (skip if already exists) ─────────────
      const existing = listCollections();
      const collectionExists = existing.includes(collectionName);

      if (!collectionExists) {
        try {
          const collectionPath = options.path ?? name;
          registerCollection(collectionName, collectionPath);
        } catch (error) {
          const message = error instanceof Error ? error.message : String(error);
          if (options.json) {
            process.stdout.write(
              JSON.stringify({ error: `Failed to register collection: ${message}` }, null, 2) + "\n",
            );
          } else {
            console.error(`  ${red("error:")} Failed to register QMD collection: ${message}`);
          }
          process.exit(1);
        }
      }

      // Ensure knowledge.imports exists
      if (!parsed.knowledge || typeof parsed.knowledge !== "object" || Array.isArray(parsed.knowledge)) {
        parsed.knowledge = { local: ["docs/knowledge", "docs/decisions"], staleness_threshold_days: 30, imports: [] };
      }
      const kb = parsed.knowledge as Record<string, unknown>;
      if (!Array.isArray(kb.imports)) {
        kb.imports = [];
      }

      (kb.imports as Array<{ name: string }>).push({ name });
      writeFileSync(configPath, JSON.stringify(parsed, null, 2) + "\n", "utf-8");

      // ── Output ─────────────────────────────────────────────────────────
      if (options.json) {
        process.stdout.write(
          JSON.stringify({ name, collection: collectionName, status: "imported" }, null, 2) + "\n",
        );
        return;
      }

      console.log(`  Imported: ${bold(name)}`);
    });
}
