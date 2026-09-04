---
name: migrate-project
description: >-
  Migrates open local Loaf issues into a harness-connected external tracker
  from a provider-neutral export packet. Use when moving a project to Linear,
  GitHub, GitLab, Gitea, Jira, or another tracker supported by a provider module.
  Produces a verified source-to-destination
  mapping receipt. Not for ongoing synchronization, provider authentication,
  private continuity transfer, or remote Loaf setup.
---

# Migrate Project

Move open tracker-owned work once, then let the destination tracker remain canonical. Loaf serializes the source packet; the harness owns the tracker connection and every provider operation.

## Contents
- Critical Rules
- Verification
- Quick Reference
- Workflow
- Mapping Policy
- Receipt

**Input:** $ARGUMENTS

---

## Critical Rules

1. **Log invocation first** — `loaf journal log "skill(migrate-project): exporting open local issues to <provider and destination>"` before any other action.
2. **Export through the public boundary** — run `loaf migrate tracker-export --json`. Do not read the SQLite database directly or reconstruct issues from private continuity artifacts.
3. **The harness owns the connection** — use only the current harness's native, already-connected tracker tools. Loaf never receives provider credentials, calls provider APIs, installs MCP servers, or proxies tracker traffic. If the connection is unavailable, stop before writes and tell the operator which harness connection is missing.
4. **Treat the project label as a hint** — `project.label_hint` suggests source display text only. Confirm the exact destination name and casing with the operator and provider reads. Do not derive it from legacy config or silently normalize it.
5. **Plan every root's destination kind** — for each packet issue with no `parent_id`, explicitly choose and confirm `issue` or one provider-native project-like kind such as `project`, `milestone`, or `epic`. Never assume every source row becomes an issue.
6. **Read before write** — resolve and re-read the exact destination workspace, organization, team, repository, and existing project-like resources before proposing changes. Never infer a destination from a similarly named resource.
7. **Detect collisions by resource kind before writes** — search the correct provider collection for every source marker and likely title match. Classify each planned resource as `create`, `adopt`, `no-op`, or `conflict`; a project-like root must not adopt an issue or vice versa. Unresolved conflicts block that resource.
8. **Confirm the mapping plan** — show the destination, per-root destination kinds, status mapping, hierarchy strategy, relationship strategy, collision classifications, and first bounded batch. Obtain confirmation before the first mutation unless the operator has already approved that exact plan.
9. **Bound all resource mutations** — create or adopt at most 10 provider resources per batch, counting projects, milestones, epics, and issues together. Stop the batch on an ambiguous response or failed verification. Do not retry a create until a read proves whether the first attempt landed.
10. **Preserve provenance without auto-linking** — every created or adopted provider resource description must contain `Loaf source alias:` followed by the alias in Markdown inline code, for example `Loaf source alias:` then `` `DOJO-1` ``. When an alias is absent, use `Loaf source ID:` followed by the ID in inline code. Never replace provider-native IDs with Loaf aliases.
11. **Map by identity, never order** — associate every provider response with its source ID/alias and destination kind, then re-read the provenance marker. Never infer mappings from provider key sequence, request order, response order, creation time, or batch position; concurrent creates may receive provider keys in a different order.
12. **Verify every batch** — re-read each destination resource, its kind, and its hierarchy/status/relationship state after writing. Only verified mappings enter the receipt.
13. **One-time migration only** — do not create dual sync, webhooks, local mirrors, provider clients, credentials, journal/wrap/handoff migration, or remote continuity sync. Local harness execution needs no Loaf server.

## Verification

- The packet has `contract_version: 1` and `export_kind: tracker-migration`
- The packet contains only `project`, `issues`, and issue-to-issue `relationships`; no database/filesystem metadata fields, session data, or private continuity records were introduced
- `project.label_hint` was treated as a hint; exact provider display name and casing were confirmed without legacy config
- Every source root has one confirmed destination kind (`issue` or a provider-native project-like kind)
- Destination workspace/team/repository and applicable project-like resources were resolved through harness-native reads before writes
- Every source row has one explicit resource-kind-aware collision classification and one destination outcome
- Hierarchy/status/relationship mappings were shown and confirmed when the provider differs from the source model
- Every resource mutation was performed in a batch of at most 10 and re-read from the provider
- Every created or adopted resource description contains the source alias or fallback source ID marker as Markdown inline code
- Every mapping was verified by source identity, destination kind, and destination re-read, never by provider identifier or response order
- Children of a project-like root were verified as associated with that exact project resource, not parented to an invented root issue
- The final response uses [templates/tracker-migration-receipt.md](templates/tracker-migration-receipt.md) and distinguishes verified mappings, conflicts, failures, and untouched source rows
- No provider credential entered Loaf and no Loaf server, Orb, or remote sync was required

## Quick Reference

```text
loaf migrate tracker-export --json
```

| Concern | Owner |
|---------|-------|
| Source filtering and serialization | Loaf CLI |
| Tracker authentication and connection | Harness/operator |
| Workspace/team/project discovery | Harness-native tracker tools |
| Per-root resource planning, collision detection, and destination writes | This workflow through harness-native tools |
| Ongoing issue state | Destination tracker |
| Journal, wraps, handoffs, and other private continuity | Loaf; not part of this migration |

| Harness | First acceptance path | Remote requirement |
|---------|-----------------------|--------------------|
| Amp | `amp --no-tui` on the local project | No Loaf server or Orb |

## Workflow

### 1. Export and validate

Run the export once and retain its exact bytes for this migration attempt. Check the contract and count issues and relationships. Reject unknown contract versions rather than guessing. Treat `project.label_hint` as source context only; it does not select or name a provider destination.

The packet is a migration input, not authorization to write. It contains open, non-archived issues only. Completed and archived history remains in the untouched source.

### 2. Resolve the destination

Using the harness's native tracker connection:

1. Read the current workspace/organization.
2. List or search teams, repositories, and provider-native project-like resources.
3. Resolve the exact destination IDs and re-read them.
4. Confirm exact destination display names and casing with the operator.
5. If more than one destination fits, ask the operator to choose.

Do not ask for a Linear, GitHub, GitLab, Gitea, Jira, or other provider API key for Loaf. If the harness lacks the required connection, pause here.

### 3. Build the resource plan

First enumerate packet roots—issues whose `parent_id` is absent. For each root, propose one destination kind:

| Destination kind | Use when | Root representation |
|------------------|----------|---------------------|
| `issue` | The root is itself executable tracker work | One destination issue; child issues may use native issue hierarchy |
| `project`, `milestone`, or `epic` | The root represents a delivery container and its descendants are executable work | One provider project-like resource; do not create a duplicate issue for the root |

Recommend a project-like destination when a root describes the overall project outcome and its children carry the executable work, but require operator confirmation. Preserve the root title, body, DoD, kind, status context, and inline-code source marker in the closest supported fields of that resource.

For a project-like root:

1. Search the provider's project/milestone/epic collection—not its issue collection—for marker and title collisions.
2. Classify and create/adopt/no-op the root resource under the same bounded-batch and identity rules as issues.
3. Re-read and verify the resource kind, native ID, display name/casing, description marker, and destination container.
4. Only then create/adopt descendants as issues and associate direct children with that verified resource using provider-native project membership.
5. Do not assign a direct child an issue parent for the project-like root. Deeper descendants may use native issue hierarchy under their nearest ancestor that actually mapped to an issue.

A source tree planned as one project-like root plus N descendant issues must yield exactly one project-like mapping and N issue mappings—not N+1 issues. Because every resource kind counts toward the limit of ten, the minimum mutation-batch count is `ceil((N + 1) / 10)`.

Then classify every planned resource. Search by the exact inline-code source marker first, then by source alias, then by likely title. Classify:

| Class | Meaning | Action |
|-------|---------|--------|
| `create` | No plausible destination resource of the planned kind | Create after confirmation |
| `adopt` | One destination resource of the planned kind clearly represents the source but lacks the marker | Add provenance and apply confirmed mappings |
| `no-op` | A verified destination resource of the planned kind already has the source marker and desired state | Record without mutation |
| `conflict` | Multiple or contradictory matches | Stop for operator choice |

Propose a provider status mapping for `triage`, `backlog`, `todo`, and `active`. Preserve distinctions with provider-native states or labels when practical; otherwise disclose the collapse.

Use native issue parent/child structure when both source endpoints map to issues. When a root maps to a project-like resource, represent its direct-child edges through verified project membership instead. For other unsupported hierarchy or `blocks`, `blocked_by`, and `relates_to` edges, propose one explicit fallback—provider-native links, a source reference in the description, or a dedicated label—and confirm it before writing.

### 4. Apply bounded batches

Process project-like roots and issue parents before their children. Within each batch:

1. Re-read any adoption target immediately before mutation.
2. Create/adopt no more than 10 total provider resources.
3. Add the inline-code source marker to each resource description without discarding existing provider content.
4. Bind each returned provider ID and kind to the source ID/alias from that specific operation; never pair results by order.
5. Re-read every resource by provider identity, verify its kind and source marker, and record its provider ID/URL.
6. Associate child issues with a verified project-like root through provider-native membership; use issue parentage only when the parent mapped to an issue.
7. Apply other hierarchy and relationships only after both endpoint mappings are verified.
8. Re-read project membership, hierarchy, and relationship state.

If a provider call times out or returns an uncertain result, search/read before retrying. Never blind-retry a create.

### 5. Return the receipt

Use [templates/tracker-migration-receipt.md](templates/tracker-migration-receipt.md). Return the receipt in the response; persist it only when the operator asks. State explicitly that local issue rows were read-only and remain available as migration history. The required `skill(migrate-project)` journal entry is continuity, not tracker migration.

## Mapping Policy

- Preserve source title and body unless the destination requires a documented structural adaptation.
- Render definition-of-done items as unchecked destination checkboxes or the closest provider-native acceptance-criteria form.
- Preserve issue `kind` with a provider-native type or label when available; disclose any collapse.
- Do not migrate local verification commands. They can contain machine-specific details and are outside the provider-neutral packet.
- Do not mark destination issues complete: the packet contains only open work.
- Do not delete, archive, retitle, or change authority on local issue rows.

## Receipt

Use [templates/tracker-migration-receipt.md](templates/tracker-migration-receipt.md) for the operator-facing result and source-to-destination mapping.
