---
title: Release skill should verify build artifacts after post-bump commits
status: raw
created: 2026-04-11T00:15:00Z
tags: [release, build, skill]
related: [content/skills/release/SKILL.md]
---

# Release skill should verify build artifacts after post-bump commits

## Problem

The release skill's Step 4 (version bump) assumes the bump commit is the last one before merge. But in practice, integration testing often reveals bugs that get fixed after the bump. The build artifacts in the bump commit then become stale.

Discovered during SPEC-029: the stdin prompt fix (`a49213c`) landed after the version bump (`69a47c7`), leaving `plugins/loaf/bin/loaf` stale until a manual rebuild.

## Fix

Add a check to Step 4's "After either path" section or Step 5 (before merge): "If additional commits landed after the version bump, rebuild and verify build artifacts are up to date before merging."

Could also be automated: the pre-merge check could compare the version bump commit's tree hash for `plugins/` and `dist/` against HEAD.
