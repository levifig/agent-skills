---
id: ADR-015
title: Portable launcher during Go port
status: Accepted
date: 2026-05-29
---

# ADR-015: Portable Launcher During Go Port

## Context

ADR-014 accepts Go as Loaf's stateful runtime and moves the public `loaf`
command toward a Go front controller. During the migration, Loaf still contains
two real command surfaces:

- New stateful commands implemented in Go.
- Existing compatibility commands implemented in the bundled TypeScript CLI.

Shipping the Go binary directly as `bin/loaf` made the package honest on
darwin/arm64, but it created two distribution failures:

- npm installs on Linux, Intel macOS, and Windows had to be blocked entirely,
  even though the TypeScript CLI still works there.
- The Claude Code plugin had no npm `os`/`cpu` guard, so non-darwin/arm64 users
  could receive an unexecutable native binary.

The eventual target is still a single Go CLI with per-platform native artifacts.
The current migration needs a smaller bridge that keeps legacy commands
portable without pretending every platform has a native Go runtime yet.

## Decision

The public `loaf` executable will be a portable Node launcher during the Go port.

The launcher is committed as `bin/loaf` and copied into the Claude Code plugin
as `plugins/loaf/bin/loaf`. Native Go binaries live below
`bin/native/<platform>-<arch>/` and the plugin mirrors the same layout.

At runtime the launcher:

1. Runs the matching native Go runtime when one exists for the current
   `process.platform` and `process.arch`.
2. Sets `LOAF_LEGACY_CLI` for the native runtime so delegated commands can find
   the bundled TypeScript fallback from package and plugin layouts.
3. Falls back to `dist-cli/index.js` for legacy TypeScript commands when no
   native runtime exists.
4. Refuses Go-only commands with an actionable native-runtime error when no
   matching native runtime exists.

The root npm package no longer declares `os` or `cpu` restrictions while this
launcher is in place. The package can install cross-platform again, but native
stateful commands only run where a matching `bin/native/<platform>-<arch>/loaf`
artifact is present.

## Consequences

### Positive

- npm installs are portable again for existing TypeScript-backed commands.
- Claude Code plugin hooks no longer execute an unguarded Mach-O binary on
  unsupported platforms.
- The transition keeps one public command path: `loaf`.
- The launcher preserves ADR-014's direction: TypeScript fallback remains a
  compatibility bridge, not a peer runtime for new stateful commands.

### Negative

- Node remains required as the public entrypoint until the Go port and
  per-platform native packaging are complete.
- Go-only commands are unavailable on platforms without a bundled native
  artifact.
- The package temporarily contains a launcher plus native binaries plus bundled
  TypeScript assets.

### Neutral

- Build verification still requires Go because native artifacts are committed
  and must remain reproducible from source.
- This decision does not define the final per-platform package matrix.

## Alternatives Considered

### Keep npm `os`/`cpu` guards

This made install failures honest, but it regressed every non-darwin/arm64 user
by blocking the TypeScript CLI even though it still works. It also did not guard
the Claude Code plugin channel. Rejected.

### Publish per-platform optional dependency packages now

This is likely the final distribution shape for the fully native CLI. It was
deferred because it adds package manifests, publish ordering, lockfile behavior,
and CI matrix work before the CLI port is complete.

### Build native binaries on postinstall

This would avoid committing platform binaries, but it requires Go on user
machines and makes install slower and less reliable. Rejected for user-facing
distribution.

### Finish the full Go port before fixing packaging

This would produce the cleanest final state, but it leaves the current dev line
with a broken plugin channel and non-portable npm package. Rejected.

## Follow-on

- When the CLI port to Go is complete, replace the launcher bridge with the
  final native packaging design.
- Add per-platform native artifacts or optional dependency packages before a
  non-dev release depends on the Go runtime everywhere.
- Remove bundled TypeScript fallback assets after the compatibility bridge is no
  longer needed.

## Related

- [ADR-014](ADR-014-go-for-stateful-runtime.md) - Go for Loaf Stateful Runtime
- [ADR-012](ADR-012-ci-verify-only-build-artifacts.md) - CI as Verifier, Not Fixer (Build Artifacts)
