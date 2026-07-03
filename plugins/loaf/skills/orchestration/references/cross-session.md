# Referencing Prior Decisions

Patterns for pulling decisions and context from earlier work into current work
without duplicating full context. Because the journal is one continuous
project-scoped log, "cross-session" reference is just querying the journal.

## Contents

- When to Reference Prior Work
- How to Find It
- What to Import
- When Decisions Conflict
- Anti-Patterns

## When to Reference Prior Work

### Good Reasons

- **Continuing related work**: building on previous feature decisions
- **Consistency**: aligning with prior architectural choices
- **Avoiding re-deliberation**: council decisions already made on similar topics
- **Context recovery**: picking up work after a long gap

### Skip Referencing When

- Starting genuinely new, unrelated work
- The decision already lives in an ADR (reference the ADR directly)
- Context would create more noise than value
- The decision was superseded or invalidated

## How to Find It

Search the journal by topic, then read the specific entries:

```bash
loaf journal search "token rotation"
loaf journal show <entry-id>
loaf journal recent --branch <related-branch>
```

Durable decisions should already be promoted to ADRs, spec changelogs, or
reports — prefer those canonical homes over re-deriving from raw entries. The
journal is the index; the artifact is the authority.

## What to Import

Import the distilled outcome, not the process. A one-line note in the current
conversation is usually enough:

```bash
loaf journal log "discover(context): reusing token-rotation decision from ADR-007 (15-min window)"
```

Import a broader context summary only when the problem space is complex, the
approach was non-obvious, or several related decisions form one strategy — and
even then, link the artifact rather than paste it.

## When Decisions Conflict

If a prior decision conflicts with the current approach:

1. Note the conflict and check whether the prior decision still applies.
   - Different context? Proceed with the new approach.
   - Same context? Understand why the original decision was made.
2. Log the change if deviating; convene a council if uncertainty remains.
3. Update the ADR if an architectural decision changes, marking the old one
   superseded.

## Anti-Patterns

| Don't | Do Instead |
|-------|------------|
| Re-derive decisions from raw entries | Read the ADR/spec/report the journal points to |
| Paste large prior context into the journal | Reference the artifact by stable ID |
| Reference without reading | Review imported content for current relevance |
| Import stale or superseded decisions | Verify the decision still holds |
| Over-import | Bring in only what the current work needs |
