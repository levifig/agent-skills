# Journal Continuity

The project journal is the only session-related structure. Entries are
project-scoped events, each tagged with an opaque harness id that correlates a
single conversation's entries. Nobody opens, closes, or transitions anything —
concurrent conversations across branches, worktrees, and harnesses just
interleave rows with different tags, which is correct by construction.

## Contents

- Core Model
- Logging Protocol
- Codex Auto Mode
- Wrap: Optional Checkpoint
- Derived Continuity
- Recovery
- Hook Integration
- Anti-Patterns

## Core Model

1. Journaling is continuous. There is no start step and no "active session"
   precondition. A skill's first action is to log itself:
   `loaf journal log "skill(<name>): <purpose>"`.
2. `loaf journal log "type(scope): description"` appends a durable, project-scoped
   entry. The current branch and harness id are attached automatically.
3. `loaf journal recent` shows the timeline (newest first); `--branch` and
   `--since-last-wrap` narrow it, `--limit` and `--json` shape output.
4. `loaf journal search <query>` runs full-text search across the project journal.
5. `loaf journal show <entry-id>` reads one entry.

## Logging Protocol

Log compact, factual entries as work happens:

```bash
loaf journal log "decision(scope): chose X because Y"
loaf journal log "discover(scope): learned Z from file/path"
loaf journal log "block(scope): waiting on external approval"
loaf journal log "unblock(scope): approval received"
loaf journal log "spark(scope): possible follow-up idea"
loaf journal log "todo(scope): concrete follow-up action"
```

Log durable facts, not thoughts. Reference task IDs, spec IDs, report IDs, and
commit refs rather than pasting long prose. The journal should let another agent
resume without reading the whole conversation.

## Codex Auto Mode

When the user has explicitly enabled Loaf's managed Codex Auto-journal capability, use the exact path-pinned command shown in the Loaf-managed block of `CODEX_HOME/AGENTS.md`; its form is `'<canonical-loaf-path>' journal log --execpolicy-safe "decision(scope): chose X because Y"`. Do not substitute a bare `loaf`, alternate executable, or shell/environment wrapper. The installed Codex rule authorizes only explicitly classified basic Loaf command leaves outside the workspace sandbox, including this hardened journal writer and approved readers; ordinary `journal log`, body/file-consuming leaves, and path-taking `change check` remain operator-gated, and the policy does not authorize a general Loaf data-directory writable root. Other harness adapters are not implied and continue to use their own ordinary surfaces.

Enable the capability once with `loaf install --to codex --codex-basic-commands`. Installation is an explicit trust decision. If the rules are absent, stale, locally modified, or conflict with user-owned `loaf.rules`, Loaf reports the condition instead of overwriting it or asking for full system access.

## Wrap: Optional Checkpoint

A `wrap` entry is a voluntary checkpoint, not a lifecycle transition. Write one
only when the conversation holds synthesis worth saving — intentions, abandoned
paths, next steps — the connective narrative that evaporates with the context
window. Almost everything else is derivable from raw entries.

```bash
loaf journal log "wrap(scope): tried X, abandoned because Y, next is Z"
```

Nothing is ever "unwrapped." A conversation that ends without a wrap leaves a
perfectly valid journal. A wrap reviews its own conversation's entries first:

```bash
loaf journal recent --since-last-wrap
```

See the `wrap` skill for the full checkpoint flow.

## Derived Continuity

Continuity is computed at read time and never persisted. At conversation start
the SessionStart hook emits a layered digest: the latest project-level wrap,
recent entries scoped to the current branch/worktree, and open
(`in_progress`/`pending`) tasks. On demand, reproduce or extend it:

```bash
loaf journal context            # the layered continuity digest
loaf journal recent --branch <b> # recent entries for one branch
loaf journal search <query>     # find prior decisions by topic
```

Pass task/spec/report references to background and delegated agents. The harness
id is attached automatically — there is no session alias to pass along.

## Recovery

After compaction, a branch switch, or a long gap:

1. Read `loaf journal context` (or the digest the hook already emitted).
2. Widen with `loaf journal recent` / `loaf journal search` when more is needed.
3. Compare against `git status`, `git log`, and the relevant specs/tasks.
4. If code and journal have drifted, log the reconciliation:
   `loaf journal log "decision(recovery): rewound to <commit>; replaying tests"`.

## Hook Integration

- SessionStart emits the layered continuity digest; subtask agent invocations exit
  silently and write nothing.
- PreCompact nudges a journal flush of unrecorded decisions and next actions.
- Post-compaction resumption re-emits the digest. Digests are never persisted.

## Anti-Patterns

| Don't | Do Instead |
|-------|------------|
| Wait to log everything at the end | Log significant facts as they happen |
| Store decisions only in chat context | Log them and promote durable ones to ADR/spec/report/docs |
| Write a placeholder wrap out of ceremony | Wrap only when there's synthesis worth saving |
| Treat a missing wrap as an open loop | A conversation without a wrap is complete and valid |
| Pass a session alias to subtask agent | Nothing to pass — the harness id is automatic |
