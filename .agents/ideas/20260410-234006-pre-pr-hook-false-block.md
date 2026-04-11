---
title: workflow-pre-pr hook false-blocks after release flow versions CHANGELOG
status: raw
created: 2026-04-10T23:40:06Z
tags: [hooks, release, changelog]
related: [cli/commands/check.ts]
---

# workflow-pre-pr hook false-blocks after release flow

## Problem

The `workflow-pre-pr` check (`loaf check --hook workflow-pre-pr`) exits 2 (blocking) when `[Unreleased]` is empty. It has an escape hatch that checks for a git tag on HEAD — if tagged, it assumes the release flow moved entries to a version header.

But in the actual release flow, the version bump happens on the **feature branch** before merge. Tags go on main **after** the squash merge. So the escape hatch never fires on the feature branch, and the hook false-blocks `gh pr create`.

## Fix

In `cli/commands/check.ts` lines 480-498: also check if a version header matching the current `package.json` version exists with entries in CHANGELOG. If `[2.0.0-dev.27]` has entries and matches the package version, the release flow has already run — don't block.

## Discovered

During SPEC-029 release flow — the hook blocked PR creation even though the changelog was properly versioned.
