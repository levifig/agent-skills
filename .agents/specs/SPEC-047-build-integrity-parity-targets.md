---
id: SPEC-047
title: "Build Integrity, Parity Contract & Target Simplification"
source: "/Users/levifig/Code/levifig/projects/loaf/.agents/drafts/20260621-020342-loaf-restructuring-roadmap.md (WS-A)"
created: 2026-06-22T09:13:21Z
status: drafting
branch: feat/build-integrity-parity-targets
source_sessions:
  - id: 20260621-001541-session
    role: shaped
---

# SPEC-047: Build Integrity, Parity Contract & Target Simplification

## Problem Statement

The Loaf build emits artifacts it never validates, ships at least one artifact
that is statically broken, and lets each target drift independently with no
contract binding them together. Concretely:

- **The build proves nothing about emitted JS/TS.** `build_test.go` installs a
  *fake* `node` (`setupFakeNodeForBuild`, used at `internal/cli/build_test.go:31`,
  asserted absent at `:56`) so the test never runs a real interpreter, and the
  Amp test asserts that the emitted file *contains TypeScript syntax inside a
  `.js` file* (`internal/cli/build_test.go:278-292`, e.g. line 283), enshrining
  the defect instead of catching it.
- **The Amp plugin is statically broken.** `generateNativeAmpPlugin` writes
  TypeScript (interfaces, type annotations, `Promise<HookResult>`) into
  `dist/amp/plugins/loaf.js` (`internal/cli/build_amp.go:103`) — invalid
  JavaScript. The `tool.call`/`tool.result` handlers take a param named `input`
  but read `call.toolName`/`call.arguments` (`internal/cli/build_amp.go:357-359,
  385-388`) — a reference to an undefined `call`, so every hook silently no-ops.
  The header still declares the API experimental
  (`@i-know-the-amp-plugin-api-is-wip-and-very-experimental-right-now`,
  `internal/cli/build_amp.go:114`).
- **Codex advisory hooks are coerced to enforcing.** The Codex hook parser
  defaults `failClosed: true` (`internal/cli/build_codex.go:629`) and reads
  `value != "false"` (`:646`), so any hook missing an explicit `failClosed: false`
  becomes a hard gate on Codex. The struct and emitted JSON
  (`nativeBuildCodexHook` at `:24-30`, `nativeCodexPreToolHookJSON` at `:46-53`)
  drop `blocking` and `if` entirely, so conditional and advisory semantics are
  lost on Codex.
- **OpenCode command coverage is keyed off the wrong signal.** Command-file
  generation skips any skill whose sidecar fields are empty
  (`internal/cli/build_opencode.go:94`), so a `user-invocable` skill with no
  OpenCode sidecar gets no command — silently unreachable.
- **Gemini is a maintained sixth target with no first-class standing.** It exists
  in `config/targets.yaml:38-42`, build wiring, `dist/gemini/`, install fencing
  (`internal/cli/install_fenced.go:23`), MCP install
  (`internal/cli/install_mcp.go:61`), and tests, but the program now scopes five
  first-class harnesses and drops Gemini.
- **No cross-harness language hygiene.** Claude-isms (subagent mechanism,
  interview tool, TODO tool, the agents file name) leak verbatim into every
  target; only command tokens (`{{IMPLEMENT_CMD}}`, `{{ORCHESTRATE_CMD}}`) are
  substituted today.
- **git-workflow documents the wrong commit format.** `git-workflow/SKILL.md`
  teaches `type(scope): description` (Verification + Quick Reference lines), but
  the native enforcement hook rejects scoped commits
  (`internal/cli/check.go:249`, "scoped commits not allowed").

The net effect: the build can ship an artifact it has not checked, and the
target matrix has no single invariant that fails when a new skill or hook breaks
parity.

## Strategic Alignment

- **Vision:** "The loaf CLI is the harness." A build that cannot validate its own
  output undermines every downstream guarantee. This spec makes the build prove
  its output and encodes harness parity as a testable invariant (roadmap §1).
- **Architecture:** Five first-class harnesses — Claude Code, Codex, Cursor,
  OpenCode, Amp — with **parity = equivalent capability via each harness's native
  idiom, NOT identical artifacts** (roadmap §1). Skills auto-load on Claude
  Code/Amp; generated commands on Cursor/Codex/OpenCode. Gemini is removed.
- **Supersedes / retires:** Removes Gemini as a build target and its install
  surface (`config/targets.yaml:38-42`, `install_fenced.go:23`,
  `install_mcp.go:61`).
- **Coordinates with SPEC-053 (WS-G):** The Gemini drop is a breaking change for
  already-installed users (orphaned `~/.gemini` skills). The *user-side cleanup*
  (orphan removal on upgrade) is hard-gated on SPEC-053's migration mechanism and
  must not ship to users before it exists. The in-repo removal (build/test/source)
  is non-breaking and ships in this spec.
- **Coordinates with SPEC-052 (WS-F):** This spec stabilizes the target matrix so
  the `~/.agents/` install relocation can build on a known-good set. SPEC-052
  depends on SPEC-053; this spec does NOT change install destinations.
- **Coordinates with SPEC-043/044/045 (WS-B):** Independent of WS-B; can ship
  first. The Amp TS plugin and the parity-matrix test established here are the
  hook surfaces any future SQLite-body Write-side enforcement hook (SPEC-043)
  must satisfy across all five harnesses.
- **Prior art — SPEC-040 (complete):** The `dist`/`plugins` drift gate
  (`git diff --exit-code -- dist plugins`) is the pattern this spec leans on:
  committed artifacts regenerate in-tree and the build fails on drift. The
  parity-matrix test extends that "build validates committed output" discipline
  to harness semantics.

## Solution Direction

Make the build validate everything it emits, fix the broken/incorrect target
output at the source, shrink the matrix from six targets to five, and bind the
five together with one parity-matrix test that fails the build on any gap.

1. **Real interpreter validation.** Remove the fake-`node` shim and the
   TypeScript-in-`.js` assertion. After a build, run `node --check` on every
   emitted JavaScript artifact and `tsc --noEmit` (or equivalent type-check) on
   every emitted TypeScript artifact (OpenCode `hooks.ts`, the new Amp TS
   plugin). Validation is part of the build/test gate; a syntactically invalid
   artifact fails the build. Treat a missing `node`/`tsc` toolchain as a clear
   skip-with-warning in local dev but a hard requirement in CI (decide the exact
   gating in Open Questions).

2. **Amp → first-class TypeScript plugin.** In `build_amp.go`, emit a valid
   TypeScript plugin at Amp's real plugin path (`.amp/plugins/` or
   `~/.config/amp/plugins/` — pin the path in Open Questions) using the stable
   event API (`session.start`, `tool.call`, `tool.result`). Fix the handler bug:
   the handler param is `input`; read tool name/arguments from `input`, not from
   the undefined `call` (`build_amp.go:357-359, 385-388`). Drop the
   `@i-know-the-amp-plugin-api-is-wip…` header (`build_amp.go:114`). Stop writing
   `loaf.js`; emit a `.ts` (or `.mts`) artifact the `tsc` step actually validates.
   Amp keeps full hook enforcement (advisory stays advisory, enforcement stays
   enforcing) and is first-class.

3. **Drop Gemini (in-repo, non-breaking).** Remove Gemini from
   `config/targets.yaml` (lines 38-42), all build wiring, `dist/gemini/`, install
   detection, `install_fenced.go:23`, `install_mcp.go` Gemini handling
   (`:61`), and every test reference (including the `dist/gemini/stale.txt`
   fixtures in `build_test.go:41,69`). The user-facing orphan-cleanup on upgrade
   is SPEC-053's responsibility.

4. **Codex advisory hook semantics.** In `build_codex.go`: default
   `failClosed: false` (change `:629`), parse `value == "true"` (change `:646`,
   matching the Amp parser at `build_amp.go:576`), and carry `blocking` and `if`
   through the struct and emitted JSON (`:24-30`, `:46-53`). Advisory Codex hooks
   stay advisory; conditional hooks keep their `if`.

5. **OpenCode command coverage off `user-invocable`.** Drive OpenCode command-file
   generation off the skill's `user-invocable` flag, not sidecar presence
   (`build_opencode.go:94`). Every `user-invocable` skill gets a command; pure
   reference skills (`user-invocable: false`) do not.

6. **Cross-target harness-language transform + content lint.** Extend the
   existing command-token substitution (which already resolves `{{IMPLEMENT_CMD}}`
   / `{{ORCHESTRATE_CMD}}` per target inside `transformMd`) with harness tokens —
   `{{HARNESS_NAME}}`, `{{INTERVIEW_TOOL}}`, `{{SUBAGENT_MECHANISM}}`,
   `{{TODO_TOOL}}`, `{{AGENTS_FILE}}` — resolved per target. Add a build-failing
   content lint over the four non-Claude first-class harnesses that fails on
   residual Claude-isms (literal "subagent", interview/TODO tool names, the
   Claude agents-file name, unresolved `{{…}}` tokens). The token vocabulary and
   per-target resolution table are part of this spec; tokenizing every source
   occurrence is implementation work guided by the lint.

7. **git-workflow commit-format fix.** Change `git-workflow/SKILL.md` from
   `type(scope): description` to unscoped `type: description` so the documented
   format matches the native enforcement hook (`check.go:249`). Rebuild the
   distributed copies.

8. **The parity contract as a single test.** Add one parity-matrix test
   enumerating, across all five first-class harnesses:
   - **Skill reachability:** every `user-invocable` workflow skill is reachable —
     auto-loaded (Claude Code, Amp) or a generated command (Cursor, Codex,
     OpenCode).
   - **Hook semantics preserved per surface:** every advisory hook stays advisory
     and every enforcement hook stays enforcing, via each harness's hook surface
     (Claude Code plugin, Codex `hooks.json`, Cursor `hooks.json`, OpenCode
     `hooks.ts`, Amp TS plugin).
   - **Zero cross-harness language leakage:** no Claude-isms in any non-Claude
     target output.
   A new skill or hook that breaks any cell fails the build.

## Scope

### In Scope
- Removing the fake `node` shim and the TypeScript-in-`.js` assertion from
  `build_test.go`; adding real `node --check` / `tsc --noEmit` validation over
  emitted JS/TS.
- Rewriting the Amp plugin generator (`build_amp.go`) to emit valid TypeScript at
  Amp's real plugin path using the stable event API, fixing the `input`/`call`
  handler bug, and dropping the WIP header.
- Removing Gemini from targets config, build wiring, `dist/gemini/`, install
  fencing/MCP, and tests (in-repo only).
- Codex advisory-hook fixes: `failClosed` default/parse, `blocking`/`if`
  carry-through.
- OpenCode command coverage driven by `user-invocable`.
- Harness-language token vocabulary + per-target `transformMd` resolution + a
  build-failing Claude-ism content lint over the four non-Claude harnesses.
- git-workflow commit-format correction (unscoped) + rebuilt distributed copies.
- The parity-matrix regression test plus the per-fix regression tests (JS/TS
  validity, Codex advisory `failClosed:false`, OpenCode coverage, sidecar-merge
  correctness, Claude-ism lint).

### Out of Scope
- Changing install *destinations* to `~/.agents/` (SPEC-052 / WS-F).
- Any user-side migration or orphan cleanup of installed Gemini/old-Amp artifacts
  (SPEC-053 / WS-G).
- SQLite bodies, search, or Write-side body enforcement hooks (SPEC-043 / WS-B).
- Session-model or status-vocabulary changes (SPEC-048 / SPEC-049).
- Skill content de-bloat or description rewrites (SPEC-050 / SPEC-051), beyond the
  single git-workflow commit-format line and mechanical token substitution.

### Rabbit Holes
- **Bundling/transpiling the Amp plugin.** Emit a `.ts` Amp loads directly via
  its plugin API; do not build a TS→JS pipeline, a bundler, or a Node toolchain
  shipped with Loaf. `tsc --noEmit` validates; it does not transpile for ship.
- **A general harness-abstraction DSL.** The token set is a small fixed
  vocabulary plus a per-target resolution table — not a templating language or a
  capability-negotiation framework.
- **Re-architecting the hooks pipeline.** Fix Codex/Amp semantics within the
  existing parser/emitter shape; do not redesign `hooks.yaml` or the hook model.
- **Chasing byte-identical artifacts across harnesses.** Parity is equivalent
  capability via native idiom, explicitly NOT identical files.

### No-Gos
- Keeping the fake `node` or any test that asserts an invalid artifact is
  "correct."
- Shipping the Gemini *user-side* removal (orphan cleanup) ahead of SPEC-053.
- Leaving the Amp plugin as `.js` containing TypeScript, or leaving the
  `call`/`input` handler bug in place.
- Making a `user-invocable` skill unreachable on any first-class harness.

## Risks

| Risk | Likelihood | Impact | Mitigation |
|------|------------|--------|------------|
| CI lacks `node`/`tsc`, so validation silently skips and re-rots | Med | High | Make the toolchain a hard CI requirement; fail (not skip) when absent in CI; document in `.claude/CLAUDE.md` "Before Committing" |
| Amp's real plugin path or stable event API differs from assumption | Med | High | Pin path + API shape in Open Questions before breakdown; verify against current Amp docs; the handler fix is independent of path choice |
| Gemini removal misses a reference and breaks the build | Low | Med | Grep-driven removal checklist; the parity-matrix test enumerates exactly five targets, catching a stray sixth |
| Codex `failClosed` flip relaxes a hook that *should* enforce | Low | High | Audit `hooks.yaml`: enforcement hooks must set `failClosed: true` explicitly; parity test asserts enforcement hooks stay enforcing on Codex |
| Token substitution corrupts legitimate prose containing `{{…}}` or "subagent" | Med | Med | Lint reports offending file:line; allowlist where a literal is intentional; only substitute the fixed token set |
| OpenCode coverage flip floods the menu with reference-skill commands | Low | Med | Gate strictly on `user-invocable: true`; reference skills set `user-invocable: false` |

## Open Questions

- [ ] Amp plugin path: `.amp/plugins/` (project) vs `~/.config/amp/plugins/`
      (global) — which does Amp load, and does install target one or both?
- [ ] Amp plugin file extension/module format the stable API expects (`.ts` vs
      `.mts`; default export shape) — confirm against current Amp plugin docs.
- [ ] `tsc --noEmit` invocation: project-wide `tsconfig` vs per-file
      `--strict false` check — what is the minimal type-check that proves
      loadability without false positives?
- [ ] Behavior when `node`/`tsc` is absent: hard-fail in CI, warn-and-skip
      locally — exact gating mechanism (env detection vs CI flag).
- [ ] Canonical Claude-ism token vocabulary and the per-target resolution table
      (what `{{SUBAGENT_MECHANISM}}` / `{{INTERVIEW_TOOL}}` / `{{TODO_TOOL}}` /
      `{{AGENTS_FILE}}` resolve to for Codex/Cursor/OpenCode/Amp).
- [ ] Does the parity-matrix test read live `hooks.yaml` + skill frontmatter and
      assert against built output, or assert a declared expectation table? (Prefer
      deriving from source so it cannot silently pass.)

## Test Conditions

- [ ] No fake `node` shim remains in `build_test.go`; the TypeScript-in-`.js`
      assertion at `build_test.go:283` is gone.
- [ ] A build with an intentionally malformed emitted `.js` fails `node --check`;
      a malformed emitted `.ts` fails the `tsc --noEmit` step.
- [ ] The Amp plugin is emitted as valid TypeScript at Amp's real plugin path,
      passes `tsc --noEmit`, and contains no `@i-know-the-amp-plugin-api-is-wip`
      header.
- [ ] The Amp `tool.call`/`tool.result` handlers read tool name/arguments from
      `input` (no reference to an undefined `call`); a test exercises a hook
      firing through the handler.
- [ ] `dist/gemini/` is not produced; no Gemini entry remains in
      `config/targets.yaml`, build wiring, `install_fenced.go`, `install_mcp.go`,
      or any test fixture; `loaf build` succeeds.
- [ ] A Codex hook with no explicit `failClosed` emits `failClosed: false`; a
      Codex hook with `if`/`blocking` carries those fields into `hooks.json`.
- [ ] Every `user-invocable` skill produces an OpenCode command file; a skill with
      `user-invocable: false` produces none.
- [ ] The content lint fails the build when a Claude-ism or an unresolved
      `{{…}}` token appears in any non-Claude first-class target output.
- [ ] `git-workflow/SKILL.md` documents unscoped `type: description`; the
      distributed copies are rebuilt to match; a sample commit message passing the
      doc passes `check.go` enforcement.
- [ ] The parity-matrix test enumerates exactly the five first-class harnesses and
      fails if any `user-invocable` skill is unreachable, any advisory hook
      becomes enforcing (or vice versa) on any surface, or any Claude-ism leaks.
- [ ] `loaf build`, `npm run typecheck`, and `npm run test` all pass; committed
      `dist/`/`plugins/` artifacts match regenerated output (`git diff
      --exit-code -- dist plugins`).

## Priority Order

Tracks ship in this order; if scope tightens, drop from the end. All tracks are
non-breaking in-repo except the Gemini user-side cleanup, which is deferred to
SPEC-053 entirely.

1. **Real interpreter validation** (non-breaking) — remove the fake `node` and
   the bad assertion; wire `node --check` / `tsc --noEmit`. Go/no-go: a malformed
   emitted artifact fails the build before continuing.
2. **Amp first-class TS plugin** (non-breaking in-repo) — valid TS at the real
   path, handler-bug fix, header dropped. Go/no-go: Amp plugin passes the
   validation from Track 1 and a handler test.
3. **Drop Gemini, in-repo** (non-breaking in-repo; user-side cleanup gated on
   SPEC-053) — remove from config/build/install/tests. Go/no-go: `loaf build`
   succeeds and no Gemini reference remains.
4. **Codex advisory hooks + OpenCode coverage** (non-breaking) — `failClosed`
   default/parse, `blocking`/`if` carry-through, command coverage off
   `user-invocable`. Go/no-go: Codex/OpenCode regression tests pass.
5. **Harness-language transform + Claude-ism lint** (non-breaking) — token
   vocabulary, per-target resolution, build-failing lint. Go/no-go: lint fails on
   a seeded Claude-ism and passes clean output.
6. **git-workflow commit-format fix** (non-breaking) — unscoped format + rebuilt
   copies. Can be dropped if scope tightens (it is a one-line doc/correctness fix).
7. **Parity-matrix test** (non-breaking; the durable guard) — single enumerating
   test over the five harnesses. Go/no-go: it passes on current output and fails
   on a seeded parity gap.
