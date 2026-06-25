---
id: TASK-254
title: Emit first-class Amp TypeScript plugin
spec: SPEC-047
status: in_progress
priority: P1
created: '2026-06-24T12:03:41Z'
updated: '2026-06-24T12:17:12Z'
completed_at: null
depends_on:
  - TASK-253
files:
  - internal/cli/build_amp.go
  - internal/cli/build_test.go
  - config/targets.yaml
  - dist/amp/
  - .agents/tasks/TASK-254-amp-first-class-typescript-plugin.md
verify: >-
  go test ./internal/cli -run 'TestRunnerBuildTargetAmp|NativeBuildAmp' -count=1
  && npm run build
done: >-
  Amp build output emits dist/amp/.amp/plugins/loaf.ts, uses the documented
  PluginAPI default-function shape, fixes tool.call/tool.result event handling,
  drops the WIP header, and passes the JS/TS validation gate.
---

# TASK-254: Emit first-class Amp TypeScript plugin

## Description

Rewrite the Amp target generator from a `.js` file containing TypeScript syntax
into a documented Amp project plugin at `dist/amp/.amp/plugins/loaf.ts`.

The generated plugin should use the current Amp plugin shape:
`import type { PluginAPI } from '@ampcode/plugin'` and
`export default function (amp: PluginAPI) { ... }`. Register event handlers via
`amp.on(...)`, and read tool details from the event parameter rather than the
undefined `call` identifier.

## Acceptance Criteria

- [x] `dist/amp/plugins/loaf.js` is no longer emitted.
- [x] `dist/amp/.amp/plugins/loaf.ts` is emitted.
- [x] The WIP/experimental API header is gone.
- [x] `tool.call` and `tool.result` handlers use the event parameter as source of
  truth and never reference an undefined `call`.
- [ ] Generated Amp TypeScript passes the validation gate from TASK-253 with
  dependency-approved `@ampcode/plugin` typing available.
- [ ] Tests exercise at least one hook firing path through the generated handler
  shape.

## Verification

```bash
go test ./internal/cli -run 'TestRunnerBuildTargetAmp|NativeBuildAmp' -count=1
npm run build
```
