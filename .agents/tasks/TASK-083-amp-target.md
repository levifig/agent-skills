---
id: TASK-083
title: Amp target
spec: SPEC-020
status: done
priority: P1
created: '2026-04-04T16:41:22.296Z'
updated: '2026-04-04T16:41:22.296Z'
completed_at: '2026-04-04T16:41:22.296Z'
---

# TASK-083: Amp target

New build target for Amp (experimental).

## Implementation

Created `cli/lib/build/targets/amp.ts`. Output:
```
dist/amp/
├── skills/           # from shared intermediate, no sidecar merge
│   └── {skill-name}/
│       └── SKILL.md
└── plugins/
    └── loaf.ts       # generated runtime plugin
```

**Skills:** Calls shared `copySkills()` — trivial copy from intermediate, no sidecar merge needed (Amp reads standard SKILL.md).

**Runtime plugin:** Calls shared `generateRuntimePlugin()` with Amp adapter:
- Experimental header: `// @i-know-the-amp-plugin-api-is-wip-and-very-experimental-right-now`
- Maps `tool.call` and `tool.result` events
- No agents (Amp has native agents)
- No command generation (Amp auto-discovers skills)

**Registration:** Added to `cli/commands/build.ts`, `config/targets.yaml`, `cli/lib/detect/tools.ts`, `cli/lib/install/installer.ts`.

## Verification

- [x] `loaf build --target amp` produces `dist/amp/skills/` + `dist/amp/plugins/loaf.ts`
- [x] Plugin includes experimental header
- [x] Plugin maps `tool.call`/`tool.result` events correctly
- [x] Skills copied from intermediate (no sidecar merge)
- [x] Registered in build, detect, install
- [x] `loaf build` (all targets) succeeds
