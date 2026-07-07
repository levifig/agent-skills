#!/usr/bin/env node
/**
 * Skill Routing Evaluation
 *
 * Tests whether Claude routes natural-language prompts to the correct Loaf
 * skill. The dry-run mode validates the suite against content/skills without
 * requiring an API key; live mode calls the Anthropic Messages API and can write
 * a JSON baseline fixture for review.
 *
 * Usage:
 *   node cli/scripts/eval-skill-routing.mjs --dry-run
 *   ANTHROPIC_API_KEY=sk-... node cli/scripts/eval-skill-routing.mjs
 *   ANTHROPIC_API_KEY=sk-... node cli/scripts/eval-skill-routing.mjs --model claude-sonnet-4-6
 *   ANTHROPIC_API_KEY=sk-... node cli/scripts/eval-skill-routing.mjs --suite conflicts --output .agents/reports/routing-baseline.json
 */

import { mkdirSync, readFileSync, readdirSync, statSync, writeFileSync } from "node:fs";
import { dirname, join } from "node:path";

const API_URL = "https://api.anthropic.com/v1/messages";
const API_KEY = process.env.ANTHROPIC_API_KEY;
const DEFAULT_MODEL = "claude-opus-4-6";
const MAX_DESC_CHARS = 250;
const EXPECTED_SKILL_COUNT = 34;

const PRICING = {
  "claude-opus-4-6": { input: 15, output: 75 },
  "claude-sonnet-4-6": { input: 3, output: 15 },
  "claude-haiku-4-5-20251001": { input: 0.8, output: 4 },
};

const args = parseArgs(process.argv.slice(2));

if (args.help) {
  printHelp();
  process.exit(0);
}

const MODEL = args.model || DEFAULT_MODEL;
const SKILLS_DIR = join(process.cwd(), "content", "skills");

const TEST_CASES = {
  architecture: [
    "Create an ADR for the caching approach",
    "Should this project use PostgreSQL or SQLite?",
    "Evaluate the tradeoffs for this technical decision",
  ],
  bootstrap: [
    "Start a new project with Loaf conventions",
    "Set up Loaf for this repository",
    "Bootstrap this repo with the standard project scaffolding",
  ],
  brainstorm: [
    "Help me think through options for notifications",
    "Explore several approaches before we choose one",
    "What are the tradeoffs between these product ideas?",
  ],
  breakdown: [
    "Break this spec into implementation tasks",
    "Create tasks for SPEC-015",
    "Decompose this plan into atomic work items",
  ],
  "loaf-reference": [
    "Which loaf command shows the recent project journal?",
    "Show me the Loaf CLI command for task status",
    "What flags does loaf journal recent support?",
  ],
  council: [
    "Convene a council on this architecture decision",
    "Gather specialist perspectives before we commit",
    "Have multiple agents debate this approach",
  ],
  "database-design": [
    "Design the schema for user accounts",
    "Should we denormalize this table?",
    "Plan the migration to add an index",
  ],
  debugging: [
    "This test is flaky, help me fix it",
    "Diagnose why this fails intermittently",
    "Track hypotheses for this production bug",
  ],
  "documentation-standards": [
    "Write an ADR for this decision",
    "Update the API documentation conventions",
    "Create a Mermaid diagram for this architecture doc",
  ],
  foundations: [
    "Review this code for maintainability basics",
    "Are we following naming conventions here?",
    "Run the general quality checklist before we ship",
  ],
  "git-workflow": [
    "Write a Conventional Commit message",
    "Create a PR title and description for this branch",
    "What branch name should I use?",
  ],
  "go-development": [
    "Write a Go CLI command for this task",
    "Handle concurrency in this Go service",
    "Write table-driven Go tests",
  ],
  handoff: [
    "Prepare transfer context for the next session",
    "Write a handoff for this branch",
    "Package the current state so another agent can continue",
  ],
  housekeeping: [
    "Clean up old agent artifacts",
    "Review sessions and tidy completed files",
    "Archive completed specs and tasks",
  ],
  idea: [
    "I have an idea: what if we added webhooks?",
    "Capture this spark for later",
    "Turn this quick thought into an idea note",
  ],
  implement: [
    "Implement this feature",
    "Start working on TASK-042",
    "Let's build this out",
  ],
  "infrastructure-management": [
    "Write a Dockerfile for this service",
    "Configure the Kubernetes manifest",
    "Set up CI/CD for this project",
  ],
  "interface-design": [
    "Evaluate this layout for accessibility",
    "What color palette works for this design?",
    "Review the typography in this component",
  ],
  "knowledge-base": [
    "Create a knowledge file for this domain",
    "This knowledge file is stale, update it",
    "What are the knowledge management conventions?",
  ],
  orchestration: [
    "Coordinate work across multiple agents",
    "Plan how these tasks should be delegated",
    "Manage the multi-agent session for this effort",
  ],
  "power-systems-modeling": [
    "Calculate the thermal rating for this conductor",
    "Validate conductor parameters against CIGRE TB 601",
    "What's the sag formula for this span?",
  ],
  "python-development": [
    "Write a FastAPI endpoint for user auth",
    "Add Pydantic validation to this model",
    "Write pytest tests for this service",
  ],
  "refactor-deepen": [
    "Find refactoring opportunities in this module",
    "Where should we simplify this design?",
    "Deepen the implementation without changing behavior",
  ],
  reflect: [
    "What did we learn from shipping this?",
    "Integrate learnings from the last sprint",
    "Update our practices based on this completed work",
  ],
  release: [
    "Cut a release from main",
    "Publish a new version",
    "Batch the landed PRs into a release",
  ],
  research: [
    "Investigate how other tools handle this",
    "Assess the current state of the project",
    "Research the options and bring back findings",
  ],
  "ruby-development": [
    "Create a Rails controller for this resource",
    "Add a Hotwire Turbo frame to this view",
    "Write Minitest tests for this model",
  ],
  "security-compliance": [
    "Check this code for hardcoded secrets",
    "Run a security review on this module",
    "Perform a threat analysis on the auth flow",
  ],
  shape: [
    "Shape this idea into a Change",
    "Write a bounded proposal for the auth feature",
    "Turn this rough concept into an implementable proposal",
  ],
  ship: [
    "Ready to merge this PR",
    "Review and land this branch",
    "Ship this pull request",
  ],
  strategy: [
    "What's our strategy?",
    "Update the strategic direction",
    "Clarify personas and long-term positioning",
  ],
  triage: [
    "Process the intake queue",
    "Turn unresolved notes into actionable tasks",
    "Prioritize the backlog of sparks and tasks",
  ],
  "typescript-development": [
    "Create a React component for this design",
    "Set up a Next.js App Router page",
    "Write Vitest tests for this utility",
  ],
  wrap: [
    "Wrap this session",
    "Shut down and summarize the work",
    "Record the final session state before stopping",
  ],
};

const CONFLICT_PROBES = [
  {
    group: "idea-triage",
    choices: ["idea", "triage"],
    expected: "idea",
    prompt: "I just had a spark for a future webhook feature; capture it before we lose it",
  },
  {
    group: "idea-triage",
    choices: ["idea", "triage"],
    expected: "triage",
    prompt: "Go through the unresolved sparks and decide which ones become tasks",
  },
  {
    group: "research-brainstorm",
    choices: ["research", "brainstorm"],
    expected: "brainstorm",
    prompt: "Let's generate a few possible directions before choosing one",
  },
  {
    group: "research-brainstorm",
    choices: ["research", "brainstorm"],
    expected: "research",
    prompt: "Survey the current project and external options, then report findings",
  },
  {
    group: "strategy-reflect",
    choices: ["strategy", "reflect"],
    expected: "strategy",
    prompt: "Define the product direction and personas before the next wave",
  },
  {
    group: "strategy-reflect",
    choices: ["strategy", "reflect"],
    expected: "reflect",
    prompt: "Fold the lessons from this shipped work back into our practices",
  },
  {
    group: "ship-release",
    choices: ["ship", "release"],
    expected: "ship",
    prompt: "Verify this branch and land the pull request",
  },
  {
    group: "ship-release",
    choices: ["ship", "release"],
    expected: "release",
    prompt: "Publish a version from the PRs already merged to main",
  },
  {
    group: "architecture-shape",
    choices: ["architecture", "shape"],
    expected: "architecture",
    prompt: "Record an ADR for the SQLite versus markdown storage decision",
  },
  {
    group: "architecture-shape",
    choices: ["architecture", "shape"],
    expected: "shape",
    prompt: "Turn this rough storage idea into a bounded Change with risks and scope",
  },
  {
    group: "idea-shape",
    choices: ["idea", "shape"],
    expected: "idea",
    prompt: "Quick — jot down this notification idea before I forget it",
  },
  {
    group: "idea-shape",
    choices: ["idea", "shape"],
    expected: "shape",
    prompt: "This notification concept has enough constraints now — bound it into a Change with scope and rabbit holes",
  },
  {
    group: "brainstorm-shape",
    choices: ["brainstorm", "shape"],
    expected: "brainstorm",
    prompt: "I don't know which direction to take on notifications yet — help me explore a few before we commit to one",
  },
  {
    group: "brainstorm-shape",
    choices: ["brainstorm", "shape"],
    expected: "shape",
    prompt: "We've settled on push notifications — bound the scope, risks, and Implementation Units for it",
  },
  {
    group: "foundations-git-workflow-documentation-standards",
    choices: ["foundations", "git-workflow", "documentation-standards"],
    expected: "foundations",
    prompt: "Check whether this change follows our general code quality conventions",
  },
  {
    group: "foundations-git-workflow-documentation-standards",
    choices: ["foundations", "git-workflow", "documentation-standards"],
    expected: "git-workflow",
    prompt: "Write a Conventional Commit and PR description for this branch",
  },
  {
    group: "foundations-git-workflow-documentation-standards",
    choices: ["foundations", "git-workflow", "documentation-standards"],
    expected: "documentation-standards",
    prompt: "Update the ADR and API documentation style guide for this change",
  },
];

let totalInputTokens = 0;
let totalOutputTokens = 0;
let totalCalls = 0;

function parseArgs(rawArgs) {
  const parsed = {
    dryRun: false,
    help: false,
    model: null,
    output: null,
    repeat: 1,
    skill: null,
    suite: "all",
  };

  for (let index = 0; index < rawArgs.length; index++) {
    const arg = rawArgs[index];
    switch (arg) {
      case "--dry-run":
      case "--validate-suite":
        parsed.dryRun = true;
        break;
      case "--help":
      case "-h":
        parsed.help = true;
        break;
      case "--model":
        parsed.model = requireValue(rawArgs, ++index, arg);
        break;
      case "--output":
        parsed.output = requireValue(rawArgs, ++index, arg);
        break;
      case "--repeat":
        parsed.repeat = Number.parseInt(requireValue(rawArgs, ++index, arg), 10);
        break;
      case "--skill":
        parsed.skill = requireValue(rawArgs, ++index, arg);
        break;
      case "--suite":
        parsed.suite = requireValue(rawArgs, ++index, arg);
        break;
      default:
        throw new Error(`Unknown argument: ${arg}`);
    }
  }

  if (!["all", "core", "conflicts"].includes(parsed.suite)) {
    throw new Error(`--suite must be one of: all, core, conflicts`);
  }
  if (!Number.isInteger(parsed.repeat) || parsed.repeat < 1) {
    throw new Error("--repeat must be a positive integer");
  }

  return parsed;
}

function requireValue(rawArgs, index, flag) {
  const value = rawArgs[index];
  if (!value || value.startsWith("--")) {
    throw new Error(`${flag} requires a value`);
  }
  return value;
}

function printHelp() {
  console.log(`Usage: node cli/scripts/eval-skill-routing.mjs [options]

Options:
  --dry-run, --validate-suite  Validate suites without calling Anthropic
  --suite <all|core|conflicts> Select which suite to run (default: all)
  --skill <name>               Run cases whose expected skill is <name>
  --model <model>              Anthropic model for live runs
  --repeat <n>                 Repeat live cases n times (default: 1)
  --output <path>              Write JSON result
  -h, --help                   Show this help`);
}

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

    const front = frontMatch[1];
    const nameMatch = front.match(/^name:\s*(.+)$/m);
    const descMatch = front.match(/description:\s*>-?\n([\s\S]*?)(?=\n[a-z][a-z0-9_-]*:|$)/);
    if (!nameMatch || !descMatch) continue;

    const name = nameMatch[1].trim();
    const description = descMatch[1]
      .split("\n")
      .map((line) => line.trim())
      .filter(Boolean)
      .join(" ")
      .trim();

    skills.push({
      name,
      description,
      truncated: description.slice(0, MAX_DESC_CHARS),
    });
  }
  return skills.sort((a, b) => a.name.localeCompare(b.name));
}

function buildCases() {
  const coreCases = Object.entries(TEST_CASES).flatMap(([expected, prompts]) =>
    prompts.map((prompt) => ({
      kind: "core",
      group: expected,
      expected,
      prompt,
      choices: null,
    }))
  );
  const conflictCases = CONFLICT_PROBES.map((probe) => ({
    kind: "conflict",
    ...probe,
  }));

  let cases;
  if (args.suite === "core") {
    cases = coreCases;
  } else if (args.suite === "conflicts") {
    cases = conflictCases;
  } else {
    cases = [...coreCases, ...conflictCases];
  }

  if (args.skill) {
    cases = cases.filter((testCase) => testCase.expected === args.skill);
  }

  if (args.repeat > 1 && !args.dryRun) {
    cases = cases.flatMap((testCase) =>
      Array.from({ length: args.repeat }, (_, repeatIndex) => ({
        ...testCase,
        repeat: repeatIndex + 1,
      }))
    );
  }

  return cases;
}

function validateSuite(skills, cases) {
  const loadedNames = new Set(skills.map((skill) => skill.name));
  const expectedNames = new Set(Object.keys(TEST_CASES));
  const conflictNames = new Set(
    CONFLICT_PROBES.flatMap((probe) => [probe.expected, ...probe.choices])
  );
  const casePrompts = cases.map((testCase) => testCase.prompt);
  const duplicatePrompts = casePrompts.filter(
    (prompt, index) => casePrompts.indexOf(prompt) !== index
  );

  const errors = [];
  const warnings = [];
  const missingTestCases = [...loadedNames].filter((name) => !expectedNames.has(name));
  const unknownExpectedSkills = [...expectedNames].filter((name) => !loadedNames.has(name));
  const unknownConflictSkills = [...conflictNames].filter((name) => !loadedNames.has(name));

  if (skills.length !== EXPECTED_SKILL_COUNT) {
    errors.push(`expected ${EXPECTED_SKILL_COUNT} skills, loaded ${skills.length}`);
  }
  for (const name of missingTestCases) {
    errors.push(`missing core test cases for loaded skill: ${name}`);
  }
  for (const name of unknownExpectedSkills) {
    errors.push(`core test case references missing skill: ${name}`);
  }
  for (const name of unknownConflictSkills) {
    errors.push(`conflict probe references missing skill: ${name}`);
  }
  for (const prompt of [...new Set(duplicatePrompts)]) {
    warnings.push(`duplicate prompt: ${prompt}`);
  }
  if (args.skill && !loadedNames.has(args.skill)) {
    errors.push(`--skill references missing skill: ${args.skill}`);
  }
  if (cases.length === 0) {
    errors.push("selected suite has no cases");
  }

  return {
    passed: errors.length === 0,
    errors,
    warnings,
    missingTestCases,
    unknownExpectedSkills,
    unknownConflictSkills,
    loadedSkills: [...loadedNames],
    expectedSkills: [...expectedNames].sort(),
  };
}

function buildSkillsListing(skills) {
  return skills.map((skill) => `- ${skill.name}: ${skill.truncated}`).join("\n");
}

async function routePrompt(skillsListing, userPrompt) {
  const systemPrompt = `You are Claude Code with these available skills:

${skillsListing}

The user will give you a task. Respond with ONLY the skill name you would invoke. Nothing else. If no skill matches, respond "none".`;

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

async function runLiveCases(skills, cases) {
  const skillsListing = buildSkillsListing(skills);
  const results = [];
  let pass = 0;
  let fail = 0;

  for (let index = 0; index < cases.length; index++) {
    const testCase = cases[index];
    const repeatSuffix = testCase.repeat ? ` #${testCase.repeat}` : "";
    process.stdout.write(
      `  [${index + 1}/${cases.length}] ${testCase.kind}:${testCase.group}${repeatSuffix} "${testCase.prompt}" -> `
    );

    try {
      const actual = await routePrompt(skillsListing, testCase.prompt);
      const passed = actual === testCase.expected;
      if (passed) {
        pass++;
        console.log(`PASS ${actual}`);
      } else {
        fail++;
        console.log(`FAIL ${actual} (expected: ${testCase.expected})`);
      }
      results.push({ ...testCase, actual, passed });
    } catch (err) {
      fail++;
      console.log(`ERROR ${err.message}`);
      results.push({ ...testCase, actual: "error", passed: false, error: err.message });
    }

    if (index < cases.length - 1) {
      await new Promise((resolve) => setTimeout(resolve, 1200));
    }
  }

  return {
    pass,
    fail,
    total: cases.length,
    accuracy: cases.length === 0 ? 0 : pass / cases.length,
    results,
  };
}

function summarizeBy(results, field) {
  const summary = {};
  for (const result of results) {
    const key = result[field];
    if (!summary[key]) {
      summary[key] = { pass: 0, total: 0, accuracy: 0 };
    }
    summary[key].total++;
    if (result.passed) {
      summary[key].pass++;
    }
  }
  for (const stats of Object.values(summary)) {
    stats.accuracy = stats.total === 0 ? 0 : stats.pass / stats.total;
  }
  return Object.fromEntries(Object.entries(summary).sort(([a], [b]) => a.localeCompare(b)));
}

function calculateCost() {
  const pricing = PRICING[MODEL] || PRICING[DEFAULT_MODEL];
  const inputCost = (totalInputTokens / 1_000_000) * pricing.input;
  const outputCost = (totalOutputTokens / 1_000_000) * pricing.output;
  return {
    input_tokens: totalInputTokens,
    output_tokens: totalOutputTokens,
    api_calls: totalCalls,
    input_cost_usd: Number(inputCost.toFixed(6)),
    output_cost_usd: Number(outputCost.toFixed(6)),
    total_cost_usd: Number((inputCost + outputCost).toFixed(6)),
  };
}

function buildReport({ skills, cases, validation, live }) {
  return {
    schema_version: 1,
    generated_at: new Date().toISOString(),
    mode: args.dryRun ? "dry-run" : "live",
    model: MODEL,
    suite: args.suite,
    skill_filter: args.skill,
    repeat: args.repeat,
    max_description_chars: MAX_DESC_CHARS,
    expected_skill_count: EXPECTED_SKILL_COUNT,
    skill_count: skills.length,
    core_case_count: Object.values(TEST_CASES).reduce((count, prompts) => count + prompts.length, 0),
    conflict_probe_count: CONFLICT_PROBES.length,
    selected_case_count: cases.length,
    skills: skills.map((skill) => ({
      name: skill.name,
      description_chars: skill.description.length,
      truncated_chars: skill.truncated.length,
    })),
    validation,
    totals: live
      ? {
          pass: live.pass,
          fail: live.fail,
          total: live.total,
          accuracy: live.accuracy,
        }
      : null,
    by_skill: live ? summarizeBy(live.results, "expected") : null,
    by_group: live ? summarizeBy(live.results, "group") : null,
    cost: live ? calculateCost() : null,
    results: live ? live.results : [],
  };
}

function printValidation(skills, cases, validation) {
  console.log(`Loaded ${skills.length} skills from ${SKILLS_DIR}`);
  console.log(`Expected skill count: ${EXPECTED_SKILL_COUNT}`);
  console.log(`Model: ${MODEL}`);
  console.log(`Description budget: ${MAX_DESC_CHARS} chars`);
  console.log(`Suite: ${args.suite}`);
  console.log(`Selected cases: ${cases.length}`);
  console.log("");

  if (validation.errors.length > 0) {
    console.log("Suite validation failed:");
    for (const error of validation.errors) {
      console.log(`  - ${error}`);
    }
  } else {
    console.log("Suite validation passed.");
  }

  if (validation.warnings.length > 0) {
    console.log("\nWarnings:");
    for (const warning of validation.warnings) {
      console.log(`  - ${warning}`);
    }
  }
}

function printLiveSummary(live) {
  const pct = live.total === 0 ? "0.0" : (live.accuracy * 100).toFixed(1);
  console.log("\nROUTING EVALUATION RESULTS");
  console.log(`Pass: ${live.pass}/${live.total} (${pct}%)`);
  console.log(`Fail: ${live.fail}/${live.total}`);

  if (live.fail > 0) {
    console.log("\nFailures:");
    for (const result of live.results.filter((item) => !item.passed)) {
      console.log(`  "${result.prompt}"`);
      console.log(`    expected: ${result.expected} -> got: ${result.actual}`);
    }
  }

  console.log("\nPer-skill accuracy:");
  for (const [skill, stats] of Object.entries(summarizeBy(live.results, "expected"))) {
    console.log(`  ${skill}: ${stats.pass}/${stats.total} (${(stats.accuracy * 100).toFixed(0)}%)`);
  }

  const cost = calculateCost();
  console.log("\nToken Usage & Cost");
  console.log(`  API calls:     ${cost.api_calls}`);
  console.log(`  Input tokens:  ${cost.input_tokens.toLocaleString()}`);
  console.log(`  Output tokens: ${cost.output_tokens.toLocaleString()}`);
  console.log(`  Total cost:    $${cost.total_cost_usd.toFixed(4)}`);
}

function writeReport(path, report) {
  mkdirSync(dirname(path), { recursive: true });
  writeFileSync(path, `${JSON.stringify(report, null, 2)}\n`);
  console.log(`\nWrote JSON report: ${path}`);
}

async function main() {
  const skills = loadSkills();
  const cases = buildCases();
  const validation = validateSuite(skills, cases);

  printValidation(skills, cases, validation);

  if (!validation.passed) {
    const report = buildReport({ skills, cases, validation, live: null });
    if (args.output) {
      writeReport(args.output, report);
    }
    process.exit(1);
  }

  if (args.dryRun) {
    const report = buildReport({ skills, cases, validation, live: null });
    if (args.output) {
      writeReport(args.output, report);
    }
    return;
  }

  if (!API_KEY) {
    console.error(
      "\nError: ANTHROPIC_API_KEY is required for live routing evals. Use --dry-run to validate the suite without a key."
    );
    process.exit(1);
  }

  const live = await runLiveCases(skills, cases);
  printLiveSummary(live);

  const report = buildReport({ skills, cases, validation, live });
  if (args.output) {
    writeReport(args.output, report);
  }

  if (live.fail > 0) {
    process.exitCode = 1;
  }
}

main().catch((err) => {
  console.error(err);
  process.exit(1);
});
