#!/usr/bin/env node
/**
 * Skill Routing Evaluation
 *
 * Tests whether Claude routes natural-language prompts to the correct Loaf skill.
 * Simulates the <available_skills> system prompt listing and asks Opus which skill
 * it would invoke for each test prompt.
 *
 * Usage:
 *   ANTHROPIC_API_KEY=sk-... node scripts/eval-skill-routing.mjs
 *   ANTHROPIC_API_KEY=sk-... node scripts/eval-skill-routing.mjs --model claude-sonnet-4-6  # cheaper runs
 *   ANTHROPIC_API_KEY=sk-... node scripts/eval-skill-routing.mjs --skill foundations        # single skill
 */

import { readFileSync, readdirSync, statSync } from "node:fs";
import { join, basename } from "node:path";

// ── Config ──────────────────────────────────────────────────────────────────

const API_URL = "https://api.anthropic.com/v1/messages";
const API_KEY = process.env.ANTHROPIC_API_KEY;
const DEFAULT_MODEL = "claude-opus-4-6";
const MAX_DESC_CHARS = 250;

// Pricing per million tokens (as of 2026-03)
const PRICING = {
  "claude-opus-4-6": { input: 15, output: 75 },
  "claude-sonnet-4-6": { input: 3, output: 15 },
  "claude-haiku-4-5-20251001": { input: 0.8, output: 4 },
};

// ── CLI args ────────────────────────────────────────────────────────────────

const args = process.argv.slice(2);
const modelFlag = args.indexOf("--model");
const MODEL =
  modelFlag !== -1 ? args[modelFlag + 1] : DEFAULT_MODEL;
const skillFlag = args.indexOf("--skill");
const SKILL_FILTER = skillFlag !== -1 ? args[skillFlag + 1] : null;

if (!API_KEY) {
  console.error("Error: ANTHROPIC_API_KEY environment variable required.");
  process.exit(1);
}

// ── Load skills ─────────────────────────────────────────────────────────────

import { fileURLToPath } from "node:url";
import { dirname } from "node:path";
const __dirname = dirname(fileURLToPath(import.meta.url));
const SKILLS_DIR = join(__dirname, "..", "content", "skills");

function loadSkills() {
  const skills = [];
  for (const dir of readdirSync(SKILLS_DIR)) {
    const skillPath = join(SKILLS_DIR, dir, "SKILL.md");
    try {
      statSync(skillPath);
    } catch {
      continue;
    }
    const content = readFileSync(skillPath, "utf-8");
    const frontMatch = content.match(/^---\n([\s\S]*?)\n---/);
    if (!frontMatch) continue;

    // Simple YAML extraction (avoids dependency)
    const front = frontMatch[1];
    const nameMatch = front.match(/^name:\s*(.+)$/m);
    const descMatch = front.match(/description:\s*>-?\n([\s\S]*?)(?=\n[a-z]|$)/);
    if (!nameMatch || !descMatch) continue;

    const name = nameMatch[1].trim();
    const description = descMatch[1]
      .split("\n")
      .map((l) => l.trim())
      .filter(Boolean)
      .join(" ")
      .trim();

    skills.push({ name, description, truncated: description.slice(0, MAX_DESC_CHARS) });
  }
  return skills;
}

// ── Test cases ──────────────────────────────────────────────────────────────
// 3 prompts per skill: [direct trigger, natural variant, edge/ambiguous]

const TEST_CASES = {
  foundations: [
    "Review this function for code style issues",
    "Are we following naming conventions here?",
    "Run the verification checklist before we ship",
  ],
  "git-workflow": [
    "Write a commit message for these changes",
    "Create a PR for this branch",
    "merge this PR",
  ],
  debugging: [
    "This test is flaky, help me fix it",
    "I'm tracking multiple hypotheses for this failure",
    "Diagnose why this is failing intermittently",
  ],
  "security-compliance": [
    "Check this code for hardcoded secrets",
    "Run a security review on this module",
    "Perform a threat analysis on the auth flow",
  ],
  "documentation-standards": [
    "Write an ADR for this decision",
    "Update the API documentation",
    "Create a Mermaid diagram for this architecture",
  ],
  orchestration: [
    "Manage the session for this work",
    "Delegate this to a specialist agent",
    "Coordinate the work across these tasks",
  ],
  "knowledge-base": [
    "Create a knowledge file for this domain",
    "This knowledge file is stale, update it",
    "What are the knowledge management conventions?",
  ],
  "python-development": [
    "Write a FastAPI endpoint for user auth",
    "Add Pydantic validation to this model",
    "Write pytest tests for this service",
  ],
  "typescript-development": [
    "Create a React component for this design",
    "Set up a Next.js App Router page",
    "Write Vitest tests for this utility",
  ],
  "ruby-development": [
    "Create a Rails controller for this resource",
    "Add a Hotwire Turbo frame to this view",
    "Write Minitest tests for this model",
  ],
  "go-development": [
    "Write a Go CLI tool for this task",
    "Handle concurrency in this Go service",
    "Write Go tests with table-driven patterns",
  ],
  "database-design": [
    "Design the schema for user accounts",
    "Write a migration to add an index",
    "Should we denormalize this table?",
  ],
  "interface-design": [
    "Evaluate this layout for accessibility",
    "What color palette works for this design?",
    "Review the typography in this component",
  ],
  "infrastructure-management": [
    "Write a Dockerfile for this service",
    "Configure the Kubernetes manifest",
    "Set up CI/CD for this project",
  ],
  "power-systems-modeling": [
    "Calculate the thermal rating for this conductor",
    "Validate these conductor parameters against CIGRE TB 601",
    "What's the sag formula for this span?",
  ],
  implement: [
    "Implement this feature",
    "Start working on TASK-042",
    "Let's build this out",
  ],
  shape: [
    "Shape this idea into a spec",
    "Write a spec for the auth feature",
    "This idea has enough constraints, let's bound it",
  ],
  breakdown: [
    "Break this spec into tasks",
    "Create tasks for SPEC-015",
    "Decompose this into atomic work items",
  ],
  brainstorm: [
    "Help me think through the options here",
    "What are the tradeoffs between these approaches?",
    "I'm exploring ideas for the notification system",
  ],
  idea: [
    "I have an idea: what if we added webhooks?",
    "Note this down for later",
    "Promote sparks from the last brainstorm",
  ],
  research: [
    "What's the current state of the project?",
    "Investigate how other tools handle this",
    "Step back and assess where we are",
  ],
  "council-session": [
    "Call a council on this decision",
    "Gather specialists to debate this",
    "What do the experts think about this approach?",
  ],
  strategy: [
    "What's our strategy?",
    "Update the strategic direction",
    "Who are our personas?",
  ],
  architecture: [
    "Should we use PostgreSQL or SQLite?",
    "Create an ADR for the caching approach",
    "Evaluate the tradeoffs for this technical decision",
  ],
  release: [
    "Release this branch",
    "Ready to merge this PR",
    "Ship it",
  ],
  reflect: [
    "What did we learn from shipping this?",
    "Update strategy based on what we built",
    "Integrate learnings from the last sprint",
  ],
  cleanup: [
    "Clean up the agent artifacts",
    "Review sessions and tidy up",
    "Archive completed specs",
  ],
  "resume-session": [
    "Resume that session",
    "Pick up where we left off",
    "Continue the work from yesterday",
  ],
  "reference-session": [
    "Reference that earlier session about auth",
    "What did we decide before about caching?",
    "Import decisions from the last session",
  ],
  bootstrap: [
    "Start a new project",
    "Set up Loaf for this repo",
    "Bootstrap my project",
  ],
};

// ── API call ────────────────────────────────────────────────────────────────

let totalInputTokens = 0;
let totalOutputTokens = 0;
let totalCalls = 0;

async function routePrompt(skillsListing, userPrompt) {
  const systemPrompt = `You are Claude Code with these available skills:

${skillsListing}

The user will give you a task. Respond with ONLY the skill name you would invoke. Nothing else — just the skill name. If no skill matches, respond "none".`;

  const response = await fetch(API_URL, {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
      "x-api-key": API_KEY,
      "anthropic-version": "2023-06-01",
    },
    body: JSON.stringify({
      model: MODEL,
      max_tokens: 50,
      system: systemPrompt,
      messages: [{ role: "user", content: userPrompt }],
    }),
  });

  if (!response.ok) {
    const err = await response.text();
    throw new Error(`API error ${response.status}: ${err}`);
  }

  const data = await response.json();
  totalInputTokens += data.usage?.input_tokens || 0;
  totalOutputTokens += data.usage?.output_tokens || 0;
  totalCalls++;

  return data.content[0]?.text?.trim().toLowerCase() || "unknown";
}

// ── Main ────────────────────────────────────────────────────────────────────

async function main() {
  const skills = loadSkills();
  console.log(`Loaded ${skills.length} skills`);
  console.log(`Model: ${MODEL}`);
  console.log(`Description budget: ${MAX_DESC_CHARS} chars\n`);

  // Build the skills listing (simulating Claude Code's <available_skills>)
  const skillsListing = skills
    .map((s) => `- ${s.name}: ${s.truncated}`)
    .join("\n");

  // Filter test cases
  const testSkills = SKILL_FILTER
    ? Object.entries(TEST_CASES).filter(([k]) => k === SKILL_FILTER)
    : Object.entries(TEST_CASES);

  const results = [];
  let pass = 0;
  let fail = 0;
  let total = 0;

  for (const [expectedSkill, prompts] of testSkills) {
    // Skip skills that don't exist in the loaded set
    if (!skills.find((s) => s.name === expectedSkill)) {
      console.log(`⚠ Skipping ${expectedSkill} (not found in skills dir)`);
      continue;
    }

    for (const prompt of prompts) {
      total++;
      process.stdout.write(`  [${total}] "${prompt}" → `);

      try {
        const actual = await routePrompt(skillsListing, prompt);
        const passed = actual === expectedSkill;

        if (passed) {
          pass++;
          console.log(`✓ ${actual}`);
        } else {
          fail++;
          console.log(`✗ ${actual} (expected: ${expectedSkill})`);
        }

        results.push({ prompt, expected: expectedSkill, actual, passed });
      } catch (err) {
        fail++;
        console.log(`✗ ERROR: ${err.message}`);
        results.push({
          prompt,
          expected: expectedSkill,
          actual: "error",
          passed: false,
        });
      }

      // Rate limit: ~50 RPM for Opus
      await new Promise((r) => setTimeout(r, 1200));
    }
  }

  // ── Report ──────────────────────────────────────────────────────────────

  const pricing = PRICING[MODEL] || PRICING[DEFAULT_MODEL];
  const inputCost = (totalInputTokens / 1_000_000) * pricing.input;
  const outputCost = (totalOutputTokens / 1_000_000) * pricing.output;
  const totalCost = inputCost + outputCost;

  console.log("\n═══════════════════════════════════════════════");
  console.log("  ROUTING EVALUATION RESULTS");
  console.log("═══════════════════════════════════════════════\n");

  console.log(`  Pass: ${pass}/${total} (${((pass / total) * 100).toFixed(1)}%)`);
  console.log(`  Fail: ${fail}/${total}`);

  if (fail > 0) {
    console.log("\n  Failures:");
    for (const r of results.filter((r) => !r.passed)) {
      console.log(`    "${r.prompt}"`);
      console.log(`      expected: ${r.expected} → got: ${r.actual}\n`);
    }
  }

  // Per-skill breakdown
  console.log("  Per-skill accuracy:");
  const bySkill = {};
  for (const r of results) {
    if (!bySkill[r.expected]) bySkill[r.expected] = { pass: 0, total: 0 };
    bySkill[r.expected].total++;
    if (r.passed) bySkill[r.expected].pass++;
  }
  for (const [skill, stats] of Object.entries(bySkill).sort(
    (a, b) => a[1].pass / a[1].total - b[1].pass / b[1].total
  )) {
    const pct = ((stats.pass / stats.total) * 100).toFixed(0);
    const bar = stats.pass === stats.total ? "✓" : "✗";
    console.log(`    ${bar} ${skill}: ${stats.pass}/${stats.total} (${pct}%)`);
  }

  console.log("\n───────────────────────────────────────────────");
  console.log("  Token Usage & Cost");
  console.log("───────────────────────────────────────────────\n");
  console.log(`  Model:         ${MODEL}`);
  console.log(`  API calls:     ${totalCalls}`);
  console.log(`  Input tokens:  ${totalInputTokens.toLocaleString()}`);
  console.log(`  Output tokens: ${totalOutputTokens.toLocaleString()}`);
  console.log(`  Input cost:    $${inputCost.toFixed(4)}`);
  console.log(`  Output cost:   $${outputCost.toFixed(4)}`);
  console.log(`  Total cost:    $${totalCost.toFixed(4)}`);
  console.log("");
}

main().catch((err) => {
  console.error(err);
  process.exit(1);
});
