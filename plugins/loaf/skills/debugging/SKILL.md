---
name: debugging
description: >-
  Covers systematic debugging, hypothesis tracking, and flaky test
  investigation. Use when diagnosing failures, tracking hypotheses, or fixing
  flaky tests. Provides methodology for root cause analysis and issue
  resolution. Not for writing new tests ...
user-invocable: true
argument-hint: '[issue or error]'
allowed-tools: 'Read, Write, Edit, Bash, Glob, Grep'
version: 2.0.0-dev.12
---

# Debugging

Systematic debugging methodology with hypothesis tracking.

## Contents
- Critical Rules
- Verification
- Topics

## Critical Rules

- **Hypothesize before changing code** -- form and record a hypothesis before making any fix attempt; never shotgun-debug by changing things at random
- **One variable at a time** -- change only one thing per test iteration to isolate cause from coincidence
- **Reproduce first** -- confirm you can reliably trigger the failure before investigating; if it cannot be reproduced, focus on gathering more signal
- **Track hypotheses explicitly** -- maintain a written list of hypotheses with their status (confirmed/rejected/pending) so investigation stays structured

## Verification

- Root cause is identified and documented, not just the symptom
- The fix is validated by reproducing the original failure scenario and confirming it no longer occurs
- For flaky tests: the test passes reliably across multiple consecutive runs after the fix

## Topics

| Topic | Reference | Use When |
|-------|-----------|----------|
| Debugging | `references/debugging.md` | Systematically debugging issues with hypotheses |
| Hypothesis Tracking | `references/hypothesis-tracking.md` | Managing multiple hypotheses during investigation |
| Test Debugging | `references/test-debugging.md` | Fixing flaky tests, isolation issues, state pollution |
