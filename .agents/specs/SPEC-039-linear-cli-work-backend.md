---
id: SPEC-039
title: Linear CLI work backend and OAuth
source: direct
created: '2026-05-22T10:17:33Z'
status: drafting
related_specs:
  - SPEC-037
  - SPEC-038
---

# SPEC-039: Linear CLI work backend and OAuth

## Problem Statement

Loaf's current Linear-native workflow depends too much on skill prose and model-mediated MCP calls. That makes important invariants fragile:

- Linear mirror issues can expose Loaf internal IDs such as `SPEC-*`.
- Relationships can be duplicated in issue bodies instead of using Linear's native parent/blocking/project/status relationships.
- Agents must remember how to create parent and child issues correctly.
- Authentication depends on whatever token surface is available to the harness.
- Moving between local and Linear-backed work is not a mechanical transition.

Loaf needs a deterministic CLI-owned backend for Linear work management, with OAuth handled by the CLI and internal/external identity mapping stored in Loaf state.

## Strategic Alignment

- **Vision:** Loaf should make agentic workflows reliable by moving mechanical protocol work into the CLI.
- **Personas:** Solo developers can start local and later connect Linear. Team leads can enforce consistent Linear structure without depending on agent memory.
- **Architecture:** CLI is the protocol layer; hooks validate boundaries; skills route to CLI commands; MCP is optional inspection.

## Solution Direction

Introduce a backend-neutral work ledger and a Linear tracker adapter.

The initial ledger may live in the current Loaf local state model so this work
can ship incrementally, but that storage is a compatibility layer, not the
target architecture. The intended durable home is an XDG-backed SQLite store
(`$XDG_DATA_HOME` or `$XDG_STATE_HOME`, exact split to be decided in the future
SQLite spec) managed through Loaf CLI/TUI/GUI surfaces.

The long-term storage boundary is:

- **Repo:** source code, durable docs, committed knowledge, and curated public
  artifacts when appropriate.
- **XDG-backed Loaf state:** specs, tasks, sessions, mappings, hook events,
  backend sync state, provenance graph, and triage resolution graph.
- **Generated exports:** Markdown reports or spec snapshots only when useful for
  review, handoff, or archival.
- **External systems:** semantic outcome language and external IDs only.

Internal Loaf state keeps the private mapping:

```yaml
spec: SPEC-123
title: Harden release changelog preservation
backend: linear
external_refs:
  linear:
    mirror_issue: ENG-456
    child_issues:
      - ENG-457
      - ENG-458
```

Linear sees only Linear-native identity and outcome language:

```text
ENG-456 Harden release changelog preservation
ENG-457 Preserve curated changelog entries
ENG-458 Validate release-only PR changelog stubs
```

Linear child issues relate to the mirror issue through Linear's native `parentId`. Blocking, project, label, assignee, priority, and status relationships use Linear's own fields. Issue bodies must not duplicate relationships that Linear already models.

## CLI Ownership

Loaf CLI owns:

- Linear OAuth login, refresh, status, and logout.
- Token storage outside the repo.
- Linear GraphQL requests.
- Spec mirror issue creation/update.
- Child task issue creation/update.
- Parent/child/blocking/project/status relationships through Linear native fields.
- Mapping between internal `SPEC-*` and external Linear IDs.
- Backend transition from local to Linear and back where feasible.

Skills should call CLI commands. They should not directly orchestrate Linear MCP calls for core workflow.

## Proposed Command Surface

```bash
loaf linear auth login
loaf linear auth status
loaf linear auth refresh
loaf linear auth logout
loaf linear configure

loaf spec sync SPEC-123 --backend linear
loaf breakdown SPEC-123
loaf work status SPEC-123
loaf task sync --backend linear
```

Exact names can change during implementation, but the command model should keep auth, configuration, sync, breakdown, and status mechanically testable.

## OAuth Direction

Use Linear OAuth2 with PKCE and a localhost callback:

1. CLI starts a temporary local callback server.
2. CLI opens the Linear authorize URL.
3. Authorization request includes `client_id`, `redirect_uri`, `response_type=code`, scopes, `state`, and PKCE challenge.
4. Browser redirects to the local callback with `code`.
5. CLI exchanges the code for access and refresh tokens.
6. CLI stores tokens outside the repo, preferably OS keychain.
7. CLI refreshes tokens mechanically when expired.

Loaf should not try to create the Linear OAuth application automatically. A Linear admin provides the OAuth application configuration.

## Relationship To Other Specs

- **SPEC-037** defines the internal spec artifact that this backend maps to Linear.
- **SPEC-038** enforces that Linear-visible payloads do not leak Loaf internal IDs or `.agents/` paths.

This spec should produce payloads that pass SPEC-038 validators by construction.

## Scope

### In Scope

- Define backend-neutral work ledger fields needed for spec-to-external mappings.
- Mark the interim local ledger as a compatibility layer designed to migrate to
  XDG-backed SQLite later.
- Add Linear OAuth CLI flow with PKCE and localhost callback.
- Store Linear tokens outside the repository.
- Add Linear GraphQL client/adaptor in the CLI.
- Create/update Linear mirror issues for specs using outcome titles.
- Create/update Linear child issues for tasks using native relationships.
- Starting a Linear child issue also promotes its parent mirror issue from
  `backlog`/`unstarted` to the team's `started`/In Progress state.
- Use Linear native relationships instead of duplicating relationships in issue bodies.
- Update skills to route Linear workflow through CLI commands.
- Preserve local mode as a first-class backend.
- Allow transition from local mode to Linear mode by creating external Linear issues from existing internal spec/task state.

### Out of Scope

- Building the full SQLite backend. This spec should define a ledger shape that
  SQLite can later own under `$XDG_DATA_HOME` or `$XDG_STATE_HOME`.
- Adding GitHub Issues as a backend.
- Replacing all local task storage immediately.
- Hook enforcement for leaked artifacts. That belongs to SPEC-038.
- Creating Linear OAuth applications automatically.
- Depending on Linear MCP for core workflow execution.

### Rabbit Holes

- Full two-way sync. Resist initially: define explicit sync/status commands before background reconciliation.
- Mirroring every Linear field locally. Resist: store only mappings and fields Loaf needs to operate.
- Putting spec summaries into Linear bodies. Resist if the summary would become a stale duplicate of the internal spec. Prefer concise outcome/context only.
- Inventing custom relationship text in issue bodies. Resist: use Linear fields.

### No-Gos

- No `SPEC-*`, `TASK-*`, `.agents/...`, tracks, or phases in Linear titles/descriptions generated by Loaf.
- No duplicated parent/child/blocking relationship prose in Linear issue bodies when Linear has native fields.
- No repository-stored Linear access or refresh tokens.
- No mandatory MCP dependency for creating or syncing Linear work.

## Risks

| Risk | Likelihood | Impact | Mitigation |
|------|------------|--------|------------|
| OAuth setup is too heavy for solo users | Medium | Medium | Keep personal API token support as a possible development fallback only if explicitly chosen |
| Token storage differs across platforms | Medium | High | Use a keychain abstraction or clear platform-specific fallback policy |
| GraphQL schema drift breaks CLI | Medium | High | Keep queries small, typed where possible, and covered by integration-friendly tests |
| Local-to-Linear transition loses task relationships | Medium | High | Store dependency and parent mappings before creating external issues |
| Linear issue bodies become stale mirrors | Medium | Medium | Keep bodies minimal; use native fields and Loaf ledger for mappings |

## Open Questions

- [ ] Which OAuth scopes are minimal for mirror issues, child issues, status updates, comments, and relationships?
- [ ] Should Loaf support personal API keys as a local-only fallback, or require OAuth for all Linear use?
- [ ] Which keychain library or OS command should Loaf use without adding unacceptable dependency weight?
- [ ] What is the exact local ledger storage format before SQLite exists?
- [ ] Which future state belongs in `$XDG_DATA_HOME` versus `$XDG_STATE_HOME`?
- [ ] What export formats should SQLite-backed state generate for Git review,
      handoff, or archival?
- [ ] Should `loaf breakdown SPEC-123` automatically sync to Linear when `backend: linear`, or require an explicit `loaf spec sync` first?
- [ ] How should Linear-to-local transition handle closed or archived Linear issues?

## Test Conditions

- [ ] `loaf linear auth login` completes OAuth PKCE flow through localhost callback.
- [ ] Tokens are stored outside the repository.
- [ ] `loaf linear auth status` reports connected workspace/user without printing secrets.
- [ ] Expired access tokens refresh mechanically.
- [ ] `loaf spec sync SPEC-XXX --backend linear` creates a Linear mirror issue with no internal Loaf IDs or `.agents/` paths.
- [ ] Child tasks are created with native `parentId` relationship to the mirror issue.
- [ ] Starting a child issue moves the child to the team's `started` state and
      promotes a `backlog`/`unstarted` parent to `started`; already-active
      parents are left unchanged.
- [ ] Starting a child issue fails before moving the child when the parent is
      `completed`, `canceled`, or archived unless the caller explicitly
      overrides the protected parent state.
- [ ] If child start succeeds but parent promotion fails, the command exits
      non-zero and reports a reconciliation error with the parent issue ID.
- [ ] Blocking relationships use Linear native fields where available.
- [ ] Linear issue bodies do not duplicate parent/child/blocking relationships already represented by fields.
- [ ] Loaf local state records `SPEC-*` to Linear ID mappings.
- [ ] Ledger documentation states that the pre-SQLite local storage is interim
      and migration-oriented, not the final architecture.
- [ ] A local spec/task set can transition to Linear mode without changing public artifact language.
- [ ] Core workflow does not require Linear MCP tools.

## Priority Order

1. **Track A - Ledger and command contract.** Define local mapping shape, CLI command semantics, and the future XDG-backed SQLite storage boundary. Go/no-go: local tests prove internal spec IDs map to external IDs without exposing them, and docs state that current local storage is interim.
2. **Track B - OAuth and GraphQL adapter.** Implement auth, token storage, refresh, and minimal GraphQL client. Go/no-go: auth status and a read-only viewer/team query work without secrets in repo output.
3. **Track C - Linear mirror/task sync.** Create mirror and child issues using native relationships and outcome language. Go/no-go: generated Linear payloads pass SPEC-038 leak validators.
4. **Track D - Skill routing update.** Update breakdown/implement guidance to call CLI commands instead of MCP for Linear workflow. Go/no-go: no core workflow instruction requires Linear MCP.
