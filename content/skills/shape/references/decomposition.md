# Decomposition

Shaping's tail, not a separate ceremony: dependency awareness, granularity judgment, and acceptance-criteria thinking, expressed as definition-of-done criteria and — only when a criterion earns its own DoD — child issues created by `loaf issue promote`.

## Contents
- When to split
- The sizing rule
- Expand–contract
- Promote, don't mint
- Coverage and containment
- Authoring criteria
- Order by likelihood-of-change
- Own the decisions

## When to split

A parent gets children only when its DoD needs more than one coherent slice. One criterion that is already right-sized stays on the parent. A checkbox becomes a sub-issue the moment it earns its own DoD — its own problem statement, its own out-of-scope, its own criteria.

Same problem, another slice → another criterion on this issue, or a promoted child if that slice now has its own DoD. A different problem discovered mid-shaping → a new backlog contract (`loaf issue new --ref <ref> --status backlog`), not a child of this one.

## The sizing rule

One test, replacing the old four-question checklist: **a slice is right-sized when it is verifiable alone and revertible alone.**

- **Verifiable alone** — the done-check can run (or be reviewed) without another slice landing first. If not, split and `loaf issue link <predecessor> blocks <successor>`, or merge — do not leave a criterion that is only true in combination.
- **Revertible alone** — the slice's landing can be reverted without undoing a sibling. If reverting would require coordinated rollback across slices, split or sequence with `blocks` edges.
- **Window-fit is implement's runtime heuristic** — if an executor slice still exceeds one fresh context window, implement sub-decomposes for execution via handoffs and scratchpad; that carving never appears in the issue tree. Shape sizes for verification and revertibility, not executor memory.
- If you are splitting just to have more rows, merge back.

Per-slice verification stays with the slice. Never a separate "verify" child; keep tests with the code they test.

## Expand–contract

The named exception for wide mechanical refactors (a rename, a schema migration, an expand-then-remove of an old path). The slice may be wide in files touched and still be one issue, because it is one coherent mechanical motion and one verification: the suite still passes after the motion.

Do not use this exception to smuggle a second problem into the parent. A mechanical rename plus a behavior change is two slices.

## Promote, don't mint

```bash
loaf issue promote <ref> <position>
```

The criterion at the 1-based position stays on the parent. A child **delivery** issue is created in `triage`, titled from the criterion text, with a copy of that criterion as its first DoD line and a claim already recorded from the copy to the parent. Coverage for that parent position holds by construction.

Then shape the child: give it a problem body and an out-of-scope statement (`loaf issue edit` replaces the body), add the criteria that make *its* DoD complete, promote again if one of those earns its own DoD.

`loaf issue promote` always mints a delivery child. Decision children are created separately:

```bash
loaf issue new --kind decision "Should the store be append-only?"
```

A decision child is ready when the title or body contains `?`. It does not claim a parent criterion.

Once **any** child contract exists, `loaf issue check` requires every parent criterion to be claimed. Sequence the tail as: write DoD → mint a child contract (`loaf issue new --ref <child-ref> --parent <parent-ref>`) for every slice that will not execute on the parent → then record ledger decisions (`loaf issue new --kind decision`, no `--ref`). If the parent stays a leaf, leave remaining unsharp questions in create-time `fog` (there is no `--fog` on edit) or file sharpened ones as sibling ledger decisions.

Manual claims, when a child criterion was added rather than promoted:

```bash
loaf issue dod add <child> "Child done-check" --serves 1
loaf issue dod claim <child> <child-position> <parent-position>
loaf issue dod unclaim <child> <child-position> <parent-position>
```

`--serves` claims the newly added child criterion against that parent position. `claim` / `unclaim` retarget an existing pair.

## Coverage and containment

`loaf issue check <ref>` runs these only when the issue has children.

- **Coverage** (failure) — every parent criterion must be claimed by at least one child criterion. An uncovered position is named in the failure list; fix it by promoting that criterion or adding a claiming child criterion.
- **Containment** (report, not a failure) — every child criterion must claim a parent criterion. An orphan is printed with a ready-to-paste remedy that files it as a **sibling** backlog issue:

  ```bash
  loaf issue new --ref '<child-ref>' --parent '<parent>' --status backlog -- '<orphan text>'
  ```

  Run the printed remedy. Do not fold the orphan back onto the parent, and do not treat it as in-scope work that somehow escaped the DoD — it is a new backlog row under the same parent.

A different problem (not an orphan criterion) is not a sibling of this decomposition. File it as a new backlog issue with no `--parent`.

## Authoring criteria

```bash
loaf issue dod add <ref> <text> [--command <cmd>] [--expect <expect>] [--tier V|H] [--serves <parent-position>]
```

- **V-tier** — used when `--command` is present, unless `--tier` overrides. Machine-checkable by `loaf issue verify <ref>`. Commands run from the **repository root**.
- **H-tier** — default when `--command` is absent. Human review; never gate input. `loaf issue verify` skips H-tier rows.

`--expect` is optional and enforced when present, with a deliberately minimal grammar: atoms join with ` and ` — `exit <N>` is the required exit code (an absent `--expect`, or one with no exit atom, means `exit 0`) and `` contains `text` `` requires the combined stdout+stderr to contain that backtick-delimited text (repeatable). Any other clause is unenforceable: verify warns naming the criterion and the clause and records it as advisory, so an expectation is either checked or loudly not — never quietly decorative.

```bash
loaf issue dod add linear:ENG-42 "Package tests pass" --command "go test ./..." --expect "exit 0"
loaf issue dod add linear:ENG-42 "Check names the uncovered criterion" --command "loaf issue check linear:ENG-42" --expect "exit 0 and contains \`uncovered\`"
loaf issue dod add linear:ENG-42 "The writeup is readable by someone new to the area" --tier H
```

A criterion whose check only restates the implementation (recomputing the expected value the way the code does) is vacuous — it can never disagree with the code under test. Prefer criteria with an independent source of truth.

`loaf issue dod list <ref>` prints the current lines. `loaf issue dod remove <ref> <position>` removes one (positions then compact).

## Order by likelihood-of-change

Present children for review, not for execution order: data models, interfaces, and user-facing flows lead; mechanical refactors and boilerplate collapse at the bottom. The reviewer's attention should land on what's most likely to need changing, not on whatever happens to run first.

Sequencing constraints that genuinely exist — this child must land before that one — are recorded as relationships:

```bash
loaf issue link <from> blocks <to>
loaf issue link <from> relates-to <to>
loaf issue link <from> remove blocks <to>
```

Never rely on `loaf issue tree` order to imply a dependency; a reviewer skimming by likelihood-of-change won't read sequencing into position. `loaf issue bucket <ref> now|next|later|none` is an advisory label only and is never read as a constraint.

## Own the decisions

Decide granularity and slice boundaries autonomously. Ask the user only when two orderings are genuinely equally valid with different trade-offs; otherwise decide and move on.
