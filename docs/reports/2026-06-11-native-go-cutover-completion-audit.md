# Native Go Cutover Completion Audit

Date: 2026-06-11 13:08
Branch: `feat/native-go-cutover-xdg-data`

This audit verifies the requested TypeScript-to-Go migration against current worktree evidence.

## Requirements

| Requirement | Verdict | Evidence |
|---|---|---|
| Create and work on a migration branch | Pass | Current branch is `feat/native-go-cutover-xdg-data`. |
| Remove the TypeScript CLI runtime path | Pass | `cli/` has zero `.ts`/`.tsx` files; `package.json` has no TypeScript/Vitest/Tsup/Commander/gray-matter dependencies; `TestNativeCutoverPackageAndSourceGuards` fails if stale TypeScript command sources, `dist-cli`, or obsolete TS config files return. |
| Keep public command dispatch native Go | Pass | `TestNativeCutoverCommandSurfaceAuditCoversPublicCommands` verifies every Go-dispatched public command has a Native Go row in the test map and that `cli/index.ts` remains absent. |
| Preserve launcher/package/plugin native-only boundaries | Pass | `TestLauncherRequiresNativeRuntimeEvenWhenLegacyBundleExists`, `TestLauncherRunsNativeRuntimeWithoutLegacyFallbackEnv`, and `TestNativeCutoverPackageAndSourceGuards` cover launcher behavior, package contents, plugin artifacts, and absence of fallback bundles. |
| Provide release packaging for native-only artifacts | Pass | `npm run build:release` builds and verifies native artifacts for `darwin-arm64`, `darwin-x64`, `linux-arm64`, `linux-x64`, `win32-arm64`, and `win32-x64`; `prepublishOnly` routes through `build:release`. |
| Use `XDG_DATA_HOME` for durable SQLite state | Pass | `PathResolver.DatabasePath` prefers `XDG_DATA_HOME`; tests cover data-home preference, state-home legacy path separation, invalid relative XDG values, and migration from legacy `XDG_STATE_HOME`. |
| Provide a solid DB migration path | Pass | `SchemaMigrations()` now has ordered migrations `0001_initial` and `0002_session_state_snapshots`; tests cover ordering, checksums, rollback, drift detection, future-sequence rejection, docs mirrors, and no secret-storage columns. |
| Preserve/migrate old state safely | Pass | Storage-home migration previews/copies legacy DBs without deleting them, refuses overwrites, and removes partial destinations on copy failure. Markdown migration dry-run/apply/resume paths preserve source markdown while importing state. |
| Add useful tests for each development slice | Pass | Current inventory: `./cmd/loaf` 12, `./internal/project` 4, `./internal/state` 145, `./internal/cli` 334. The map lists representative tests per migration surface. |
| Provide a test map | Pass | `docs/reports/2026-06-10-native-go-cutover-test-map.md` maps command surfaces, coverage areas, representative tests, verification gates, and ongoing guardrails. |
| Verify final state | Pass | Final gates passed: `npm run build:release`, `npm test`, `go vet ./...`, `npm run typecheck`, `npm run verify:go-artifacts`, `gofmt -l .`, `git diff --check`, and `find cli -type f \( -name '*.ts' -o -name '*.tsx' \) | sort`. |

## Caveat

The worktree contains a large unrelated `.agents/` deletion set that predates this audit. It was not reverted or modified as part of the migration work.
