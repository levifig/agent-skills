/**
 * Domain Glossary
 *
 * Data layer for the Loaf domain glossary KB convention. Glossary lives at
 * `docs/knowledge/glossary.md` with `type: glossary` frontmatter and four
 * structured sections: Canonical Terms, Candidates, Relationships, and
 * Flagged ambiguities.
 *
 * Mutation policy lives in CLI verbs (propose/stabilize/upsert/list/check)
 * — see `cli/commands/kb.ts`. This module is the storage substrate only.
 */

import { existsSync, mkdirSync, readFileSync, writeFileSync } from "fs";
import { dirname, join } from "path";
import matter from "gray-matter";

// ─────────────────────────────────────────────────────────────────────────────
// Types
// ─────────────────────────────────────────────────────────────────────────────

export interface GlossaryTerm {
  name: string;
  definition: string;
  avoid: string[];
}

export interface GlossaryData {
  frontmatter: GlossaryFrontmatter;
  canonical: GlossaryTerm[];
  candidates: GlossaryTerm[];
  relationships: string;
  flaggedAmbiguities: string;
}

export interface GlossaryFrontmatter {
  type: "glossary";
  topics?: string[];
  last_reviewed?: string;
  [key: string]: unknown;
}

// ─────────────────────────────────────────────────────────────────────────────
// Path Resolution
// ─────────────────────────────────────────────────────────────────────────────

export const GLOSSARY_RELATIVE_PATH = join("docs", "knowledge", "glossary.md");

export function glossaryPath(projectRoot: string): string {
  return join(projectRoot, GLOSSARY_RELATIVE_PATH);
}

// ─────────────────────────────────────────────────────────────────────────────
// Parser
// ─────────────────────────────────────────────────────────────────────────────

const SECTION_CANONICAL = "Canonical Terms";
const SECTION_CANDIDATES = "Candidates";
const SECTION_RELATIONSHIPS = "Relationships";
const SECTION_FLAGGED = "Flagged ambiguities";

const KNOWN_SECTIONS = new Set<string>([
  SECTION_CANONICAL,
  SECTION_CANDIDATES,
  SECTION_RELATIONSHIPS,
  SECTION_FLAGGED,
]);

export function parseGlossary(text: string): GlossaryData {
  const parsed = matter(text);
  const frontmatter = parsed.data as Record<string, unknown>;

  if (frontmatter.type !== "glossary") {
    throw new Error(
      `glossary frontmatter must have \`type: glossary\` (got ${JSON.stringify(frontmatter.type)})`,
    );
  }

  const sections = splitSections(parsed.content);

  // Reject prose sitting before the first `## ` header — we cannot round-trip
  // it, and silently dropping it would corrupt the file on rewrite.
  const preamble = sections[""] ?? "";
  if (preamble.trim().length > 0) {
    throw new Error(
      "glossary body must start with a `## ` section header — preamble prose is not supported",
    );
  }

  for (const name of Object.keys(sections)) {
    if (name && !KNOWN_SECTIONS.has(name)) {
      throw new Error(`unknown glossary section: ${JSON.stringify(name)}`);
    }
  }

  // Require all four sections so the round-trip property holds for any input
  // the parser accepts. The serializer always emits all four; mirroring that
  // expectation in the parser closes the gap.
  for (const required of KNOWN_SECTIONS) {
    if (!(required in sections)) {
      throw new Error(`glossary missing required section: ${JSON.stringify(required)}`);
    }
  }

  return {
    frontmatter: frontmatter as GlossaryFrontmatter,
    canonical: parseTerms(sections[SECTION_CANONICAL] ?? ""),
    candidates: parseTerms(sections[SECTION_CANDIDATES] ?? ""),
    relationships: trimBlock(sections[SECTION_RELATIONSHIPS] ?? ""),
    flaggedAmbiguities: trimBlock(sections[SECTION_FLAGGED] ?? ""),
  };
}

function splitSections(body: string): Record<string, string> {
  const lines = body.split("\n");
  const sections: Record<string, string> = {};
  let current = "";
  let buffer: string[] = [];
  let inFence = false;

  const flush = () => {
    sections[current] = buffer.join("\n");
  };

  for (const line of lines) {
    if (isFenceLine(line)) {
      inFence = !inFence;
      buffer.push(line);
      continue;
    }
    const match = /^##\s+(.+?)\s*$/.exec(line);
    if (!inFence && match && !line.startsWith("### ")) {
      flush();
      current = match[1];
      buffer = [];
      continue;
    }
    buffer.push(line);
  }
  flush();
  return sections;
}

/** Recognize ``` and ~~~ fenced-code-block delimiters. */
function isFenceLine(line: string): boolean {
  return /^\s*(```|~~~)/.test(line);
}

function parseTerms(body: string): GlossaryTerm[] {
  const terms: GlossaryTerm[] = [];
  const lines = body.split("\n");

  let current: { name: string; lines: string[] } | null = null;
  let inFence = false;

  const flush = () => {
    if (!current) return;
    terms.push(termFromLines(current.name, current.lines));
    current = null;
  };

  for (const line of lines) {
    if (isFenceLine(line)) {
      inFence = !inFence;
      if (current) current.lines.push(line);
      continue;
    }
    const match = /^###\s+(.+?)\s*$/.exec(line);
    if (!inFence && match) {
      flush();
      current = { name: match[1], lines: [] };
      continue;
    }
    if (current) current.lines.push(line);
  }
  flush();

  return terms;
}

function termFromLines(name: string, lines: string[]): GlossaryTerm {
  let avoid: string[] = [];
  const definitionLines: string[] = [];

  for (const line of lines) {
    const match = /^_Avoid_:\s*(.*?)\s*$/.exec(line);
    if (match) {
      avoid = parseAvoidList(match[1]);
      continue;
    }
    definitionLines.push(line);
  }

  return {
    name,
    definition: trimBlock(definitionLines.join("\n")),
    avoid,
  };
}

function parseAvoidList(raw: string): string[] {
  return raw
    .split(",")
    .map((entry) => entry.trim())
    .filter((entry) => entry.length > 0);
}

function trimBlock(body: string): string {
  return body.replace(/^\n+/, "").replace(/\n+$/, "");
}

// ─────────────────────────────────────────────────────────────────────────────
// Serializer
// ─────────────────────────────────────────────────────────────────────────────

export function serializeGlossary(data: GlossaryData): string {
  const sections: string[] = [];

  sections.push(`## ${SECTION_CANONICAL}\n`);
  sections.push(renderTerms(data.canonical));

  sections.push(`## ${SECTION_CANDIDATES}\n`);
  sections.push(renderTerms(data.candidates));

  sections.push(`## ${SECTION_RELATIONSHIPS}\n`);
  sections.push(renderFreeForm(data.relationships));

  sections.push(`## ${SECTION_FLAGGED}\n`);
  sections.push(renderFreeForm(data.flaggedAmbiguities));

  const body = sections.join("\n");
  return matter.stringify(body, data.frontmatter);
}

function renderTerms(terms: GlossaryTerm[]): string {
  if (terms.length === 0) return "";
  const blocks = terms.map(renderTerm);
  return `${blocks.join("\n\n")}\n`;
}

function renderTerm(term: GlossaryTerm): string {
  const lines: string[] = [`### ${term.name}`, ""];
  if (term.definition.length > 0) {
    lines.push(term.definition);
  }
  if (term.avoid.length > 0) {
    if (term.definition.length > 0) lines.push("");
    lines.push(`_Avoid_: ${term.avoid.join(", ")}`);
  }
  return lines.join("\n");
}

function renderFreeForm(body: string): string {
  if (body.trim().length === 0) return "";
  return `${body}\n`;
}

// ─────────────────────────────────────────────────────────────────────────────
// File Operations
// ─────────────────────────────────────────────────────────────────────────────

export function ensureGlossaryExists(projectRoot: string): boolean {
  const path = glossaryPath(projectRoot);
  if (existsSync(path)) return false;

  const dir = dirname(path);
  if (!existsSync(dir)) mkdirSync(dir, { recursive: true });

  const empty: GlossaryData = {
    frontmatter: {
      type: "glossary",
      topics: ["glossary"],
      last_reviewed: today(),
    },
    canonical: [],
    candidates: [],
    relationships: "",
    flaggedAmbiguities: "",
  };

  writeFileSync(path, serializeGlossary(empty), "utf-8");
  return true;
}

export function readGlossary(projectRoot: string): GlossaryData {
  const path = glossaryPath(projectRoot);
  if (!existsSync(path)) {
    throw new Error(
      `glossary not found at ${GLOSSARY_RELATIVE_PATH} — call ensureGlossaryExists first`,
    );
  }
  const raw = readFileSync(path, "utf-8");
  return parseGlossary(raw);
}

export function writeGlossary(projectRoot: string, data: GlossaryData): void {
  const path = glossaryPath(projectRoot);
  const dir = dirname(path);
  if (!existsSync(dir)) mkdirSync(dir, { recursive: true });
  writeFileSync(path, serializeGlossary(data), "utf-8");
}

// ─────────────────────────────────────────────────────────────────────────────
// Term Lookup Helpers
// ─────────────────────────────────────────────────────────────────────────────

export function findTerm(
  data: GlossaryData,
  name: string,
): { term: GlossaryTerm; section: "canonical" | "candidates" } | null {
  const needle = name.toLowerCase();
  for (const term of data.canonical) {
    if (term.name.toLowerCase() === needle) {
      return { term, section: "canonical" };
    }
  }
  for (const term of data.candidates) {
    if (term.name.toLowerCase() === needle) {
      return { term, section: "candidates" };
    }
  }
  return null;
}

/**
 * Resolve an alias to its canonical term. Scoped to canonical-only by design:
 * candidates are tentative — their `_Avoid_:` lists may shift before the term
 * stabilizes, so steering vocabulary choices by candidate aliases would chase
 * a moving target. SPEC-034's open question on alias scope resolved here.
 */
export function findCanonicalForAlias(
  data: GlossaryData,
  alias: string,
): GlossaryTerm | null {
  const needle = alias.toLowerCase();
  for (const term of data.canonical) {
    if (term.avoid.some((a) => a.toLowerCase() === needle)) return term;
  }
  return null;
}

function today(): string {
  return new Date().toISOString().slice(0, 10);
}
