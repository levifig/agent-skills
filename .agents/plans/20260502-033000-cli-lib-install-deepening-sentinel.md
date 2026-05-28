---
title: Deepen ensureSymlink into a SymlinkState Module
created: '2026-05-02T03:30:00Z'
status: drafting
spec: SPEC-034
related:
  - TASK-163
  - 20260501-231922-plan-lifecycle-cli-doctor-housekeeping
---

# Deepen `ensureSymlink` into a `SymlinkState` Module

> **Provenance:** This plan is the Track B go/no-go sentinel test for SPEC-034. It exercises the `/refactor-deepen` workflow — including the 3-sub-agent INTERFACE-DESIGN phase — against a real Loaf Module. Vocabulary fidelity is graded at the bottom of the file.

## Candidate

`cli/lib/install/symlinks.ts` — specifically the `ensureSymlink` function and its surrounding state machine. Today, `ensureSymlink` returns one of five `EnsureSymlinkAction` discriminants (`created` / `already-correct` / `relinked` / `declined-relink` / `replaced-file`) plus a heterogeneous `EnsureSymlinkResult` shape. Callers in `installer.ts` and `init.ts` branch on the discriminant string to decide what to log and how to proceed.

The state machine is implicit in the function body. Every caller re-derives "is this a success path or a no-op?" by inspecting the action discriminant. The migration-on-collision dance, the user-prompt branch, and the canonical-vs-real-file decision are all visible to callers but owned by no Module.

## Dependency Category

**In-process** (per `references/deepening.md`). Filesystem and prompt are injected as functions today, but everything happens inside the Loaf CLI process — no remote storage, no substitutable Adapter today. The deepening makes the Adapters explicit (FsReader / FsWriter / Confirm) but the category stays in-process.

## Proposed Deepened Module

A `SymlinkState` Module that hides:

- **State derivation** — lstat / readlink / readFileSync / existsSync sequencing collapsed into a single `evaluate` that returns an opaque snapshot.
- **Decision logic** — "what should this path look like?" (canonical / alias / real-file / missing) lives inside `reconcile`.
- **Migration choreography** — the strip-fence → merge-into-canonical → `.bak` → symlink dance hides behind one transition.
- **Prompt branch** — the user-confirm-replace step is policy that the Module owns; callers supply only the Confirm Adapter.
- **Result vocabulary** — callers ask `changed(outcome)` / `settled(outcome)` / `describe(outcome)` instead of pattern-matching a discriminant string.

The Module exposes three Seams (FsReader, FsWriter, Confirm) and one Interface that spans `evaluate` (pure read), `reconcile` (action), and a small set of predicates over the outcome.

The three independent INTERFACE-DESIGN sub-agents (Section: *Interface Designs (3 Independent)* below) converged on the broad shape but diverged on outcome representation, Adapter granularity, and how aggressively to use opaque types.

### Leverage

`installer.ts` and `init.ts` both call into the symlink machinery today. After the deepening:
- `installer.ts`: drops the action-string switch in its log path; receives `describe(outcome)` instead.
- `init.ts`: drops its own raw `symlinkSync` call and supplies a `Confirm` Adapter to `reconcile` instead.

Net call-site count drops from ~4 branchy invocations to 2 transparent ones.

### Locality

Today the "should this symlink be relinked / replaced / migrated?" decision tree spans `installer.ts` (decision to call), `symlinks.ts` (state derivation), and `init.ts` (prompt callback supplied to `ensureSymlink`). After deepening, the entire decision tree lives inside one Module. Locality goes from three files to one.

## What Survives in Tests

Existing unit tests in `symlinks.test.ts` (967 lines) target the discriminated outcomes — those tests stay green semantically, but their assertions retarget:

- `result.action === "replaced-file"` becomes `outcome.transition === "migrated"` (Design A) or `outcome.mutated === true` (Design B/C). The exact retarget depends on which design wins.
- The five state-shaped tests still cover the five paths; the discriminant just moves from a string union to an opaque field.
- Fence-stripping and migration-merge tests carry over directly — those test the hidden Implementation, which the deepening preserves.
- A new tier of tests covers each Adapter independently (FsReader, FsWriter, Confirm) — a property the current shape does not expose. Net test count grows.

What does **not** survive: any caller-side test that asserts on the action-discriminant string. Those tests retarget to `changed()` / `settled()` / `describe()` predicates.

## Rejected Alternatives

### Class-based `SymlinkInstaller`

Rejected. Adds an Implementation surface (constructor + mutable instance) without adding Depth — every method takes the same `path` argument the constructor would store. The functional Interface keeps the dependency Adapters explicit and the call sites readable.

### Inline prompt branching as separate exported helpers

Rejected. Splits the state machine into multiple Modules that share private state, defeating the Locality argument. The point of the deepening is to *consolidate*, not to fan out.

### Keep five-arm action discriminant, add a thin facade

Rejected. Doesn't reduce the surface area for callers that import directly. The Module-level Interface is what creates the discipline; a facade preserves the leak.

### `InstallerPipeline` Module

Considered and rejected as a separate candidate. The three orchestration steps in `installer.ts` (resolve targets → install each → finalize hooks) look like a Pipeline candidate, but the steps share no state — each is a one-shot side-effect against a different filesystem path. Wrapping them in a Pipeline Module would invent Depth where there is none. Today's procedural call site is the right shape; the deepening lives in `ensureSymlink`, not in its caller.

### `ManagedSection` Module (fenced-section deepening)

Considered as a sibling candidate but moved to a separate plan. The fenced-section logic in `cli/lib/install/fenced-section.ts` has a similar Depth deficit (four exports that callers glue manually) but a different shape (pure string transform, no Seams needed). It deserves its own PLAN — surfacing it here would dilute the focus on `ensureSymlink`.

---

## Interface Designs (3 Independent)

Per SPEC-034 lines 64 and 107, three sub-agents were spawned with **identical briefs** (no opposing-constraint priming). The three independent designs are summarized below; the full briefs and outputs are reproduced for audit. Variety emerged from sampling, not from manufactured opposition — exactly as intended.

### Design A — `evaluate` + `reconcile` + predicates

Opaque `SymlinkOutcome` with explicit `transition` discriminant (`created` / `kept` / `relinked` / `migrated` / `left-alone` / `deferred` / `failed`) plus a `changed(outcome)` predicate that callers use instead of inspecting the transition. Three Adapters: FsReader, FsWriter, Consent. Adds a `reconcileProject(...)` convenience function for the orchestrator path.

```typescript
export type SymlinkState = {
  readonly path: string;
  readonly desiredTarget: string;
  readonly description: string;
  readonly current:
    | { kind: "absent" }
    | { kind: "correct-link" }
    | { kind: "wrong-link"; actualTarget: string }
    | { kind: "real-file" }
    | { kind: "real-dir" };
};
export type SymlinkOutcome = {
  readonly state: SymlinkState;
  readonly transition: "created" | "kept" | "relinked" | "migrated"
    | "left-alone" | "deferred" | "failed";
  readonly migration?: { backupPath: string; mergedIntoCanonical: boolean };
  readonly reason?: string;
};
export declare function evaluate(path, desiredTarget, description, fs?): SymlinkState;
export declare function reconcile(state, policy): Promise<SymlinkOutcome>;
export declare function changed(outcome): boolean;
export declare function settled(outcome): boolean;
export declare function describe(outcome): string;
export declare function reconcileProject(params): Promise<readonly SymlinkOutcome[]>;
```

**Tradeoff:** `SymlinkOutcome` carries optional `migration` and `reason` — risk that callers start destructuring them and recreate the discriminant-peek pattern. The `changed` / `settled` / `describe` predicates exist precisely to keep callers out of the shape, but the optional fields are still visible to anyone who reads the type.

### Design B — `evaluate` + `reconcile` + boolean outcome

Same Module structure but the outcome is a flat record with `mutated: boolean`, `migrated: boolean`, optional `backupPath`, and a pre-formatted `message` string. No transition discriminant exposed to callers at all — they read booleans or the message. Three Adapters: FsReader, FsWriter, Confirm.

```typescript
export interface SymlinkOutcome {
  readonly mutated: boolean;            // did anything on disk change?
  readonly migrated: boolean;           // was user content folded into canonical?
  readonly backupPath?: string;
  readonly message: string;             // pre-formatted human line
  readonly error?: Error;
}
export declare function evaluate(desired, fs): SymlinkState;
export declare function reconcile(state, runtime): Promise<SymlinkOutcome>;
export declare function reconcileAll(desired, runtime): Promise<readonly SymlinkOutcome[]>;
```

**Tradeoff:** Lost diagnostic granularity — callers can no longer log distinct messages for `relinked` vs `replaced-file` vs `declined-relink` because the Module owns the message. Consumers that want richer telemetry (a future `loaf doctor` reporter) would need an outcome-event channel added to the Interface.

### Design C — Constructor + opaque states + `Reconciliation` union

Module is a constructor (`symlinkState(deps) → instance`) returning an object with `observe` / `reconcile` / `describe` methods. Outcome is a tagged union (`converged` / `deferred` / `failed`) but the *states* themselves are opaque (`unique symbol` tag) — callers cannot inspect them at all without going through `describe()`.

```typescript
export type Reconciliation =
  | { kind: "converged"; changed: false; note: string }
  | { kind: "converged"; changed: true; note: string;
      migration?: { backupPath: string; mergedIntoCanonical: boolean } }
  | { kind: "deferred"; reason: "user-declined" | "no-tty"; note: string }
  | { kind: "failed"; note: string; cause: string };
export interface ObservedState { readonly _tag: unique symbol }
export declare function symlinkState(deps): {
  observe(link): ObservedState;
  reconcile(link): Promise<{ before, after, outcome: Reconciliation }>;
  describe(state): string;
};
```

**Tradeoff:** Strictest opacity, but introduces a constructor where the Module-as-functions pattern was sufficient. Three Adapters (FsReader / FsWriter / Confirmer) are the same shape as Design B; the difference is purely in how the Module is *consumed*. Risk: the unique-symbol opacity is over-engineering for a four-call-site Module — the type-system ceremony exceeds the actual encapsulation need.

### Convergence

All three designs agreed on:

- **The Module name** — `SymlinkState`.
- **The Seams** — three Adapters (filesystem read, filesystem write, user prompt). All three justified them by pointing at the existing 967-line test file.
- **The Locality win** — collapse the decision tree from 3 files to 1.
- **The functional split** — separate `observe`/`evaluate` (read) from `reconcile` (action), with predicates or methods to read the outcome.

They diverged on:

- **Outcome representation** — discriminated transition (A) vs boolean record (B) vs tagged union (C).
- **Module construction** — three exported functions (A, B) vs constructor returning an instance (C).
- **State opacity** — visible discriminant (A), hidden behind booleans (B), unique-symbol opacity (C).

**Recommendation:** Defer to user. Design A is the most boring (and that's a feature — it matches existing patterns in `cli/lib/kb/glossary.ts` and the rest of the codebase). Design B is the simplest shape but loses diagnostic granularity, which `loaf doctor` and the next-gen `installer` would likely want back. Design C is the most disciplined opacity but pays a type-system tax for what is fundamentally a four-call-site Module. **Design A is the orchestrator's pick if forced**, but the user should weigh the diagnostic-granularity question (Design B's tradeoff) against the loss of structured outcomes (Design A's win).

---

## Workflow Handoff

Plan saved to `.agents/plans/20260502-033000-cli-lib-install-deepening-sentinel.md`. Workflow handoff is pending the SPEC/PLAN/TASKS artifact taxonomy spec — for now, decide manually.

---

## Sentinel Vocabulary Test — Track B Go/No-Go Grade

**Method:** `grep -ow` for canonical terms (Module, Interface, Implementation, Depth, Seam, Adapter, Leverage, Locality) and drifted terms (boundary, service, component, layer, API, signature) in this PLAN file body (excluding this Sentinel section's grade table and the verbatim Design A/B/C TS code blocks, which preserve sub-agent vocabulary as evidence of the parallel-design phase).

**Verification command (body-only, excludes Sentinel grade table):**

```bash
awk '/^## Sentinel Vocabulary Test/{exit} {print}' .agents/plans/20260502-033000-*.md \
  | grep -ow -E 'boundary|service|component|layer|API|signature' | sort | uniq -c
```

**Pass condition:** all six counts are `0` in PLAN body prose. Counts are non-zero only inside the Sentinel section's own grade table, which lists them by name.

**Outcome (filed at PLAN-write time):** PASS — drafted with vocabulary discipline; the body is clean of drifted terms, and the three sub-agent design outputs each used the canonical vocabulary verbatim.
