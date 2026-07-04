# Worktree Storage Bootstrap

## Summary

Fresh linked worktrees can still strand agents before useful work starts. The current SPEC-036 gate correctly refuses commands when a worktree has local `.agents/` state without a valid `.moved-to` back-pointer, but normal branch/worktree creation still materializes tracked `.agents/**` files. That means a new agent may hit the refusal before it has enough context to run the sanctioned migration.

This change should make fresh-worktree bootstrap boring: the first mutating Loaf action should either work directly when the worktree only contains tracked historical `.agents` material, or present a single safe command path that an agent can execute without being trapped by the gate.

## Direction

- Keep `.agents` centralized to the main worktree per ADR-013.
- Keep the refusal for real, un-migrated worktree-local agentic state.
- Add a narrow bootstrap path for fresh linked worktrees whose local `.agents` content is only checkout material that already exists in the main worktree.
- Preserve dry-run/explicit-apply behavior for migration when content movement is required.
- Treat this as the first branch-local change capsule: implementation notes, tests, and acceptance criteria live here while cross-change observations stay in the project journal.

## Working Hypothesis

The current gate treats any worktree-local `.agents` file as pre-A3 state unless a valid `.moved-to` back-pointer exists. That was correct for real divergent worktree state, but too coarse for fresh worktrees where Git simply checked out historical `.agents/**` files that are byte-identical to the main worktree.

The fix should classify worktree-local `.agents` into three states before refusing:

| State | Meaning | Expected Behavior |
|-------|---------|-------------------|
| `none` | No local `.agents` content except maybe `.moved-to` | Do not refuse |
| `identical-checkout` | Every local `.agents` file is byte-identical to the main worktree counterpart; no extra local-only files | Bootstrap/recognize as safe |
| `divergent-local-state` | Missing counterpart, changed content, symlink, stale pointer, or partial migration leftovers | Refuse non-migrate commands |

The implementation can either write `.agents/.moved-to` during bootstrap or treat `identical-checkout` as non-refusing without writing. Prefer writing the back-pointer if it can be done safely and deterministically, because it makes the next command cheap and inspectable.

## Acceptance Criteria

- A fresh linked worktree whose local `.agents/**` files are byte-identical to the main worktree no longer dead-ends on the SPEC-036 refusal before useful work can begin.
- The first normal Loaf command in that fresh worktree either:
  - creates `.agents/.moved-to` pointing at the main worktree, or
  - proceeds through an explicit non-refusing `identical-checkout` classification documented in tests.
- A linked worktree with any local-only `.agents` file still refuses non-migrate commands with exit code `2` and the SPEC-036 nudge.
- A linked worktree with same relative path but different file content still refuses non-migrate commands with exit code `2`.
- A linked worktree with a stale `.agents/.moved-to` pointer still refuses.
- A linked worktree with symlinks under `.agents/` still refuses migration with the existing symlink message.
- `loaf migrate worktree-storage` remains dry-run by default and keeps current explicit `--apply` behavior.
- The fix does not weaken `validate-push`, hook advisory behavior, or other check surfaces.
- The change capsule remains branch-local and reviewable; cross-change continuity remains in the SQLite journal.

## Non-Goals

- Do not re-centralize `.agents` somewhere outside the main worktree.
- Do not remove the SPEC-036 safety gate for real un-migrated local state.
- Do not revive session entities or session lifecycle vocabulary.
- Do not solve the broader change-capsule CLI in this change.
- Do not sync Linear tasks by default; this change is local/Git/SQLite scoped.

## Test Strategy

Add focused regression tests before or alongside the implementation.

### Unit/Runner Coverage

Target: `internal/cli/cli_test.go`

- Add a helper that seeds identical `.agents` content in main and linked worktree.
- Add `TestRunnerAllowsFreshWorktreeWithIdenticalAgentsCheckout`:
  - create main repo;
  - seed `.agents/AGENTS.md` and at least one nested `.agents/specs/...` file on main;
  - create linked worktree;
  - seed the same files into linked `.agents`;
  - run a normal non-migrate command that currently gets gated, such as `journal recent` or `version`;
  - assert no exit code `2` and no SPEC-036 refusal.
- Add `TestRunnerRefusesLinkedWorktreeWithLocalOnlyAgentsFile`:
  - same setup, but add `.agents/local-only.md` in linked;
  - run `journal recent`;
  - assert silent exit code `2` and SPEC-036 nudge.
- Add `TestRunnerRefusesLinkedWorktreeWithDivergentAgentsFile`:
  - same relative file exists in both locations with different content;
  - run `journal recent`;
  - assert silent exit code `2` and SPEC-036 nudge.
- Keep current tests for:
  - main checkout no-op;
  - non-git rejection;
  - dry-run/apply migration;
  - stale pointer;
  - symlink refusal;
  - partial-leftover refusal.

### Public Binary Coverage

Target: `cmd/loaf/main_test.go`

- Extend `TestPublicBinaryPreA3WorktreeRefusalNudgeNatively` or add a sibling test:
  - fresh linked worktree with identical `.agents` checkout succeeds on `version` or `journal recent`;
  - divergent linked `.agents` still refuses.
- This guards the real built binary path, not only the in-process runner.

### Full Verification

Run and require green:

```bash
go test ./...
npm run test:smoke
npm run build
```

If the implementation changes generated help, hooks, or CLI reference output, rebuild and commit generated files with the source changes.

## Implementation Plan

### Phase 1: Classify Worktree `.agents` State

Add a small internal classifier near `detectPreA3StateNative` in `internal/cli/worktree_storage_migration.go`.

Suggested shape:

```go
type worktreeAgentsState string

const (
    worktreeAgentsNone worktreeAgentsState = "none"
    worktreeAgentsIdenticalCheckout worktreeAgentsState = "identical-checkout"
    worktreeAgentsDivergentLocalState worktreeAgentsState = "divergent-local-state"
)
```

Classifier rules:

- If local `.agents` is missing or contains no files except `.moved-to`: `none`.
- If `.moved-to` exists and points at the normalized main root: `none` unless local content remains.
- If `.moved-to` exists but points nowhere or not to main: `divergent-local-state`.
- If symlinks exist under linked `.agents`: `divergent-local-state`.
- Enumerate local agent files with `enumerateWorktreeAgentFiles`.
- For every local file, require the same relative path under main `.agents` and byte-identical content.
- If all files match: `identical-checkout`.
- Otherwise: `divergent-local-state`.

### Phase 2: Add Safe Bootstrap

Decide one of two implementations while coding:

- Preferred: when state is `identical-checkout`, write `.agents/.moved-to` with `mainRoot + "\n"` and allow the command.
- Fallback: when state is `identical-checkout`, allow the command without writing, and rely on tests/documentation.

Prefer the first if writing the pointer cannot mask divergent files. Do not delete identical checked-out files in the bootstrap path; deletion remains the explicit migration command's job.

### Phase 3: Wire Dispatcher Gate

Update `detectPreA3StateNative` so it refuses only `divergent-local-state`.

Preserve the allow-list in `shouldRefuseCommandNative`:

- `migrate`
- `help`
- `--help`
- `--version`

Do not broaden the allow-list as part of this change unless a test proves it is required.

### Phase 4: Preserve Migration Semantics

Review `runWorktreeStorageMigration` after classifier changes:

- `loaf migrate worktree-storage` in a fresh identical checkout should still be understandable.
- If the pointer already exists and no real content needs moving, output should be a no-op/already-migrated style message.
- Explicit `--apply` should still move divergent content according to current conflict policy when migration is genuinely needed.
- Existing dry-run/apply tests should pass unchanged unless the new classification requires a deliberate expected-output update.

### Phase 5: Docs and Capsule Update

Update this plan's notes only if implementation makes a meaningful product decision not already captured here.

Do not rewrite old SPEC-036 or ADR-013 unless the decision changes. This should be a refinement of bootstrap behavior, not a replacement for the storage model.

## Quality Gates

- No silent data loss: bootstrap may write `.moved-to`, but must not delete or move files.
- No weakening of divergent-state refusal.
- No branch/date naming drift: branch stays `fix/worktree-storage-bootstrap`; date remains only in `docs/changes/20260704-worktree-storage-bootstrap/`.
- No untracked temp files or worktrees left behind after verification.
- Project journal contains:
  - one `decision(...)` entry if the implementation chooses write-pointer vs classify-only;
  - one `discover(...)` entry for any failed attempt or surprising behavior;
  - one `commit(...)` entry after committing.

## Definition of Done

- Implementation committed on `fix/worktree-storage-bootstrap`.
- `docs/changes/20260704-worktree-storage-bootstrap/plan.md` reflects any final decision that differs from this plan.
- `go test ./...` passes.
- `npm run test:smoke` passes.
- `npm run build` passes.
- `git status --short` is clean except for intentionally ignored/user-confirmed artifacts.
- Remaining cleanup queue is updated:
  - PR #89 remote update/merge decision;
  - `feat/pi-first-harness` archive/delete decision;
  - stale stashes decision;
  - Claude handoff report delete/keep decision.

## Review Notes

- This is intentionally narrower than the future CR/change-capsule system. It uses the `docs/changes/YYYYMMDD-slug/` layout as a pilot artifact while the implementation remains normal code/tests.
- The observed dogfood failure was `npm run build` in `/tmp/loaf-journal-terminology-pass`: `bin/loaf __generate-cli-ref` was refused until `loaf migrate worktree-storage --apply` ran.
- The desired user experience is that a new worktree created for an agent starts usable, not that every safety check becomes advisory.

## Linear Mapping

Recommended Linear shape:

- Issue: `CR: loaf/worktree-storage-bootstrap`
- Tasks only if useful:
  - Add bootstrap classification for identical tracked `.agents` content.
  - Add fresh-worktree regression coverage.
  - Update help/docs if the command output changes.

Do not sync these tasks by default until the team needs visible parallel execution.

## Open Questions Before Execution

- Should bootstrap write `.agents/.moved-to`, or should it only classify identical checkout content as safe? Recommendation: write the pointer.
- Should `loaf migrate worktree-storage --apply` remove identical checked-out files after writing the pointer? Recommendation: preserve current explicit migration semantics; do not expand deletion behavior unless tests show stale content causes practical trouble.
- Should this change update ADR-013? Recommendation: no, because the project-scoped storage decision stands; this is an implementation refinement.
