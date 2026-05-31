---
id: ADR-014
title: Go for Loaf stateful runtime
status: Accepted
date: 2026-05-28
---

# ADR-014: Go for Loaf Stateful Runtime

## Decision

Loaf's stateful runtime and SQLite-backed operational state will be implemented in Go.

The public `loaf` command should move toward a Go front controller: Go owns command dispatch and implements new stateful behavior directly, while unmigrated TypeScript commands are delegated through a compatibility bridge during migration. The bridge is transitional. It exists to avoid a big-bang rewrite, not to make TypeScript and Go equal long-term runtime peers.

SPEC-040's SQLite work starts in Go. The first implementation slice introduces the Go runtime foundation, SQLite lifecycle, project identity, and `loaf state` commands before any existing command family is rewritten.

## Context

Loaf currently ships a TypeScript CLI bundled by `tsup` and distributed through npm/plugin artifacts. That has worked for content transformation, target builds, installers, and filesystem checks, but SPEC-040 changes the center of gravity. SQLite-backed operational state turns Loaf from a content-oriented CLI into a durable local state engine.

That shift changes the runtime criteria:

- Dependency surface and supply-chain risk matter more because the state layer becomes trusted infrastructure.
- Distribution should be simple for hook execution, plugin bundles, and future non-Node hosts.
- SQLite integration needs a stable, testable, cross-platform story.
- The CLI will increasingly own protocol work, migrations, storage, and state transitions, not just generate files.

Node now has `node:sqlite`, but the API was still marked experimental in the Node 22 line and release-candidate in current documentation at the time of this decision. Third-party Node SQLite packages add native install and packaging concerns. Go does not include SQLite in the standard library, but it gives Loaf a smaller runtime shape: one compiled binary, strong standard-library coverage, first-party vulnerability tooling, and straightforward cross-platform distribution.

## Consequences

### Positive

- **Smaller runtime contract.** Users and hooks can eventually run one native `loaf` binary without requiring Node for stateful commands.
- **Lower dependency pressure.** Go's standard library covers most CLI, filesystem, HTTP, crypto, JSON, and process needs, reducing pressure to add packages for routine work.
- **Better supply-chain posture.** Go modules plus `govulncheck` give Loaf a first-party path for vulnerability checks against reachable code.
- **Cleaner SQLite foundation.** The SQLite driver becomes an explicit architectural dependency with measurable acceptance criteria rather than an incidental npm package choice.
- **Incremental migration.** A Go front controller lets new stateful commands land in Go while existing TypeScript commands keep working until migrated.

### Negative

- **Two-runtime transition.** During migration, the public command may need to ship a Go binary plus bundled TypeScript fallback assets.
- **Build and release complexity increases before it decreases.** CI, release artifacts, npm packaging, plugin bundles, and local development all need Go wiring while TypeScript remains present.
- **SQLite still requires a dependency decision.** Go's `database/sql` is standard, but SQLite requires a driver. Driver choice must weigh cgo, cross-compilation, dependency count, binary size, maintenance, and vulnerability surface.
- **Implementation capacity shifts.** Existing TypeScript command patterns cannot be mechanically copied; Go code needs idiomatic package boundaries, error handling, and tests.

### Neutral

- Existing TypeScript build and content-generation code remains valid until deliberately migrated or retired.
- This decision does not require rewriting all existing commands before SPEC-040 begins.
- This decision does not choose the SQLite driver. That choice belongs to SPEC-040 Track 0.

## Alternatives Considered

### Stay on TypeScript with `node:sqlite`

This would avoid introducing a second language and would keep the current build system simpler. It was rejected for the stateful runtime because the Node SQLite surface was not mature enough to make it the foundation for Loaf's operational store, and it keeps Loaf tied to Node for the commands most likely to run inside hooks and future runtime surfaces.

### Stay on TypeScript with a third-party SQLite package

This would use a more established SQLite package than `node:sqlite`, but it adds npm supply-chain exposure and likely native install or bundling complexity. It was rejected because SPEC-040 is specifically the moment where dependency/security posture should improve, not become more fragile.

### Rewrite the entire CLI in Go before SQLite

This would produce a clean single-runtime codebase, but it is too much unrelated change before proving the state model. It was rejected because the migration would be broad, risky, and slow to validate. SQLite Track A is the right proving slice.

### Use Rust for the stateful runtime

Rust offers strong safety and distribution properties, but it raises implementation complexity and does not match Loaf's operational need as directly as Go. Loaf needs a pragmatic, dependency-light CLI/runtime substrate more than maximum low-level control.

## Follow-on

- Amend SPEC-040 so Track 0 introduces the Go runtime foundation and front-controller migration strategy.
- Choose the SQLite driver in SPEC-040 Track 0 using explicit security, packaging, and cross-platform criteria.
- Update build/release workflows only when implementation begins; this ADR does not change current shipped artifacts by itself.

## Related

- [SPEC-040](../../.agents/specs/SPEC-040-sqlite-backed-loaf-operational-state.md) - SQLite-backed Loaf operational state
- [ADR-013](ADR-013-agentic-state-storage-model.md) - Agentic state is project-scoped, not branch-scoped
