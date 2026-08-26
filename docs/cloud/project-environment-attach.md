# Cloud Project-Environment Attach (LOAF-67)

Human-review guide for wiring unattended attach on ephemeral cloud hosts. V-tier checks cover bootstrap script markers and install layout; this document is the H-tier review surface for operator setup.

## Contents

- Prerequisites
- Mint the client wire
- Cursor Cloud Agents
- Amp Orbs
- CI and review checklist

## Prerequisites

- Self-hosted sync relay running (`loaf serve`) with admin bootstrap complete (`loaf auth setup`).
- Project registered in Loaf state (`loaf project show` from the repo root).
- `LOAF_PROJECT_ENV=1` on every cloud session that runs `loaf install` or `loaf attach` so harness surfaces stay project-local (`.cursor/`, `.amp/`, `.agents/skills/`).

Never place the operator master key or Emergency Kit in cloud project environment variables. Cloud hosts receive only the bundled client wire minted by `loaf auth link`.

## Mint the client wire

From an operator workstation with admin access:

```bash
loaf auth link <connection-name> --project
```

Copy the emitted client wire (single line) into the cloud secret store as `LOAF_CLIENT_TOKEN`. Optionally set `LOAF_SYNC_URL` when the relay endpoint differs from the wire default.

## Cursor Cloud Agents

1. Ensure `.cursor/environment.json` points install/start at `.cursor/loaf-cloud-install.sh` and `.cursor/loaf-cloud-start.sh` (validated by `TestCloudEnvironmentBootstrapArtifactsInstallCLI`).
2. Add project environment variables in the Cursor Cloud project settings:
   - `LOAF_PROJECT_ENV=1`
   - `LOAF_CLIENT_TOKEN=<client wire from auth link>`
   - `LOAF_SYNC_URL=<relay URL>` (optional)
3. Install phase builds the native CLI and runs `loaf install --to cursor --yes` under project layout.
4. Start phase re-exports `LOAF_PROJECT_ENV=1` and runs `loaf attach` when `LOAF_CLIENT_TOKEN` is non-empty.
5. SessionStart hooks fail closed when substrate auth is configured locally but attach has not succeeded.

## Amp Orbs

1. Fresh orb: `.agents/setup` builds the CLI and runs `loaf install --to amp --yes`.
2. Resume: `.agents/resume` re-runs install, then `loaf attach` when `LOAF_CLIENT_TOKEN` is set.
3. Configure Amp project secrets / environment:
   - `LOAF_PROJECT_ENV=1`
   - `LOAF_CLIENT_TOKEN=<client wire from auth link>`
   - `LOAF_SYNC_URL=<relay URL>` (optional)

## CI and review checklist

H-tier review (not gated by `loaf issue verify`):

- [ ] Client wire minted for a named connection scoped to the correct project ID
- [ ] Cloud project env uses `LOAF_CLIENT_TOKEN`, not master key or admin wire
- [ ] `LOAF_PROJECT_ENV=1` present on install and session entry scripts
- [ ] Bootstrap scripts unchanged except via reviewed PR (markers enforced in Go tests)
- [ ] Attach refusal paths understood: identity mismatch, revoked token, gross HLC skew
- [ ] Sync relay reachable from cloud network; TLS preferred (use `--allow-insecure-http` only in dev)

Related: [cloud-attach-walkthrough.md](../knowledge/cloud-attach-walkthrough.md) (step-by-step for Cursor Cloud, Amp, and GitHub Actions CI), `internal/cli/cloud_environment_bootstrap.go`, `docs/security/substrate-e2e-threat-model.md`, ADR-029 fact envelope sync contract.
