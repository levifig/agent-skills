---
title: "Auto-discover monorepo version files (detect-and-persist)"
captured: 2026-05-01T17:26:31Z
status: raw
tags: [release, monorepo, dx, cli]
related:
  - SPEC-031-release-flow-hardening
  - TASK-143-add-monorepo-version-file-discovery-agents-loaf-js
  - 20260330-194524-idea-version-aware-release.md
---

# Auto-discover monorepo version files (detect-and-persist)

## Nugget

On first `loaf release` (or via a new `loaf release init`), run a bounded subdirectory scan for known manifests (`package.json`, `pyproject.toml`, `Cargo.toml`) with a depth limit (~2) and an ignore list (`node_modules`, `.venv`, `target`, `dist`, `vendor`, `.agents`). Surface the candidates to the user for confirmation, then persist the selection to `.agents/loaf.json` `release.versionFiles`. Converts "two manual config edits per project" into "one confirmation prompt → forever solved."

## Problem/Opportunity

Today, `detectVersionFiles` (`cli/lib/release/version.ts:392`) only auto-detects at the repo root. Subdirectory manifests like `backend/pyproject.toml` fall through to the `configOverrides` path, which means a human (or an implementer agent) has to hand-edit `.agents/loaf.json` the first time they release. The release-prep implementer hit this twice on the same project — a strong signal the loop should self-heal.

## Initial Context

- **Precedence stays intact.** Existing two-tier resolution (CLI override → config → root auto-detect) already encodes "explicit beats implicit." Detect-and-persist slots in *before* the config tier on first run, then writes config so subsequent runs are deterministic.
- **Why not pure auto-detect?** Pulling a vendored fixture's `package.json` into a release bump would be worse than the current papercut. The author of `version.ts` left a comment to that effect ("partial monorepo bumps are worse than no bump at all"). Confirmation-and-persist preserves that invariant.
- **Workspace-aware detection** (`package.json#workspaces`, `[tool.uv.workspace]`, `Cargo.toml [workspace] members`) is a possible enhancement but adds one parser per ecosystem; bounded glob covers the common monorepo shapes without it.
- **Prior work:** SPEC-031 + TASK-143 shipped the declarative config that this idea would automate populating. Idea `20260330-194524-idea-version-aware-release` captured "configurable version files" but its scope was replacing hardcoded candidates with config — not discovery.
- **Open questions for shaping:** Should the prompt happen on every release until accepted, or only when no `release.versionFiles` exists? Should `loaf release init` be a separate command or just a `--init-config` flag? How loud should the warning be when discovery finds candidates the user *didn't* select (in case they add a new package later)?

---

*Captured via /loaf:idea — shape with /loaf:shape when ready*
