/**
 * loaf kb glossary subcommands
 *
 * Mutation policy lives here, not in skills. Five verbs:
 *   upsert    — write/update a canonical term
 *   stabilize — promote a candidate to canonical
 *   propose   — write a candidate (low-commitment, exploratory)
 *   list      — enumerate entries
 *   check     — resolve a term to its canonical form, alias, or unknown
 *
 * Write commands (upsert/stabilize/propose) fail fast in Linear-native mode
 * with an explicit error referencing the deferred artifact-taxonomy spec.
 * Read commands (list/check) work in both modes.
 */

import { Command } from "commander";

import { findGitRoot } from "../lib/kb/resolve.js";
import { readLoafConfig } from "../lib/config/agents-config.js";
import { existsSync } from "fs";
import {
  ensureGlossaryExists,
  findCanonicalForAlias,
  findTerm,
  glossaryPath,
  readGlossary,
  writeGlossary,
  type GlossaryData,
  type GlossaryTerm,
} from "../lib/kb/glossary.js";

// ANSI helpers (mirrors kb.ts)
const bold = (s: string) => `\x1b[1m${s}\x1b[0m`;
const green = (s: string) => `\x1b[32m${s}\x1b[0m`;
const red = (s: string) => `\x1b[31m${s}\x1b[0m`;
const yellow = (s: string) => `\x1b[33m${s}\x1b[0m`;
const cyan = (s: string) => `\x1b[36m${s}\x1b[0m`;
const gray = (s: string) => `\x1b[90m${s}\x1b[0m`;

// ─────────────────────────────────────────────────────────────────────────────
// Linear-native gate
// ─────────────────────────────────────────────────────────────────────────────

export const LINEAR_NATIVE_WRITE_ERROR =
  "Linear-native glossary writes pending artifact-taxonomy spec — local mode only for now.";

export function isLinearNative(projectRoot: string): boolean {
  const config = readLoafConfig(projectRoot);
  return config.integrations?.linear?.enabled === true;
}

function guardLinearNativeWrite(projectRoot: string): void {
  if (isLinearNative(projectRoot)) {
    // Print the exact spec'd error string with no prefix or decoration so the
    // user-visible bytes match SPEC-034 verbatim.
    console.error(LINEAR_NATIVE_WRITE_ERROR);
    process.exit(1);
  }
}

// ─────────────────────────────────────────────────────────────────────────────
// Helpers
// ─────────────────────────────────────────────────────────────────────────────

function resolveProjectRoot(): string {
  try {
    return findGitRoot();
  } catch {
    console.error(`  ${red("error:")} Not inside a git repository`);
    process.exit(1);
  }
}

function parseAvoidOption(raw: string | undefined): string[] {
  if (!raw) return [];
  return raw
    .split(",")
    .map((entry) => entry.trim())
    .filter((entry) => entry.length > 0);
}

/** For write paths only — creates the file if missing, then reads it. */
function loadOrCreate(projectRoot: string): GlossaryData {
  ensureGlossaryExists(projectRoot);
  return readGlossary(projectRoot);
}

/**
 * For read paths — returns an empty in-memory glossary if the file does not
 * exist yet. Reads must never side-effect on the filesystem (lazy creation
 * is reserved for writes per SPEC-034 no-go).
 */
function loadForRead(projectRoot: string): GlossaryData {
  if (!existsSync(glossaryPath(projectRoot))) {
    return {
      frontmatter: { type: "glossary" },
      canonical: [],
      candidates: [],
      relationships: "",
      flaggedAmbiguities: "",
    };
  }
  return readGlossary(projectRoot);
}

function truncateOneLine(text: string, max = 80): string {
  const firstLine = text.split("\n")[0] ?? "";
  if (firstLine.length <= max) return firstLine;
  return `${firstLine.slice(0, max - 1)}…`;
}

// ─────────────────────────────────────────────────────────────────────────────
// upsert
// ─────────────────────────────────────────────────────────────────────────────

interface UpsertOptions {
  definition: string;
  avoid?: string;
}

export function upsertHandler(
  projectRoot: string,
  term: string,
  options: UpsertOptions,
): { action: "created" | "updated" } {
  guardLinearNativeWrite(projectRoot);
  const data = loadOrCreate(projectRoot);
  const avoid = parseAvoidOption(options.avoid);

  // If the same term already exists as a candidate, promote it implicitly to
  // canonical so the caller doesn't have to chain `stabilize`.
  data.candidates = data.candidates.filter(
    (t) => t.name.toLowerCase() !== term.toLowerCase(),
  );

  // Reject when one of the requested aliases is itself canonical — the user
  // is asking us to make a canonical entry shadow another. This is exactly
  // the failure mode flagged in the open question on conflicting aliases.
  for (const alias of avoid) {
    const conflict = data.canonical.find(
      (t) =>
        t.name.toLowerCase() === alias.toLowerCase() &&
        t.name.toLowerCase() !== term.toLowerCase(),
    );
    if (conflict) {
      console.error(
        `  ${red("error:")} alias ${JSON.stringify(alias)} is already canonical (term: ${conflict.name}); pick a different surface or run ${cyan("loaf kb glossary stabilize")} first`,
      );
      process.exit(1);
    }
  }

  const existingIndex = data.canonical.findIndex(
    (t) => t.name.toLowerCase() === term.toLowerCase(),
  );

  const next: GlossaryTerm = {
    name: term,
    definition: options.definition,
    avoid,
  };

  let action: "created" | "updated";
  if (existingIndex >= 0) {
    data.canonical[existingIndex] = next;
    action = "updated";
  } else {
    data.canonical.push(next);
    action = "created";
  }

  writeGlossary(projectRoot, data);
  return { action };
}

// ─────────────────────────────────────────────────────────────────────────────
// check
// ─────────────────────────────────────────────────────────────────────────────

export type CheckResult =
  | { kind: "canonical"; term: GlossaryTerm }
  | { kind: "candidate"; term: GlossaryTerm }
  | { kind: "alias"; alias: string; canonical: GlossaryTerm }
  | { kind: "unknown"; query: string };

export function checkHandler(projectRoot: string, term: string): CheckResult {
  const data = loadForRead(projectRoot);

  const direct = findTerm(data, term);
  if (direct) {
    return direct.section === "canonical"
      ? { kind: "canonical", term: direct.term }
      : { kind: "candidate", term: direct.term };
  }

  const alias = findCanonicalForAlias(data, term);
  if (alias) {
    return { kind: "alias", alias: term, canonical: alias };
  }

  return { kind: "unknown", query: term };
}

// ─────────────────────────────────────────────────────────────────────────────
// list
// ─────────────────────────────────────────────────────────────────────────────

export type ListMode = "all" | "canonical" | "candidates";

export function listHandler(
  projectRoot: string,
  mode: ListMode,
): { canonical: GlossaryTerm[]; candidates: GlossaryTerm[] } {
  const data = loadForRead(projectRoot);
  return {
    canonical: mode === "candidates" ? [] : data.canonical,
    candidates: mode === "canonical" ? [] : data.candidates,
  };
}

// ─────────────────────────────────────────────────────────────────────────────
// stabilize
// ─────────────────────────────────────────────────────────────────────────────

interface StabilizeOptions {
  definition?: string;
}

export function stabilizeHandler(
  projectRoot: string,
  term: string,
  options: StabilizeOptions = {},
): { previousDefinition: string; nextDefinition: string } {
  guardLinearNativeWrite(projectRoot);
  // Use a read-only load so failing-to-find doesn't leave an empty glossary
  // file behind — lazy creation is reserved for *successful* writes per
  // SPEC-034 line 102 No-Go.
  const data = loadForRead(projectRoot);

  const candidateIndex = data.candidates.findIndex(
    (t) => t.name.toLowerCase() === term.toLowerCase(),
  );

  if (candidateIndex < 0) {
    console.error(`  ${red("error:")} not in candidates: ${JSON.stringify(term)}`);
    process.exit(1);
  }

  // From this point on we know we'll mutate state — promote loadForRead's
  // in-memory data into a real on-disk file before the first write.
  ensureGlossaryExists(projectRoot);

  const candidate = data.candidates[candidateIndex];
  const nextDefinition = options.definition ?? candidate.definition;

  // Move from candidates to canonical, preserving alias list; allow definition
  // override during promotion.
  data.candidates.splice(candidateIndex, 1);

  const existingCanonical = data.canonical.findIndex(
    (t) => t.name.toLowerCase() === term.toLowerCase(),
  );
  const promoted: GlossaryTerm = {
    name: candidate.name,
    definition: nextDefinition,
    avoid: candidate.avoid,
  };
  if (existingCanonical >= 0) {
    data.canonical[existingCanonical] = promoted;
  } else {
    data.canonical.push(promoted);
  }

  writeGlossary(projectRoot, data);
  return { previousDefinition: candidate.definition, nextDefinition };
}

// ─────────────────────────────────────────────────────────────────────────────
// propose
// ─────────────────────────────────────────────────────────────────────────────

interface ProposeOptions {
  definition: string;
  avoid?: string;
}

export function proposeHandler(
  projectRoot: string,
  term: string,
  options: ProposeOptions,
): { action: "created" | "updated" } {
  guardLinearNativeWrite(projectRoot);
  const data = loadOrCreate(projectRoot);
  const avoid = parseAvoidOption(options.avoid);

  // Refuse to propose a term that's already canonical — ambiguous about intent.
  const canonicalConflict = data.canonical.find(
    (t) => t.name.toLowerCase() === term.toLowerCase(),
  );
  if (canonicalConflict) {
    console.error(
      `  ${red("error:")} ${JSON.stringify(term)} is already canonical; use ${cyan("upsert")} to update it`,
    );
    process.exit(1);
  }

  const existingIndex = data.candidates.findIndex(
    (t) => t.name.toLowerCase() === term.toLowerCase(),
  );

  const next: GlossaryTerm = {
    name: term,
    definition: options.definition,
    avoid,
  };

  let action: "created" | "updated";
  if (existingIndex >= 0) {
    data.candidates[existingIndex] = next;
    action = "updated";
  } else {
    data.candidates.push(next);
    action = "created";
  }

  writeGlossary(projectRoot, data);
  return { action };
}

// ─────────────────────────────────────────────────────────────────────────────
// Commander Registration
// ─────────────────────────────────────────────────────────────────────────────

export function registerKbGlossarySubcommand(kb: Command): void {
  const glossary = kb
    .command("glossary")
    .description("Domain glossary mutation and lookup");

  // ── upsert ─────────────────────────────────────────────────────────────
  glossary
    .command("upsert <term>")
    .description("Write or update a canonical term")
    .requiredOption("--definition <text>", "Term definition")
    .option(
      "--avoid <list>",
      "Comma-separated list of alternative surfaces to avoid",
    )
    .action((term: string, options: UpsertOptions) => {
      const root = resolveProjectRoot();
      const result = upsertHandler(root, term, options);
      const verb = result.action === "created" ? green("created") : cyan("updated");
      console.log(`  ${verb} canonical: ${bold(term)}`);
    });

  // ── check ──────────────────────────────────────────────────────────────
  glossary
    .command("check <term>")
    .description("Resolve a term to canonical, alias, or unknown")
    .action((term: string) => {
      const root = resolveProjectRoot();
      const result = checkHandler(root, term);

      switch (result.kind) {
        case "canonical": {
          console.log(`  ${green("canonical:")} ${bold(result.term.name)}`);
          if (result.term.definition.length > 0) {
            console.log(`    ${result.term.definition}`);
          }
          if (result.term.avoid.length > 0) {
            console.log(`    ${gray("avoid:")} ${result.term.avoid.join(", ")}`);
          }
          process.exit(0);
        }
        case "candidate": {
          console.log(`  ${yellow("candidate:")} ${bold(result.term.name)}`);
          if (result.term.definition.length > 0) {
            console.log(`    ${result.term.definition}`);
          }
          process.exit(0);
        }
        case "alias": {
          // Spec line 136 wording is verbatim "avoided, use <canonical>".
          console.log(`avoided, use ${result.canonical.name}`);
          if (result.canonical.definition.length > 0) {
            console.log(`    ${result.canonical.definition}`);
          }
          process.exit(0);
        }
        case "unknown": {
          console.log(`  ${gray("unknown:")} ${result.query}`);
          process.exit(1);
        }
      }
    });

  // ── list ───────────────────────────────────────────────────────────────
  glossary
    .command("list")
    .description("Enumerate glossary entries")
    .option("--canonical", "Show only canonical entries")
    .option("--candidates", "Show only candidate entries")
    .option("--all", "Show all entries (default)")
    .action(
      (options: { canonical?: boolean; candidates?: boolean; all?: boolean }) => {
        const root = resolveProjectRoot();

        let mode: ListMode = "all";
        if (options.canonical && !options.candidates) mode = "canonical";
        else if (options.candidates && !options.canonical) mode = "candidates";

        const result = listHandler(root, mode);
        const total = result.canonical.length + result.candidates.length;
        if (total === 0) {
          // Spec: empty glossary prints a single message and exits 0.
          console.log("No glossary entries yet");
          process.exit(0);
        }

        // Spec: one `<term>: <truncated definition>` line per entry, no
        // section headers — keeps the surface scriptable with grep/awk.
        for (const term of result.canonical) {
          console.log(`${term.name}: ${truncateOneLine(term.definition)}`);
        }
        for (const term of result.candidates) {
          console.log(`${term.name}: ${truncateOneLine(term.definition)}`);
        }
      },
    );

  // ── stabilize ──────────────────────────────────────────────────────────
  glossary
    .command("stabilize <term>")
    .description("Promote a candidate to canonical")
    .option(
      "--definition <text>",
      "Override definition during promotion (default: keep candidate's)",
    )
    .action((term: string, options: StabilizeOptions) => {
      const root = resolveProjectRoot();
      stabilizeHandler(root, term, options);
      console.log(`  ${green("stabilized:")} ${bold(term)}`);
    });

  // ── propose ────────────────────────────────────────────────────────────
  glossary
    .command("propose <term>")
    .description("Write a candidate term (low-commitment, exploratory)")
    .requiredOption("--definition <text>", "Candidate definition")
    .option(
      "--avoid <list>",
      "Comma-separated list of alternative surfaces to avoid",
    )
    .action((term: string, options: ProposeOptions) => {
      const root = resolveProjectRoot();
      const result = proposeHandler(root, term, options);
      const verb = result.action === "created" ? green("proposed") : cyan("updated");
      console.log(`  ${verb} candidate: ${bold(term)}`);
    });
}
