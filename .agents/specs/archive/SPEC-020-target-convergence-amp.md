---
id: SPEC-020
title: 'Cross-Harness Skills, Hook Consolidation & Target Convergence'
source: brainstorm/amp-target-exploration + brainstorm-session-20260331
created: '2026-03-30T23:13:00.000Z'
status: complete
---

# SPEC-020: Cross-Harness Skills, Hook Consolidation & Target Convergence

## Problem Statement

Loaf ships skills to 5 AI coding tools but builds them 5 separate ways, with ~1,217 lines of duplicated logic. Adding a 6th target (Amp) exposed the duplication and prompted a broader investigation. The late discovery that 3 of those tools already share a skill path (`.agents/skills/`) — plus the realization that skill descriptions aren't optimized for cross-harness routing — reframes the problem:

**Skills are the knowledge layer; hooks are the enforcement layer.** The build system should treat skills as a cross-harness artifact, not 5 independent copies. And hook enforcement should be predictable and reactive (firing on tool use, not hoping agents remember), but the hook *logic* should consolidate into the CLI rather than living in 42 separate shell scripts.

### Key findings from research

1. **Amp, Codex, and Cursor all read from `.agents/skills/`** — skills are already a shared content artifact in practice. The build system hasn't caught up.

2. **"Plugin" means two different things** — distribution packaging (Claude Code, Codex, Cursor marketplace bundles) vs runtime event interception (Amp, OpenCode TypeScript plugins). These are separate concerns.

3. **Most tools natively turn skills into commands** — Claude Code via `user-invocable` (unscoped `/command` works when no collision), Codex via `/skills` menu or `$skill-name` mention, Cursor via `/skill-name`, Amp via model-driven loading. Only OpenCode needs explicit command generation.

4. **Command substitution is nearly universal** — All targets use unscoped commands (`/implement`, `/resume`). Claude Code previously required `/loaf:implement` scoping, but now supports unscoped commands when there's no plugin collision. The `/loaf:` prefix is a disambiguation fallback, not the primary form. This makes a shared skill intermediate much more viable.

5. **Hook scripts duplicate logic** — 42 shell scripts (30-100 lines each) independently parse context, detect projects, run checks, and format output. The detection/parsing/formatting is duplicated across all of them. Four shared libraries (~400 LOC) provide reusable utilities, but each hook still has significant inline logic.

6. **Each harness has unique strengths** — Codex has `nickname_candidates` for agents, Cursor has 19 hook events with `failClosed`, Amp has `registerTool()` and `registerCommand()`, Claude Code has prompt hooks. Loaf should leverage these, not lowest-common-denominator them.

7. **Skill descriptions aren't cross-harness ready** — All 30 skills exceed Claude Code's 250-char truncation budget. Descriptions need to work across different routing models (truncation, model-driven, agent-decides).

8. **Not all hooks are validation checks** — The hook system has 5 distinct categories: validation gates (~30), prompt hooks (1), session lifecycle (5), side-effect hooks (3-4), and stateful tracking (1-2). Only validation gates can cleanly become `loaf check` invocations.

### Measured duplication

| Function | Instances | Lines each | Total duplicate lines |
|---|---|---|---|
| `copySkills()` | 5 | ~40 | ~160 |
| `copyAgents()` | 3 | ~25 | ~50 |
| `substituteCommands()` | 5 | ~5 | ~20 |
| Runtime hook generator | 1 (OpenCode) | ~150 | Would be ~150 for Amp |
| Hook scripts (validation) | ~30 | ~30-100 | ~1,500+ |
| Hook shared libraries | 4 | ~100 | ~400 |

## Solution Direction

### Architecture: Three Layers

**Layer 1: Shared content intermediate** — Skills are sourced once into a pre-transformation intermediate (`dist/skills/`), then each target applies its own lightweight transform (sidecar merge, version injection). Command substitution is universal (unscoped `/command` form). The shared intermediate eliminates duplicated discovery, copying, and shared-template distribution logic.

**Layer 2: Delivery adapters** — Runtime plugins for Amp/OpenCode, shell hook trigger configs for Claude/Cursor/Codex, agent adapters where formats differ.

**Layer 3: Distribution / install packaging** — Marketplace bundles for Claude Code/Codex/Cursor, standalone config for Codex hooks, project/user installs to shared skill paths.

### Design Principles

1. **Don't wrap native commands.** The agent knows `git commit`, `gh pr merge`, etc. intrinsically. Hooks intercept these — they work WITH the agent's natural behavior. The CLI is for Loaf-specific operations (`loaf build`, `loaf check`, `loaf spec`, `loaf task`, etc.).

2. **Hooks stay reactive.** Hooks fire automatically on tool use — the agent doesn't need to remember anything. This is predictable enforcement. Move validation logic to the CLI backend; keep the triggering mechanism as hooks.

3. **Leverage each harness' strengths.** Don't lowest-common-denominator. Codex agents get nicknames, Cursor hooks get richer event configs, OpenCode runtime plugins get native event adapters, Claude Code gets prompt hooks, etc.

4. **Clean source, sidecars at build.** SKILL.md source files contain only standard Agent Skills spec fields (`name`, `description`, `license`, `compatibility`, `metadata`, `allowed-tools`). All tool-specific behavioral fields (`user-invocable`, `context`, `model`, `argument-hint`, etc.) stay in sidecar files (`SKILL.{target}.yaml`), merged into frontmatter at build time. This keeps source portable and tool awareness in a clear, auditable layer.

5. **npm/tsup for now.** The CLI stays on Node.js/tsup. Bun migration is deferred until there's a concrete performance need (e.g., `loaf check` cold start measured and proven slow).

### Artifact Matrix

| Target | Primary build output | Skills source | Hooks artifact | Agents artifact |
|---|---|---|---|---|
| Claude Code | `plugins/loaf/` | Intermediate + Claude sidecar merge | Plugin-bundled `hooks/hooks.json` | Plugin-bundled Markdown |
| OpenCode | `dist/opencode/` | Intermediate + OpenCode sidecar merge | Runtime plugin `.ts` | Markdown + commands |
| Amp | `dist/amp/` | Intermediate (no sidecar needed) | Runtime plugin `.ts` (experimental header) | None |
| Codex | `dist/codex/` | Intermediate + Codex sidecar merge | Standalone at `$CODEX_HOME/hooks.json` | Deferred (TOML) |
| Cursor | `dist/cursor/` | Intermediate + Cursor sidecar merge | Plugin-bundled `hooks/hooks.json` | Plugin-bundled Markdown |
| Gemini | `dist/gemini/` | Intermediate + Gemini sidecar merge | None | None |

---

## Phase 1: Foundation — Shared Content Layer

### Extract shared content modules

Create three shared modules in `cli/lib/build/lib/`:

**`skills.ts`** — Shared skill packaging:

```typescript
interface CopySkillsOptions {
  srcDir: string;
  destDir: string;
  targetName: string;
  version: string;
  targetsConfig: TargetsConfig;
  transformMd: (content: string) => string;
  extraDirs?: string[];
  mergeFrontmatter?: (base: SkillFrontmatter, skillDir: string) => SkillFrontmatter;
}
export function copySkills(options: CopySkillsOptions): void
```

**`agents.ts`** — Shared agent packaging (Markdown-based targets only):

```typescript
interface CopyAgentsOptions {
  srcDir: string;
  destDir: string;
  targetName: string;
  version: string;
  defaults?: Record<string, unknown>;
  sidecarRequired?: boolean;
}
export function copyAgents(options: CopyAgentsOptions): void
```

**`commands.ts`** — Shared command substitution:

```typescript
export function createCommandSubstituter(targetName: string): (content: string) => string
```

Since all targets now use unscoped commands (`/implement`, `/resume`, etc.), the base substitution is universal. Claude Code retains an optional post-substitution pass for `/loaf:` scoping in multi-plugin environments, applied only during plugin bundling.

### Shared skill intermediate

Build a shared skill intermediate at `dist/skills/` during the first phase of `loaf build`. This is a **pre-sidecar-merge, pre-version-injection** staging artifact:
- Contains all skills with universal command substitution applied and base frontmatter only
- No target-specific sidecar fields merged
- No version injected (targets that need it add it during their copy step)
- Shared templates distributed
- References, templates, and scripts copied

Each target then performs a lightweight copy-and-transform from `dist/skills/`:
- **Sidecar merge** — Load `SKILL.{target}.yaml`, merge into frontmatter
- **Version injection** — Targets that inject version do so during copy
- **Extra dirs** — Cursor copies `assets/`, others skip

This eliminates duplicated skill discovery, copying, template distribution, and markdown transformation. The per-target step is just frontmatter enrichment.

**Note:** Do NOT extract the runtime plugin generator in this phase. The OpenCode hook generation logic will be fundamentally rewritten in Phase 3. Extracting it here would create a refactor-then-rewrite cycle.

### Refactor existing targets

Migrate all 5 existing targets to read from the shared intermediate:
- Codex and Gemini first (lowest risk — sidecar merge + version injection only)
- Then Cursor (adds `assets/` dir handling)
- Then OpenCode (adds command generation from sidecars)
- Then Claude Code (adds `/loaf:` scoping pass + Claude-specific sidecar merge via `loadSkillExtensions`)
- Parity checks: byte-identical output for Codex and Gemini; functionally identical for Claude Code, Cursor, OpenCode
- **Bug fix:** Remove prompt hook filter from Cursor target (`cursor.ts` ~line 269). Cursor natively supports `type: "prompt"` hooks — the current filter is unnecessarily restrictive. Covered by functional parity check.

---

## Phase 2: Skill Quality — Cross-Harness Routing

### SKILL.md structural convention

Research across three external harnesses (GSD, gstack, Superpowers) converged on one principle: **the instructions that matter most need the highest attention weight.** The current SKILL.md format treats all content as equal — a topics table has the same visual weight as critical rules. This dilutes the "harness effect" (the behavioral change the skill produces when loaded).

**Structural convention for all SKILL.md files:**

```markdown
---
name: skill-name
description: >- ...
---

# Skill Title

Brief intro (1-2 sentences).

## Critical Rules

[5-15 lines of non-negotiable rules. These ARE the harness effect.
If you removed this section, the model's behavior would change.
Use absolutist language: "Always", "Never", "No exceptions".]

## Verification

[5-10 lines. What to check after work. Hook-to-skill migration
instructions go HERE, not in a reference file. Example:
"After editing Python files, run `mypy --show-error-codes`.
If `mypy` is not available in the project, skip this check."]

## Quick Reference

[10-20 lines. Naming conventions, patterns, cheat sheets.
Only if small enough to scan, not study. Omit if not needed.]

## Topics

| Topic | Reference | Use When |
|-------|-----------|----------|
| ... | [ref.md](references/ref.md) | ... |
```

**Rule:** Everything above the Topics table should change the model's behavior when loaded. Everything in the Topics table is depth on demand (loaded only when the model follows a link). If the always-apply section is empty, the skill is just an index — it's not harnessing anything.

**Instruction tiering by skill type:**

| Skill type | Examples | Always-apply target | Techniques |
|---|---|---|---|
| **Reference** (light) | `python-development`, `database-design` | 5-10 lines: critical rules + verification | Bright-line rules, verification checklist |
| **Cross-cutting** (medium) | `foundations`, `git-workflow`, `security-compliance` | 10-20 lines: iron laws + conventions | Absolutist language, forbidden patterns |
| **Workflow** (heavy) | `implement`, `shape`, `research`, `release` | 20-40 lines: numbered steps + success criteria | Anti-rationalization lists, commitment/announcement, success criteria checklists |

**Adherence techniques** (from Superpowers' research-backed framework — Meincke et al. 2025, Cialdini 2021):

| Technique | Use in | Example |
|---|---|---|
| **Anti-rationalization** | Workflow skills | "If you think 'this is too simple for a spec' — that's exactly when you need one" |
| **Bright-line rules** | All skills | "No bare `except:` clauses. No exceptions." |
| **Forbidden output patterns** | Verification, review | Never say "should work", "probably fine", "looks good" |
| **Success criteria checklists** | Workflow skills | `- [ ] Tests pass` / `- [ ] Types check` |
| **Commitment/announcement** | All invocable skills | "I'm using the implement skill to..." |

**Token budget awareness:** Skills should target these sizes for the always-apply section (everything above Topics table):
- Reference skills: < 500 tokens (~50 lines)
- Cross-cutting skills: < 1000 tokens (~100 lines)
- Workflow skills: < 2000 tokens (~200 lines)
- Total SKILL.md (including Topics table): < 5000 tokens (~500 lines)

### Skill description rework

Optimize all ~30 skill descriptions for cross-harness routing using a **two-tier description strategy**:

**Tier 1 — First 250 characters** (Claude Code truncation boundary):
- Must be self-sufficient for routing: action verb + core domain + primary trigger phrase
- Claude Code truncates descriptions at ~250 chars in the skill listing context budget
- This is the ONLY description text Claude Code's routing model sees

**Tier 2 — Full description** (up to 1024 chars, used by Cursor, Codex, OpenCode, Amp):
- These harnesses read the full description with no truncation
- Add negative routing for confusable skills ("Not for...")
- Add success criteria for workflow skills ("Produces...")
- Add edge-case disambiguation phrases

**Convention:**
```
[Action verb] [core domain] [with technologies]. [Trigger phrases].
← 250-char boundary should fall here or later →
[Negative routing]. [Success criteria for workflow skills].
```

**All descriptions must:**
- Start with third-person action verbs
- Include trigger phrases that work across routing models (truncation, model-driven, agent-decides)
- Have routing-critical phrases (verb + domain + triggers) within first 250 chars
- Use full description space for disambiguation on non-Claude-Code targets

**Build system behavior:** Claude Code target truncates descriptions at 250 chars during sidecar merge. All other targets preserve the full description. No manual truncation needed in source `SKILL.md` files.

### Verb/noun skill principle and skill hygiene

On every harness, **a skill IS a command** — `/skill-name` on Claude Code and Cursor, `$skill-name` on Codex, auto-discovered on Amp, commands-from-sidecar on OpenCode. This means skill granularity directly affects cross-harness command discoverability.

**Principle:** If a user would naturally say "do X" and X is a process, X should be a skill (command). If X is knowledge the agent needs while doing Y, X should be a reference skill.

| Type | Pattern | Instruction tier | Examples |
|---|---|---|---|
| **Verb-skill** (command) | User says "do X" | Workflow (20-40 lines) | `/implement`, `/debug`, `/shape`, `/release`, `/council` |
| **Noun-skill** (reference) | Agent needs X while doing Y | Reference (5-10 lines) | `python-development`, `foundations`, `infrastructure-management` |

**Skill hygiene actions for Phase 2:**

| Action | Skill | Rationale |
|---|---|---|
| **Rename** | `council-session` → `council` | Shorter, cleaner command name. Set `user-invocable: true` |
| **Flip to invocable** | `debugging` | `/debug` is a natural command — "help me debug this" |
| **Evaluate merge** | `resume-session` + `reference-session` | Both are session utilities, not core framework process. Consider merging into one `session` skill or absorbing into `orchestration`. Frees 1-2 description slots in routing pool |
| **Verify verb/noun classification** | All 30 skills | Ensure no processes are buried inside noun-skills as reference files when they should be standalone verb-skills |

### Sidecar audit

Audit all sidecar files across targets:
- 43 `SKILL.claude-code.yaml` files
- 13 `SKILL.opencode.yaml` files
- 0 `SKILL.{codex,cursor,gemini}.yaml` files (create where needed)

Goals:
- Verify every sidecar field is genuinely target-specific (not a field that belongs in base SKILL.md)
- Promote any universal fields (e.g., `description` overrides that should be the base description) back to SKILL.md source
- Document which sidecar fields each tool actually reads and acts on
- Ensure sidecar fields are not duplicated across targets where they should be shared

SKILL.md source files remain clean — standard Agent Skills spec fields only.

#### Invocability semantic mapping

These fields control similar behavior across harnesses but have different semantics:

| Intent | Claude Code | Cursor | Codex |
|---|---|---|---|
| **Reference skill** (model auto-invokes, hidden from user menu) | `user-invocable: false` | No field needed (default behavior) | `agents/openai.yaml` with implicit invocation |
| **Dangerous action** (user must explicitly invoke, model cannot auto-trigger) | `disable-model-invocation: true` | `disable-model-invocation: true` | `agents/openai.yaml` with explicit-only policy |
| **Hidden from all** (neither model nor user can invoke) | Both fields set | Not supported natively | Not supported natively |

For Loaf's reference skills (e.g., `python-development`, `database-design`):
- Claude Code sidecar: `user-invocable: false` (current behavior, correct)
- Cursor sidecar: no `disable-model-invocation` needed (model auto-invocation is desired)
- Codex sidecar: implicit invocation policy (default, no override needed)

### CLI reference skill

Create a non-user-invocable reference skill that teaches agents when to use each `loaf` CLI command. Uses existing `{{COMMAND}}` substitution system for per-target command surfaces.

### Agent profile audit

Review the 3 agent profiles (implementer, reviewer, researcher) against all target capabilities:
- Do profiles translate to Markdown agents (Claude Code, OpenCode, Cursor)?
- What Codex-specific enhancements should apply? (`nickname_candidates`, `model_reasoning_effort`, sandbox modes)
- Does Amp need any agent-like artifacts, or are its native agents sufficient?
- Document target-specific agent enhancements in the agent sidecar files

#### Agent `tools` field format validation

The `tools` field format diverges across harnesses:
- **Claude Code**: comma-separated string (`tools: Read, Glob, Grep`)
- **Cursor**: officially documents only `readonly: true/false` — the `tools` object map (`{Read: true, Write: true}`) that Loaf currently emits is **undocumented**
- **OpenCode**: YAML/Markdown with permission model (`edit: allow`, `bash: deny`)
- **Codex**: TOML with different structure entirely (deferred)

**Action:** Verify whether Cursor respects the `tools` object map. If not, switch read-only agents (reviewer, researcher) to use `readonly: true` instead. Claude Code agents can preload skills via the `skills` frontmatter field — document this as the workaround for Cursor's limitation where subagents can't auto-load skills.

---

## Phase 3: Hook Consolidation

### Hook reclassification

The current hook system has ~35 hooks across pre-tool, post-tool, and session categories. Most non-blocking language/infra/design hooks are **nudges, not enforcement** — the agent already knows how to run these tools via skill instructions. Moving them out of hooks reduces complexity, eliminates latency on every tool call, and makes enforcement cross-harness by default (all harnesses have skills, not all have rich hooks).

**Hooks that STAY as hooks (~15):**

| Category | Hooks | Why hooks? |
|---|---|---|
| **Blocking enforcement** (5) | `check-secrets`, `validate-push`, `workflow-pre-pr`, `validate-commit`, `security-audit` | Can't trust agent; deterministic gates |
| **Prompt hooks** (1) | `workflow-pre-merge` | Text injection, not a script |
| **Advisory git workflow** (1) | `workflow-pre-push` | Pre-push reminder |
| **Side-effect / integration** (3) | `generate-task-board`, `detect-linear-magic`, `kb-staleness-nudge` | Side effect IS the purpose |
| **Post-merge checklist** (1) | `workflow-post-merge` | Advisory checklist injection |
| **Session lifecycle** (6) | `session-start-soul`, `session-start`, `session-end`, `pre-compact-archive`, `kb-session-start`, `kb-session-end` | Different input model (no stdin JSON) |

**Hooks that MOVE to skill instructions (~20):**

| Current hook | Target skill | Instruction form |
|---|---|---|
| `format-check` | `foundations` | "Run formatters (black/prettier/standardrb) before committing" |
| `tdd-advisory` | `foundations` | "Write or update tests alongside implementation changes" |
| `validate-changelog` | `documentation-standards` | "Validate CHANGELOG format before committing" |
| `changelog-reminder` | `documentation-standards` | "Update CHANGELOG after significant changes" |
| `python-type-check`, `python-type-check-progressive` | `python-development` | "Run `mypy --show-error-codes` after editing Python files" |
| `python-ruff-lint`, `python-ruff-check` | `python-development` | "Run `ruff check` before committing Python changes" |
| `python-pytest-validation`, `python-pytest-execution` | `python-development` | "Run `pytest` after significant changes" |
| `python-bandit-scan` | `python-development` | "Run `bandit` on security-sensitive Python code" |
| `typescript-tsc-check` | `typescript-development` | "Run `tsc --noEmit` after editing TypeScript files" |
| `typescript-bundle-analysis` | `typescript-development` | "Check bundle size impact for significant changes" |
| `typescript-eslint-check` | `typescript-development` | "Run ESLint after editing TypeScript/JavaScript" |
| `rails-migration-safety`, `rails-migration-safety-deep` | `ruby-development` | "Check migration safety before committing migrations" |
| `rails-test-execution` | `ruby-development` | "Run Rails tests after significant changes" |
| `rails-brakeman-scan`, `rails-rubocop-check` | `ruby-development` | "Run Brakeman/RuboCop on Ruby code" |
| `infra-validate-k8s`, `infra-k8s-dry-run` | `infrastructure-management` | "Validate K8s manifests after editing" |
| `infra-dockerfile-lint` | `infrastructure-management` | "Lint Dockerfiles after editing" |
| `infra-terraform-plan` | `infrastructure-management` | "Run `terraform plan` after editing .tf files" |
| `design-a11y-validation`, `design-a11y-audit`, `design-token-check` | `interface-design` | "Validate accessibility and design tokens after UI changes" |

**Why skill instructions are better for these:**
- **Cross-harness by default** — all harnesses have skills; Codex can't intercept Edit/Write via hooks at all
- **Agent can exercise judgment** — skip tsc on a WIP scratch file, run full suite before commit
- **No latency tax** — hooks fire on EVERY `Edit|Write`, even for a one-line comment change
- **The agent already knows these tools** — hooks were just nudging it to remember

**Acknowledged tradeoff:** Moving ~20 checks from deterministic hooks to skill instructions means the agent MAY skip them. This is intentional — these checks were non-blocking advisories, not enforcement gates. The SKILL.md structural convention (Critical Rules section with absolutist language, verification checklists) maximizes adherence. The checks that CANNOT be skipped (secrets, git gates, dangerous commands) remain as blocking hooks.

### `failClosed` support

Add `failClosed` field to the hook schema in `hooks.yaml`:

```yaml
hooks:
  pre-tool:
    - id: secrets-check
      skill: foundations
      script: hooks/pre-tool/secrets-check.sh
      matcher: "Edit|Write|Bash"
      failClosed: true   # Block action if hook crashes/times out
```

**Semantics:** When `failClosed: true`, a hook crash, timeout, or non-zero exit (other than 0 or 2) blocks the action instead of allowing it. Default: `false` (fail-open, current behavior).

**Security-critical hooks that should be `failClosed: true`:**
- `secrets-check` — prevents committing secrets
- `security-audit-*` hooks — prevents insecure code patterns
- Any hook where "unknown = unsafe"

**Build output:** Emit `failClosed` in generated `hooks.json` for Claude Code and Cursor (both support it natively). Codex hooks also support it. OpenCode runtime plugin translates via the subprocess error handling path (non-zero, non-2 exit → reject when failClosed is true for that hook).

**Note:** Claude Code's default is also `false` (fail-open). Cursor defaults to `false` with explicit documentation of the `failClosed` field. Both tools' documentation confirms support.

### `loaf check` command

A unified CLI command for the **enforcement hooks** that stay as hooks (5-6 checks, not the full ~25):

```bash
loaf check --hook <hook-id> < /dev/stdin
```

**Checks implemented by `loaf check`:**
- `check-secrets` — scan for hardcoded secrets, API keys, credentials in file content or Bash commands
- `validate-push` — verify version bump, CHANGELOG entry, and successful build before `git push`
- `workflow-pre-pr` — enforce PR title format and CHANGELOG entry before `gh pr create`
- `validate-commit` — validate commit messages follow Conventional Commits conventions
- `security-audit` — detect dangerous Bash command patterns (rm -rf /, chmod 777, eval of untrusted input)

`loaf check`:
- Receives hook context via stdin (same JSON the harness provides)
- Runs the appropriate check for the hook ID
- **Exit code**: 0 = pass (including warnings), 2 = block. Exit 1 = crash/error (always treated as a hook failure, never an intentional result)
- **Stdout**: plain text — human-readable, agent-readable. Warnings use a `WARN:` prefix
- Optional `--json` flag for machine-to-machine use (e.g., future `loaf health` aggregator)
- One invocation per hook — no multi-hook batching

**Interface contract:** `loaf check` output is plain text + exit codes. The per-harness adaptation happens at the **hook config layer**, not inside `loaf check`:

| Harness | How it reads `loaf check` output |
|---|---|
| **Claude Code** | Exit 0 = allow, exit 2 = block (stderr shown to agent). Stdout as `additionalContext`. Native behavior, no adapter needed |
| **Cursor** | Same as Claude Code. `failClosed` field in hooks.json controls crash behavior |
| **Codex** | Exit 2 = block (stderr as reason). **Bash-only matching** — Edit/Write hooks don't fire. `loaf check` output for non-Bash hooks is never reached |
| **OpenCode** | Runtime plugin calls subprocess, reads exit code. Exit 2 → `throw Error(stderr)` to block |

`loaf check` itself is harness-agnostic — the same binary, same exit codes, same output format. The harness-specific behavior is in the **generated hook config** (which events fire, which matchers apply, how stdout/stderr is consumed).

**Scope reduction:** Previously, `loaf check` was envisioned as a backend for ~25 validation hooks including language-specific linting (tsc, mypy, ruff, eslint, rubocop), infra validation (k8s, terraform, dockerfile), and design checks (a11y, tokens). These are now **skill instructions** — the agent runs these tools as part of its workflow, guided by skill content. `loaf check` only implements the checks where the agent cannot be trusted (secrets, git workflow gates, dangerous commands).

**`loaf` binary distribution (prerequisite for Phase 3):**

The entire direct-command hook strategy depends on `loaf` being callable as a binary. This is a Phase 3 prerequisite, not an open question.

**Distribution mechanism:** `npm install -g loaf` (or `npm link` for development). The `loaf` CLI is already a Node.js/tsup-bundled binary with a `bin` entry in `package.json`. It needs to be:
1. **Published to npm** — so users can `npm install -g loaf` and have it on PATH
2. **Bundled in the Claude Code plugin** — `plugins/loaf/bin/loaf` referenced via `${CLAUDE_PLUGIN_ROOT}/bin/loaf`
3. **Verified by `loaf install`** — `loaf install --to <target>` checks that `loaf` is callable from the target's hook execution environment and warns if not

**Per-target resolution:**
- **Claude Code**: `${CLAUDE_PLUGIN_ROOT}/bin/loaf` (plugin-bundled, always available)
- **Cursor**: PATH-based (`loaf` from global npm install). If Cursor sandbox restricts PATH, bundle in `.cursor/bin/loaf`
- **Codex**: PATH-based (`loaf` from global npm install)
- **OpenCode**: PATH-based, called from runtime plugin subprocess

**Build step:** Phase 3 adds a `loaf build` post-step that copies the compiled CLI binary into plugin output directories that need it (Claude Code plugin, Cursor if bundling). This is the bridge between "CLI tool on developer's machine" and "binary callable from hook execution environments."

### Session journal model

Sessions are **append-only structured journals** — a running log of what happened, what was decided, what was learned, and what's next. Think "conventional commits meets bullet journal." Any agent working on a spec/task reads the journal to get context and appends entries to keep it current.

**Session file format:**

```markdown
---
spec: SPEC-020
branch: feat/target-convergence
status: active
created: 2026-03-31T14:30:00Z
last_entry: 2026-03-31T16:00:00Z
---

# SPEC-020: Target Convergence

## 2026-03-31 14:30 — Start
- resume(phase-1): extracting shared modules, 5/12 tasks done
- context: last commit abc1234 "refactor: extract copySkills"

## 2026-03-31 14:45
- discover(cursor): prompt hooks filtered at line 269 — bug
- decide(hooks): remove filter, cursor supports prompt hooks natively

## 2026-03-31 15:10
- block(parity): Codex output differs by trailing newline
- hypothesis: gray-matter serialization adds trailing \n
- try: trim output before byte comparison

## 2026-03-31 15:25
- unblock(parity): confirmed gray-matter quirk, fixed with trim
- commit(def5678): "fix: trim frontmatter for Codex byte parity"

## 2026-03-31 15:50
- spark(descriptions): two-tier strategy? 250-char for CC, full for others
- assume(cursor-routing): reads full description, no truncation — verify in Phase 2
- task(TASK-009): completed — Cursor target migration

## 2026-03-31 16:00 — End
- progress: tasks 5→7 completed
- next: Phase 1 task 8 — OpenCode target migration
- todo: test Codex byte parity on CI, not just local
```

**Entry vocabulary:**

| Type | Meaning | Written by | ID in scope? |
|---|---|---|---|
| `resume` | Session started, current state | Hook (auto) | — |
| `pause` | Session ended, progress summary | Hook (auto) | — |
| `progress` | Task completion delta | Hook (auto) | — |
| `commit(SHA)` | Code committed | Hook (auto) | Yes — short SHA |
| `pr(#N)` | PR created or updated | Hook (auto) | Yes — PR number |
| `merge(#N)` | PR merged | Hook (auto) | Yes — PR number, SHAs |
| `branch(name)` | Branch created or switched | Hook (auto) | Yes — branch name |
| `task(TASK-ID)` | Task started/completed | Hook (auto) | Yes — task ID |
| `linear(ID)` | Linear issue status change | Hook (auto) | Yes — Linear issue ID |
| `decide` | Decision made (with rationale) | Agent | Topic scope |
| `discover` | Something learned | Agent | Topic scope |
| `block` | Something is blocked | Agent | Topic scope |
| `unblock` | Blocker resolved | Agent | Topic scope |
| `assume` | Assumption made (needs validation) | Agent | Topic scope |
| `conclude` | Conclusion reached | Agent | Topic scope |
| `spark` | Idea emerged (promote via `/idea`) | Agent | Topic scope |
| `todo` | Something needs doing (promote to task) | Agent | Topic scope |
| `hypothesis` | Theory being tested | Agent | Topic scope |
| `try` | Approach being attempted | Agent | Topic scope |
| `reject` | Approach abandoned (with reason) | Agent | Topic scope |

**Scope convention:** Scope in parens carries an ID when one exists (`commit(abc1234)`, `pr(#15)`, `task(TASK-008)`, `linear(PLT-123)`), or a topic when it doesn't (`decide(hooks)`, `discover(cursor)`).

**Session = branch scope.** One branch = one session file. Two terminals on the same branch share a session. Different branches have different sessions.

**Edge cases:**
- **`main` / default branch:** Creates a general-purpose session (no spec link). Useful for ad-hoc work, exploration, reviews.
- **Detached HEAD:** Uses the commit SHA as session key. Rare; mainly for bisecting or reviewing specific commits.
- **Branch renamed:** Session file stays as-is (named by original branch creation timestamp + slug). The spec frontmatter `branch:` field is the lookup key, not the session filename.
- **Stacked branches:** Each branch gets its own session. Parent branch context is available via the spec linkage chain, not session inheritance.

**Spec linkage:** Sessions link to specs via frontmatter. Specs link back:

```yaml
# Session frontmatter         # Spec frontmatter
spec: SPEC-020                 branch: feat/target-convergence
branch: feat/target-convergence  session: 20260331-143000-target-convergence.md
```

Tasks link to specs, specs link to sessions — the chain is `task → spec → session`.

### `loaf session` subcommands

```bash
loaf session start   # Find/create session for current branch, append resume entry, output context
loaf session end     # Append pause entry with progress, prompt for final decide/conclude/todo entries
loaf session log     # Append a typed entry: loaf session log "decide(hooks): remove bash wrappers"
loaf session archive # Move session to archive when branch merges, extract key decisions
```

**What `loaf session start` does:**
1. Validates SOUL.md is present; restores from template if missing
2. Detects current git branch
3. Finds linked spec (branch name → spec frontmatter `branch:` field)
4. Finds/creates session file for this branch in `.agents/sessions/`
5. Computes current state: task completion (from spec's tasks), recent commits (from git log), branch status
6. Appends `resume` entry with computed state
7. Outputs last 15-20 journal entries as context to the model
8. Surfaces stale knowledge count (from kb-session-start logic)

**What `loaf session end` does:**
1. Appends `pause` entry with progress summary (tasks completed this session, commits made)
2. Injects prompt asking agent to append any final `decide`/`conclude`/`todo` entries
3. Updates session frontmatter `last_entry` timestamp

**What `loaf session log` does:**
1. Receives entry text as argument or via stdin
2. Validates entry follows `type(scope): description` format
3. Appends timestamped entry to current branch's session file
4. Used by hooks (auto-entries) and agents (manual entries)

**What `loaf session archive` does:**
1. Triggered when branch merges (or manually)
2. Moves session file to `.agents/sessions/archive/`
3. Sets status to `archived` in frontmatter
4. Extracts key decisions (`decide` entries) into spec changelog or memory

**Hook-automated journal entries:**

| Hook event | Trigger | Journal entry |
|---|---|---|
| SessionStart | Every session | `resume(context): branch, spec, task status, last commit SHA` |
| PostToolUse on `Bash(git commit)` | After commit | `commit(SHA): "commit message"` |
| PostToolUse on `Bash(gh pr create)` | After PR creation | `pr(#N): created "PR title"` |
| PostToolUse on `Bash(gh pr merge)` | After merge | `merge(#N): squash into main, SHA1→SHA2` |
| Stop | Session ends | `pause: progress summary` |
| PreCompact | Before compaction | `compact: context preserved, N entries in journal` |

**Agent-written entries:** The agent appends `decide`, `discover`, `block`, `spark`, `assume`, `hypothesis`, `try`, `reject`, `conclude`, `todo` entries during normal work. These are the high-value context that makes the journal useful for the next conversation or another agent.

**Journal nudge hooks** (compel agent to write high-value entries):

| Hook event | Condition | Nudge injected |
|---|---|---|
| Stop | Edits or commits happened this turn but zero `decide`/`discover`/`conclude` entries written | "Log key decisions or discoveries to the session journal. See the `orchestration` skill for journal entry types and format: `decide(scope)`, `discover(scope)`, `conclude(scope)`, `spark(scope)`, etc." |
| PostToolUse on `Bash(git commit)` | Always, after auto-logging `commit(SHA)` | "What was the key decision behind this commit? See the `orchestration` skill for journal conventions. Use: `loaf session log 'decide(scope): rationale'`" |
| PreCompact | Always, before context compaction | "Context is about to compact. See the `orchestration` skill for journal entry types and log any unrecorded decisions, blockers, or discoveries now." |

These are **nudges, not blocks** — they inject prompts that make it psychologically harder for the agent to skip journaling, and **point to the skill** that defines the vocabulary and patterns. The Stop hook is the strongest signal: if you did work but didn't explain why, the hook asks you to and tells you how.

**Cross-agent protocol:** When any agent starts work on a branch, `loaf session start` outputs the last 15-20 journal entries. The agent reads context, continues work, appends entries. Subagents delegated to tasks read and write to the same session file. The journal becomes the shared context layer across all agents working on a branch.

**Concurrency and permissions:**
- Journal writes go through `loaf session log` (a Bash command), not through Edit/Write tools. This means read-only agents (reviewer, researcher) CAN write journal entries — the tool restriction is about code modification, not journal logging.
- Concurrent appends: `loaf session log` uses atomic append (`>>` with a single write call). Two agents writing simultaneously may interleave entries but cannot corrupt the file. Entry timestamps provide ordering.
- No file locking required — append-only journals are naturally safe for concurrent writers.

**Absorbs `resume-session` and `reference-session` skills:**
- "Resume" = `loaf session start` (auto-detects branch, loads journal context)
- "Reference past session" = read an archived journal file (it's just a file in `.agents/sessions/archive/`)
- Both standalone skills can be removed

Session hooks in hook configs call `loaf session` directly:

```jsonc
// Claude Code (plugin.json)
"SessionStart": [{ "command": "\"${CLAUDE_PLUGIN_ROOT}/bin/loaf\" session start" }]
// Cursor (hooks.json)
"sessionStart": [{ "command": "loaf session start" }]
```

Git-related hooks (commit, PR, merge) call `loaf session log`:

```jsonc
// Claude Code (plugin.json)
"PostToolUse": [{
  "matcher": "Bash",
  "if": "Bash(git commit:*)",
  "hooks": [{ "command": "\"${CLAUDE_PLUGIN_ROOT}/bin/loaf\" session log --from-hook" }]
}]
```

`--from-hook` flag tells `loaf session log` to parse the hook's stdin JSON and extract the relevant ID (commit SHA from git output, PR number from gh output).

### Direct command generation (no bash wrappers)

The ~25 validation hook scripts are **eliminated entirely**. Instead of generating bash wrapper scripts that call `loaf check`, the build system generates `hooks.json` entries that reference `loaf check` directly as the command:

```jsonc
// Claude Code (plugin.json) — command references plugin-bundled binary
"PreToolUse": [{
  "matcher": "Edit|Write|Bash",
  "hooks": [{
    "type": "command",
    "command": "\"${CLAUDE_PLUGIN_ROOT}/bin/loaf\" check --hook secrets-check",
    "timeout": 30
  }]
}]

// Cursor (hooks.json) — loaf on PATH or plugin-relative
"preToolUse": [{
  "command": "loaf check --hook secrets-check",
  "matcher": "Shell|Edit|Write",
  "timeout": 30,
  "failClosed": true
}]

// Codex (.codex/hooks.json) — loaf on PATH
"PreToolUse": [{
  "command": "loaf check --hook secrets-check",
  "matcher": "Bash"
}]
```

**Why this works:** All three shell-hook harnesses (Claude Code, Cursor, Codex) accept a raw shell command in the `command` field — not just shell scripts. Codex docs explicitly show `python3 script.py` as an example. The bash wrapper layer was solving a problem that doesn't exist.

**What the build generates per target:**
- **Claude Code**: `plugin.json` with `loaf check` commands using `${CLAUDE_PLUGIN_ROOT}/bin/loaf` path
- **Cursor**: `hooks.json` with `loaf check` commands (PATH-based or plugin-relative)
- **Codex**: `.codex/hooks.json` with `loaf check` commands (PATH-based)
- **OpenCode / Amp**: Runtime plugin `.ts` that calls `loaf check` as subprocess (same as before)

**Per-target differences handled at build time:**
- Event names: `PreToolUse` (CC, Codex) vs `preToolUse` (Cursor) vs `tool.call` (OpenCode/Amp)
- Matcher tool names: Codex only supports `Bash`; Cursor has separate shell/MCP/file events
- Config schema: CC nests hooks inside matcher groups; Cursor is flat
- `failClosed`: emitted for CC/Cursor/Codex (all support it)
- Binary path: `${CLAUDE_PLUGIN_ROOT}/bin/loaf` (CC), PATH-based (Cursor/Codex), subprocess (OpenCode/Amp)

**Eliminated artifacts:** ~25 bash wrapper scripts (previously `content/hooks/pre-tool/*.sh` validation hooks). These are replaced by direct `hooks.json` command entries generated at build time. The 4 shared shell libraries (`lib/json-parser.sh`, `config-reader.sh`, `agent-detector.sh`, `timeout-manager.sh`) are also eliminated — their logic moves into `loaf check` TypeScript.

**Remaining shell scripts** (not eliminated):
- Session hooks: direct `loaf session start/end` commands in generated hook configs (same as enforcement hooks — no wrappers)
- Side-effect hooks: 2-3 scripts that stay as-is (task-board generation, archive-context)
- CLI-wrapping hooks: 1-2 scripts that stay as-is (kb-staleness-nudge)

**Exit code contract is unchanged:** `loaf check` returns 0 (pass/warn) or 2 (block). Exit 1 = internal error. The harness interprets exit codes identically whether they come from a bash wrapper or a direct command.

### Hooks that stay as hooks

**CLI-wrapping hooks** (already call `loaf` commands — wrapping them in `loaf check` would be double-wrapping):
- `kb-staleness-nudge` — already calls `loaf kb check --file "$file_path" --json`, manages per-session state via temp files. Stays as-is
- `kb-session-end` — paired with kb-staleness-nudge, cleans up temp files. Absorbed into `loaf session end`

**Side-effect hooks** (exist to trigger file generation, not to validate):
- `orchestration-generate-task-board` — calls `scripts/generate-task-board.sh` to regenerate `TASKS.md` when task files change. Stays as a hook; the side effect IS the purpose
- `archive-context` — creates `.context-snapshots/` archives. Stays as a hook

**Prompt hooks:**
- `workflow-pre-merge` — text injection defined in hook config (`type: prompt`). Not a script. Passes through to `hooks.json` unchanged

### Runtime plugin unification

Create `cli/lib/build/lib/hooks/runtime-plugin.ts`:

```typescript
interface RuntimePlatform {
  platform: 'opencode' | 'amp';
  header: string;
  events: {
    preTool: string;
    postTool: string;
    sessionStart: string;
    sessionEnd?: string;
  };
  toolNameAccessor: string;
  wrapExport: (body: string) => string;
  rejectPattern: (msgVar: string) => string;
  /** Platform-specific events beyond the shared core (e.g., OpenCode's 30+ events) */
  extraEvents?: (config: HooksConfig, srcDir: string) => string;
}
export function generateRuntimePlugin(
  config: HooksConfig, srcDir: string, distDir: string, platform: RuntimePlatform
): void
```

Shared core: `runHook()` (calls `loaf check` / `loaf session` as subprocess), `matchesTool()`, hook grouping, exit code interpretation, `failClosed` handling. Per-platform adapter handles event wiring and response patterns.

**`extraEvents` callback:** OpenCode has 30+ events (including `session.compacting`, file events, LSP events, shell environment injection). The shared interface covers the common denominator (`preTool`, `postTool`, `sessionStart`, `sessionEnd`). Platform-specific events are emitted by `extraEvents`:

- **OpenCode `extraEvents`:** Maps `session.compacting` → `loaf session log "compact: context compaction triggered"`, file change events → side-effect hooks, environment injection for shell hooks

**Compaction hooks:** The `session.compacting` (OpenCode) and `preCompact` (Cursor) events append a `compact` journal entry to the session — they do NOT archive the session (archival only happens on branch merge). For Cursor this is handled via the shell hook path. For OpenCode it routes through `extraEvents`.

---

## Phase 4: New Targets & Install Convergence

### Amp target

New file `cli/lib/build/targets/amp.ts`. Output:

```
dist/amp/
├── skills/           # copied from shared intermediate
│   └── {skill-name}/
│       └── SKILL.md
└── plugins/
    └── loaf.ts       # generated runtime plugin (experimental header)
```

- Calls shared `copySkills()` (intermediate → dist/amp/ — trivial copy, no sidecar merge)
- Calls shared `generateRuntimePlugin()` with Amp adapter
- Skips agents (Amp has native agents)
- Skips command generation (Amp auto-discovers skills)

Register in: `cli/commands/build.ts`, `config/targets.yaml`, `cli/lib/detect/tools.ts`, `cli/lib/install/installer.ts`.

**Note:** Amp's plugin API is experimental. The generated plugin includes a `// @i-know-the-amp-plugin-api-is-wip-and-very-experimental-right-now` header as required by Amp. If the API breaks, the plugin is low-cost to regenerate from the shared `RuntimePlatform` interface.

### Codex hook output (Bash enforcement only)

Generate Codex hook configuration as a build artifact:
- Output: `dist/codex/.codex/hooks.json` (direct `loaf check` commands)
- **Only Bash-matching enforcement hooks**: `check-secrets`, `validate-push`, `validate-commit`, `workflow-pre-pr`, `security-audit`
- Do NOT generate hooks for Edit/Write matchers (Codex can't intercept them)
- Install places at `$CODEX_HOME/hooks.json` (respecting `$CODEX_HOME` env var)
- `$CODEX_HOME` is always used if set; no hardcoded fallback path

### Install convergence

Converge install logic around shared `.agents/skills/` path:

| Tool | Skills install destination | Hooks install destination |
|---|---|---|
| Amp | `.agents/skills/` or `~/.config/agents/skills/` | `.amp/plugins/` or `~/.config/amp/plugins/` |
| Codex | `.agents/skills/` or `~/.agents/skills/` | `$CODEX_HOME/hooks.json` |
| Cursor | `.agents/skills/` (native discovery) | Plugin-bundled |
| Claude Code | Plugin-bundled | Plugin-bundled |
| OpenCode | Plugin-specific path | Plugin-specific path |

**User-owned hooks coexistence:** For targets where Loaf installs hooks into a shared config file (Codex `$CODEX_HOME/hooks.json`, Cursor project-level `hooks.json`):
- `loaf install` writes only Loaf-namespaced hook entries (prefixed with `loaf-` or scoped by a `loaf` key)
- Existing user hooks in the same file are preserved — Loaf never overwrites the entire file
- `loaf install --upgrade` replaces only Loaf-owned entries, leaving user entries untouched

### Fenced-section management for project instructions

Loaf framework conventions (session journal vocabulary, verification patterns, CLI commands) must be visible to the agent in the project's CLAUDE.md or AGENTS.md — but Loaf cannot own the whole file. Solution: **fenced sections** with HTML comment delimiters.

**Format:**

```markdown
# My Project Instructions

[user content — Loaf never touches anything outside the fences]

<!-- loaf:managed:start v2.1.0 -->
<!-- Maintained by loaf install/upgrade — do not edit manually -->
## Loaf Framework

[Compact framework essentials: session journal entry types, loaf CLI commands,
 verification conventions, link to orchestration skill for full details]
<!-- loaf:managed:end -->
```

**Behavior:**
- `loaf install` creates the fenced section at the end of the file (or creates the file if missing)
- `loaf install --upgrade` finds `<!-- loaf:managed:start ... -->` / `<!-- loaf:managed:end -->` markers and replaces only the content between them. User content above/below is preserved
- Version in the start marker (`v2.1.0`) lets upgrade decide whether to refresh — skip if already current
- If fences are not found (user deleted them), `loaf install --upgrade` appends a new fenced section (does not search-and-replace elsewhere in the file)
- The fenced content is generated at build time from the framework reference skill content, kept compact (~20-30 lines)

**Per-target file:**
| Target | File | Notes |
|---|---|---|
| Claude Code | `.claude/CLAUDE.md` | Fenced section at end. Plugin skills provide full detail on demand |
| Cursor | `.cursor/rules/loaf.mdc` or `.agents/AGENTS.md` | Could use an always-apply `.mdc` rule instead of fencing AGENTS.md |
| Codex | `.agents/AGENTS.md` or `.codex/AGENTS.md` | Codex reads AGENTS.md natively |
| OpenCode | `.agents/AGENTS.md` | OpenCode reads AGENTS.md |
| Amp | `.agents/AGENTS.md` | Amp reads AGENTS.md |

**Three-layer framework instruction model:**

| Layer | Content | When loaded | Maintained by |
|---|---|---|---|
| **Fenced section** (CLAUDE.md/AGENTS.md) | Compact essentials (~20-30 lines) | Every conversation (always-loaded) | `loaf install/upgrade` |
| **Framework reference skill** (non-user-invocable) | Full conventions, entry vocabulary, command surfaces | On demand (model loads when needed) | `loaf build` |
| **SessionStart hook output** | Live context (journal entries, spec/branch/task state) | Every session start | `loaf session start` |
- Claude Code and OpenCode use plugin-bundled hooks (no shared file), so no coexistence concern

### Codex hook enforcement limitation

Codex `PreToolUse` hooks only match `Bash` tool calls. File edits via the native Edit/Write tools are **uninterceptable**. The docs explicitly note: "Model can work around this by writing scripts, so treat as guardrail." This means:

- File-level validation hooks (format checks, security scans) cannot fire on Codex via hooks
- Bash command validation (dangerous command prevention, secrets in commands) works normally
- The skill instructions layer (SKILL.md content) is the primary enforcement mechanism on Codex, not hooks
- `failClosed` is less meaningful on Codex given the limited blocking surface

This is a platform limitation, not a Loaf limitation. The build system should still generate Codex hooks for the Bash checks that do work, but the hook coverage gap should be documented.

---

## Scope

### In Scope

- Extract `copySkills()`, `copyAgents()`, `substituteCommands()` into shared modules
- Build shared skill intermediate (`dist/skills/` — pre-sidecar, pre-version)
- Refactor all 5 existing targets onto shared intermediate + per-target transform
- Rework all ~30 skill descriptions for cross-harness routing (two-tier strategy)
- Audit all sidecar files; promote universal fields to base SKILL.md
- Create CLI reference skill (non-user-invocable, per-target command surfaces)
- Audit 3 agent profiles across all target capabilities
- Update README.md to reflect all SPEC-020 changes (Phase 2 deliverable)
- Implement fenced-section management for user project CLAUDE.md/AGENTS.md files (Phase 4 — install convergence)
- Reclassify ~20 non-blocking language/infra/design hooks as skill instructions (move to SKILL.md content)
- Build `loaf check` CLI command for ~5-6 enforcement hooks (secrets, git workflow, security)
- Build `loaf session start/end/log/archive` subcommands with journal model (append-only structured log, auto-entries from hooks, branch-scoped, spec-linked)
- Remove `resume-session` and `reference-session` skills (absorbed by `loaf session start` and archived journal reads)
- Generate direct `loaf check` commands into `hooks.json` — no intermediate bash wrapper scripts
- Remove ~20 validation hook scripts that become skill instructions; remove 4 shared shell libraries
- Generate `loaf session` commands into hook configs for session lifecycle (3 thin wrappers remain for env var setup)
- Leave CLI-wrapping hooks as-is (kb-staleness-nudge, etc. — already call `loaf` commands)
- Leave side-effect hooks as-is (task-board generation, detect-linear-magic — side effects ARE the purpose)
- Leave prompt hooks unchanged (pass through to hook config)
- Create shared runtime plugin generator for OpenCode + Amp
- Amp target: skills from shared intermediate + runtime plugin (experimental header)
- Add Codex hook output for Bash-matching enforcement hooks only (respecting `$CODEX_HOME`)
- Converge install logic around `.agents/skills/`
- Publish `loaf` to npm and bundle binary in plugin output for sandboxed hook environments (Claude Code, Cursor)

### Out of Scope

- Codex TOML agent generation (`.codex/agents/*.toml`) — separate follow-on
- Wrapping native commands (git, gh) in Loaf CLI commands
- Skill activation analytics / test harness — separate feature
- `loaf health` aggregation command — separate concern
- Bun migration — deferred until concrete performance need is measured (npm/tsup works)
- Amp agents, `registerTool()`, or `registerCommand()` — Amp gets skills + runtime plugin, not custom tools or agents
- Amp sidecar files (`SKILL.amp.yaml`) unless a real override appears
- Cursor advanced hook expansion beyond preserving current behavior
- Gemini target work beyond shared skill packaging
- Distribution repo workflow (marketplace publishing pipeline)
- Codex hooks for Edit/Write matchers (platform limitation — Bash-only)

### Rabbit Holes

- Don't build a generic "TargetCapabilities" framework — a few shared modules, not a meta-build system
- Don't try to make Amp work without `PLUGINS=all` — that's Amp's requirement
- Don't force Claude Code, Codex, and Cursor into one shell-hook generator — Cursor's event surface and Codex's standalone hook placement make that premature. The shared part is `loaf check` (the logic); the per-target part is `hooks.json` generation (the config)
- Don't generate bash wrapper scripts for validation hooks — the harnesses accept `loaf check` as a direct command. Bash wrappers are an unnecessary indirection layer
- Don't roll Codex TOML agents into this spec
- Don't replace native commands with `loaf` wrappers — hooks intercept, don't replace
- Don't over-abstract hook response formats — each harness family has its own exit code / response contract
- Don't extract the runtime plugin generator in Phase 1 — it gets rewritten in Phase 3 to use `loaf check`
- Don't try to batch multiple hooks into one `loaf check` invocation — harnesses fire each hook entry independently (Claude Code groups by matcher in config, but each hook runs its own command). One `loaf check` call per hook
- Don't wrap hooks that already call `loaf` commands (kb-staleness-nudge calls `loaf kb check`) — double-wrapping adds latency and complexity for zero benefit

## Compatibility / Migration Risks

| Change | Risk | Mitigation |
|---|---|---|
| Hook-to-skill migration | Agents may not run checks as consistently as hooks did | Skill instructions should be clear about when to run checks; accept that agent judgment replaces deterministic firing |
| OpenCode `hooks.js` → `hooks.ts` | Users manually referencing old filename | Call out in release notes. Install command updates OpenCode config if needed |
| Shared `skills.ts` / `agents.ts` refactor | Output parity may change | Byte-level parity checks for Codex/Gemini; functional parity for others |
| `loaf check` replaces validation hooks | Different error handling or output format | Test each hook's behavior before/after migration. Direct command generation means no intermediate scripts to debug |
| Validation hook scripts eliminated | Users referencing hook scripts directly would break | Hook scripts were never a public API — they were build artifacts. Release notes should document the change |
| Shared shell libraries eliminated | Any hook still sourcing `lib/json-parser.sh` etc. would fail | Remaining hooks (side-effect, CLI-wrapping) must be audited for library dependencies before removal |
| `loaf session` replaces session hooks | Session output format may change | Compare session-start output before/after |
| Codex emits `.codex/hooks.json` | Multi-artifact build for Codex | Treat hooks as explicit install concern |
| Shared `.agents/skills/` becomes first-class | Assumptions about target-specific `dist/` paths | Preserve target outputs, converge install incrementally |
| `loaf` binary must be on PATH for hooks | Users with local-only installs get hook failures | Build resolves path; install verifies accessibility |
| Cursor sandbox PATH | Cursor may restrict PATH in hook execution environment | Test early in Phase 3; bundle alongside hooks if needed |

## Dependencies

| Dependency | Type | Notes |
|---|---|---|
| SPEC-018 | Soft (parallel) | OpenCode hooks parity. Runtime plugin extraction here makes SPEC-018's fixes easier |
| SPEC-014 | Hard (resolved) | Skill activation model already in place |

## Deferred Follow-On Work

- **Codex TOML agent generation** — Markdown→TOML transform + `.codex/agents/` install
- **Skill routing validation** — `loaf skill test` command: verify truncation-safe trigger phrases (first 250 chars), confusable skill disambiguation, cross-harness routing simulation. Supersedes the narrower "skill activation analytics" concept
- **Distribution repo** — Release/publishing pipeline for marketplace bundles
- **Cursor advanced hook events** — Leverage Cursor's 18+ event surface: `subagentStart` (inject skill refs for subagents that can't auto-load), `subagentStop`, `beforeShellExecution` (shell-specific validation), `beforeMCPExecution`, `beforeReadFile`, `afterFileEdit`, `afterAgentResponse`. Audit each for `failClosed` applicability
- **Codex feature flag handling** — `loaf install --to codex` should detect and warn about `[features] codex_hooks = true` requirement, or set it automatically if Codex config is writable
- **Bun migration** — Migrate CLI from Node.js/tsup to Bun for native TypeScript execution. Benchmark `loaf check` cold start under both. Justified when performance data shows measurable benefit
- **Amp advanced features** — `registerTool()` custom tools, `registerCommand()` entries, agent definitions when Amp API stabilizes further
- **`loaf health`** — Aggregation command for SessionStart signals
- **OpenCode extended events** — Leverage OpenCode's file events, LSP events, and `shell.env` (environment injection) beyond the shared `extraEvents` callback. Requires OpenCode plugin API stabilization

## Test Conditions

### Phase 1: Foundation
- [ ] `loaf build` succeeds for all targets (Node.js/tsup)
- [ ] Shared skill intermediate exists at `dist/skills/` after build
- [ ] `dist/skills/` contains all skills with base frontmatter (no sidecar fields, no version) and universal command substitution applied
- [ ] `loaf build --target codex` final output is byte-identical before/after shared module extraction
- [ ] `loaf build --target gemini` final output is byte-identical before/after
- [ ] `loaf build --target claude-code` final output is functionally identical
- [ ] `loaf build --target cursor` final output is functionally identical (including prompt hooks passing through — no longer filtered)
- [ ] `loaf build --target opencode` final output is functionally identical
- [ ] `npm run typecheck` passes
- [ ] `npm run test` passes

### Phase 2: Skill Quality
- [ ] All SKILL.md files follow structural convention: Critical Rules → Verification → Quick Reference → Topics (in that order)
- [ ] Every skill has a non-empty "Critical Rules" or equivalent always-apply section above the Topics table
- [ ] Reference skills: always-apply section < 500 tokens; Cross-cutting: < 1000; Workflow: < 2000
- [ ] Total SKILL.md files < 5000 tokens (~500 lines) — overflow content extracted to references/
- [ ] Workflow skills have success criteria checklists and anti-rationalization guidance where applicable
- [ ] All skill descriptions have routing-critical phrases (action verb + domain + trigger) within first 250 chars (verified by `loaf build` output logging each description's char count and first-250 preview)
- [ ] Full descriptions (up to 1024 chars) include negative routing for confusable skills and success criteria for workflow skills
- [ ] Claude Code target truncates descriptions at 250 chars; all other targets preserve full description
- [ ] All descriptions start with third-person action verbs
- [ ] ~20 hook-to-skill migrations complete — verification instructions added to "Verification" section of relevant SKILL.md files
- [ ] Migrated skills include tool availability checks ("if `mypy` is available, run it") not hard requirements
- [ ] `council-session` renamed to `council`, set `user-invocable: true`
- [ ] `debugging` set to `user-invocable: true`
- [ ] `resume-session` and `reference-session` marked for removal in Phase 3 (absorbed by session journal model)
- [ ] All skills classified as verb (command) or noun (reference) — no processes buried inside noun-skills
- [ ] Sidecar audit complete — every field documented as genuinely target-specific or promoted to base
- [ ] Invocability semantic mapping documented and applied (user-invocable ↔ disable-model-invocation ↔ Codex policy)
- [ ] Agent `tools` field format validated per-target (Cursor: confirm support or switch to `readonly`)
- [ ] CLI reference skill exists with per-target command substitution
- [ ] README.md updated: skill count, session model description, hooks description, pipeline commands, multi-target table (add Amp, Codex hooks), skill rename/removals
- [ ] CLAUDE.md updated: SKILL.md structural convention, verb/noun principle, two-tier descriptions, session journal vocabulary, `loaf check` and `loaf session` commands, hook model changes
- [ ] Agent profiles: each profile has verified sidecar fields per target (CC: tools string, Cursor: readonly or validated tools map, Codex: deferred TOML, OpenCode: permission model)

### Phase 3: Hook Consolidation
- [ ] `loaf check --hook <id>` runs successfully for all enforcement hook IDs (`check-secrets`, `validate-push`, `workflow-pre-pr`, `validate-commit`, `security-audit`)
- [ ] `loaf check` uses exit codes: 0=pass (including warnings), 2=block, 1=internal error only
- [ ] `loaf check --json` returns structured JSON for machine consumers
- [ ] `loaf session start` detects current branch, finds/creates session file, appends `resume` entry, outputs last 15-20 entries
- [ ] `loaf session start` auto-links to spec when branch matches spec frontmatter `branch:` field
- [ ] `loaf session start` creates ad-hoc session for branches without a linked spec
- [ ] `loaf session end` appends `pause` entry with progress summary (commits, tasks completed)
- [ ] `loaf session log` appends typed entries in `type(scope): description` format
- [ ] `loaf session log --from-hook` parses hook stdin JSON and extracts IDs (commit SHA, PR number, etc.)
- [ ] `loaf session archive` moves session to `.agents/sessions/archive/`, sets status to `archived`
- [ ] PostToolUse hooks for `git commit`, `gh pr create`, `gh pr merge` auto-append journal entries with SHAs/PR numbers
- [ ] Journal entries include unique IDs when available: commit SHAs, PR numbers, task IDs, Linear issue IDs, branch names
- [ ] `resume-session` and `reference-session` skills removed
- [ ] Stop hook nudges for journal entries when edits/commits happened but no decide/discover/conclude entries written
- [ ] Post-commit hook nudges for decision rationale after auto-logging commit SHA
- [ ] PreCompact hook nudges for journal flush before context compaction
- [ ] Generated `hooks.json` for Claude Code contains direct `loaf check --hook <id>` commands (no bash wrapper scripts)
- [ ] Generated `hooks.json` for Cursor contains direct `loaf check --hook <id>` commands
- [ ] Generated `.codex/hooks.json` contains direct `loaf check --hook <id>` commands (Bash-matching enforcement only)
- [ ] ~20 migrated hook scripts removed from build output; 4 shared shell libraries removed
- [ ] Remaining hook scripts (side-effect, CLI-wrapping) audited for shared library dependencies before lib removal
- [ ] Session hooks call `loaf session start/end` directly in generated hook configs
- [ ] Git hooks call `loaf session log --from-hook` for auto journal entries
- [ ] CLI-wrapping hooks (kb-staleness-nudge, etc.) left unchanged — verified still functional
- [ ] Side-effect hooks (task-board, archive-context) left unchanged — verified still functional
- [ ] Prompt hooks pass through unchanged
- [ ] Hook behavior is functionally identical before/after migration for all hook categories
- [ ] Runtime plugin generated for OpenCode using `loaf check`/`loaf session` backend
- [ ] Runtime plugin generated for Amp using `loaf check`/`loaf session` backend
- [ ] Generated runtime plugins are valid TypeScript
- [ ] Session lifecycle events (`session.start`/`session.created`) mapped correctly in both runtime adapters
- [ ] `loaf` binary accessible from Claude Code, Cursor, and Codex hook environments
- [ ] Security-critical hooks emit `failClosed: true` in generated `hooks.json`
- [ ] OpenCode plugin handles `session.compacting` event via `extraEvents` callback
- [ ] `failClosed` field emitted in hooks.json for Claude Code, Cursor, and Codex

### Phase 4: New Targets & Install
- [ ] `loaf build --target amp` produces `dist/amp/skills/` + `dist/amp/plugins/loaf.ts`
- [ ] Generated Amp plugin includes experimental header comment
- [ ] Generated Amp plugin maps `tool.call` and `tool.result` events correctly
- [ ] `loaf install --to amp` copies skills to correct paths (`.agents/skills/` or `~/.config/agents/skills/`)
- [ ] Codex build produces hook output at `dist/codex/.codex/hooks.json` with Bash-matching enforcement hooks only
- [ ] `loaf install --to codex` respects `$CODEX_HOME` env var
- [ ] Install convergence works for shared `.agents/skills/` path across Amp, Codex, and Cursor
- [ ] `loaf install` creates fenced section in target's CLAUDE.md/AGENTS.md (or creates the file)
- [ ] `loaf install --upgrade` replaces content between fences only — user content outside fences preserved
- [ ] Fenced section includes version marker; upgrade skips if already current
- [ ] Fenced content is compact (~20-30 lines) with links to framework reference skill for full details
- [ ] `loaf` binary is accessible from hook execution environments (verified for Claude Code, Cursor, Codex)

## Cross-Harness Compatibility Reference

Research-derived matrix documenting how each harness handles skills, hooks, agents, and commands. Use during implementation to verify assumptions.

| Dimension | Claude Code | Cursor | Codex | OpenCode | Amp | Gemini |
|---|---|---|---|---|---|---|
| **Skill path** | `.claude/skills/` | `.agents/skills/`, `.cursor/skills/` | `.agents/skills/` | `.opencode/`, `.claude/`, `.agents/skills/` | `.agents/skills/` | `.agents/skills/` |
| **Skill format** | `SKILL.md` + YAML | `SKILL.md` + YAML | `SKILL.md` + YAML | `SKILL.md` + YAML | `SKILL.md` + markdown | `SKILL.md` + YAML |
| **Description routing** | 250-char truncation, model-driven | Full description, agent-decides | Full description, model-driven | Metadata-first, on-demand | Model-driven auto-select | Unknown |
| **Hook format** | `hooks.json` (settings/plugin) | `hooks.json` (18+ events) | `hooks.json` (5 events, feature-flagged) | TS plugin (30+ events) | TS plugin (5 events) | None |
| **`failClosed` support** | Yes (default: false) | Yes (default: false) | Yes | Via subprocess error handling | Via subprocess error handling | N/A |
| **Agent format** | Markdown + YAML | Markdown + YAML | TOML (`.codex/agents/`) | Markdown/YAML | Native agents | None |
| **Agent tool restriction** | `tools: Read, Glob` (string) | `readonly: true` (documented); `tools` map (undocumented) | TOML structure | YAML permissions | N/A | N/A |
| **Command routing** | `/skill` or `/plugin:skill` | `/skill-name` | `$skill` or `/skills` | Via commands config | Auto-discover | None |
| **Invocability control** | `user-invocable`, `disable-model-invocation` | `disable-model-invocation` | `agents/openai.yaml` policy | N/A | N/A | N/A |
| **Compaction hook** | Pre-compact (context-archiver agent) | `preCompact` event | None | `session.compacting` event | None | None |

**Sources:** Claude Code docs (code.claude.com), Cursor docs (cursor.com/docs), Codex docs (developers.openai.com/codex), OpenCode docs (opencode.ai/docs), Amp manual (ampcode.com/manual), Agent Skills spec (agentskills.io/specification). Research conducted 2026-03-31.

## Changelog

- 2026-03-31 — Fenced-section management: framework conventions installed into user project CLAUDE.md/AGENTS.md via HTML comment fences. `loaf install` creates, `loaf install --upgrade` replaces only between fences. Three-layer model: fenced section (always-loaded, compact) + framework reference skill (on-demand, full) + SessionStart hook (live context). Per-target file selection documented.
- 2026-03-31 — Codex review fixes (15 findings): Fixed Amp scope contradiction (restored runtime plugin, kept experimental header). Fixed session archive/compaction conflation (compaction = journal entry, archival = branch merge only). Added per-harness loaf-check output adapter table. Added session concurrency/permissions model (atomic append, read-only agents write via loaf session log). Strengthened binary distribution as Phase 3 prerequisite (npm publish + plugin bundling). Added hook-to-skill enforcement tradeoff acknowledgment. Added user-hooks coexistence policy. Cleaned Bun references, standardized Codex invocation terminology, added branch-scoped session edge cases, added description char-count verification method. No backwards compatibility needed per user direction.
- 2026-03-31 — Session journal model: replaced static session state files with append-only structured journals ("conventional commits meets bullet journal"). Branch-scoped, spec-linked, auto-maintained by hooks (commit SHAs, PR numbers, merge IDs), enriched by agents (decisions, discoveries, sparks, blockers). Added `loaf session log` subcommand and `--from-hook` flag. Absorbs `resume-session` and `reference-session` skills. Cross-agent protocol: any agent reads/writes the journal for shared context.
- 2026-03-31 — Verb/noun skill principle: skills ARE commands on all harnesses, so processes should be verb-skills (user-invocable) and knowledge should be noun-skills (reference). Rename `council-session` → `council` (user-invocable). Flip `debugging` to user-invocable. Evaluate `resume-session` + `reference-session` for merge. Added verb/noun classification audit to Phase 2 test conditions.
- 2026-03-31 — SKILL.md structural convention: researched GSD, gstack, and Superpowers harnesses for instruction adherence patterns. Added Phase 2 structural convention (Critical Rules → Verification → Quick Reference → Topics), instruction tiering by skill type (reference/cross-cutting/workflow), adherence techniques (anti-rationalization, bright-line rules, forbidden output patterns, success criteria checklists), and token budget targets per tier. Based on convergent finding: instructions that matter most need highest attention weight.
- 2026-03-31 — Hook reclassification + scope reduction: ~20 non-blocking language/infra/design hooks reclassified as skill instructions (moved to SKILL.md content). `loaf check` reduced from ~25 domain checks to 5-6 enforcement checks. Bun migration deferred (npm/tsup works). Amp simplified to skills-only (plugin API experimental). Codex hooks scoped to Bash-matching enforcement only. Net effect: Phase 3 dramatically smaller, Phase 4 simpler.
- 2026-03-31 — Direct command generation: replaced bash thin-wrapper approach with direct `loaf check` commands in generated `hooks.json`. Eliminates bash wrapper scripts and 4 shared shell libraries. All shell-hook harnesses accept raw commands — wrappers were unnecessary. Added Codex hook enforcement limitation note (Bash-only tool matching).
- 2026-03-31 — Cross-harness evaluation pass: researched all 6 harness docs + Agent Skills spec + AAIF context. Identified 12 gaps. Amendments: two-tier description strategy (250-char safe + full 1024), `failClosed` hook schema support, `RuntimePlatform.extraEvents` callback for OpenCode's 30+ events, invocability semantic mapping table, agent `tools` field format validation, Cursor prompt hook filter bug fix, compaction hook mapping across harnesses. Expanded deferred follow-on work (Cursor advanced hooks, skill routing validation, Codex feature flag handling, OpenCode extended events). Added cross-harness compatibility reference matrix.
- 2026-03-31 — Gap closure pass: moved Bun migration to Phase 3 (with perf comparison), specified `loaf session` subcommands in detail (consolidating 3+2+1 scripts), corrected side-effect hooks as staying hooks (task-board is active, not discontinued), dropped multi-hook batching (harnesses fire individually), identified CLI-wrapping hooks that stay as-is (no double-wrapping), added `loaf` binary distribution strategy and Cursor sandbox concern.
- 2026-03-31 — Deep review pass: fixed shared artifact model (intermediate, not final), reversed metadata migration (clean source + sidecars), recategorized hooks (5 types, not one), fixed exit code semantics (0/2 + 1=error), added `loaf session` subcommands, addressed `loaf` PATH resolution, added Cursor sandbox risk, unbounded appetite.
- 2026-03-31 — Reshaped from brainstorm session: reframed as cross-harness skills + hook consolidation. Absorbed skill simplification idea. Added Bun migration, `loaf check` CLI backend, CLI reference skill, agent profile audit.
- 2026-03-31 01:19 — Original draft: artifact matrix, implementation ordering, migration risks, deferred follow-on specs.
