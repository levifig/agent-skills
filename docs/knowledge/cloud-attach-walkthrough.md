# Cloud and CI Attach Walkthrough

## Contents

- Overview
- Prerequisites
- Environment variables
- Cursor Cloud Agents
- Amp Orbs
- CI (GitHub Actions)
- Failure modes
- Verification

Ephemeral hosts — Cursor Cloud Agents, Amp Orbs, and CI runners — start from a fresh clone with no Loaf installed and no substrate attach state. This walkthrough covers the operator path from minting a project-scoped client credential through unattended attach on each surface. It satisfies the LOAF-67 H-tier connection documentation and the LOAF-78 walkthrough criterion for project-environment secrets and attach pre-warm.

## Overview

Three layers stack on cloud hosts:

1. **Bootstrap (LOAF-78)** — install scripts build the Loaf CLI and run `loaf install --to <harness>` under `LOAF_PROJECT_ENV=1` so skills, commands, and hooks resolve from the project checkout (`.cursor/`, `.amp/`, `.agents/skills/`), not user-level config.
2. **Secrets (LOAF-67)** — per-project environment secrets carry the bundled client credential minted by `loaf auth link`. Never paste the operator master key or account admin secret into cloud or CI secrets.
3. **Attach (LOAF-67)** — the start script (or an explicit CI step) runs `loaf attach`, which reads `.agents/loaf.conf`, `LOAF_CLIENT_TOKEN`, and optionally `LOAF_SYNC_URL`, then completes the unattended ceremony: HTTPS health → auth → pull → decrypt probe → capability check → HLC skew check → identity match.

When attach enforcement is active and the environment is unattached, every substrate-touching `loaf` command exits nonzero with a machine-readable refusal. SessionStart journal context hooks fail closed the same way. Exempt commands: `--version`, help, and the auth/attach/install surface needed to become attached.

## Prerequisites

Complete these steps once on a trusted operator machine before configuring cloud or CI environments.

### 1. Run a sync relay

Self-host the sync server (`loaf serve`) or use your deployed relay. The endpoint must be HTTPS in production (TLS-terminated reverse proxy is fine).

```bash
loaf serve --listen :8080 --db /path/to/sync.sqlite
```

### 2. Create the substrate account

```bash
loaf auth setup --endpoint https://sync.example.com --server-db /path/to/sync.sqlite
```

Store the Emergency Kit offline. The admin wire stays on the operator machine only.

### 3. Mint a project-scoped client credential

From the project checkout (`.agents/loaf.conf` must exist with `project_id`):

```bash
loaf auth link cursor:myproject --project "$(jq -r .project_id .agents/loaf.conf)"
```

Copy the one-line bundled client wire printed after `auth link ok`. This string is the value for `LOAF_CLIENT_TOKEN` in cloud and CI secrets.

Connection names are arbitrary but should encode harness and project for audit (`cursor:loaf`, `amp:loaf`, `ci:loaf`).

## Environment variables

| Variable | Required | Surface | Purpose |
|----------|----------|---------|---------|
| `LOAF_CLIENT_TOKEN` | Yes (cloud/CI attach) | All | Bundled client credential from `loaf auth link` |
| `LOAF_PROJECT_ENV` | Yes (cloud harness install) | Cursor Cloud, Amp | Routes `loaf install` to project-local `.cursor/`, `.amp/`, `.agents/skills/` |
| `LOAF_SYNC_URL` | No | All | Override sync endpoint embedded in the bundled credential |
| `LOAF_CONNECTION_NAME` | No | All | Connection name recorded in attach state (defaults to token id) |

Bootstrap scripts in this repository export `LOAF_PROJECT_ENV=1`. Cursor Cloud does not carry install-time shell exports into agent sessions, so the start script re-exports it. Prefer also setting `LOAF_PROJECT_ENV=1` as a durable project environment variable in the harness UI when available.

## Cursor Cloud Agents

This repository ships Cursor Cloud bootstrap artifacts:

| File | Role |
|------|------|
| `.cursor/environment.json` | Points install/start scripts |
| `.cursor/loaf-cloud-install.sh` | Builds CLI, runs `loaf install --to cursor` |
| `.cursor/loaf-cloud-start.sh` | Re-exports `LOAF_PROJECT_ENV`, pre-warms attach when token present |

### Operator setup

1. Open the Cursor Cloud project settings for this repository.
2. Add **project environment variables** (secrets):
   - `LOAF_CLIENT_TOKEN` — the bundled wire from `loaf auth link`
   - `LOAF_PROJECT_ENV` — `1` (recommended as a durable var, not only via start script)
   - `LOAF_SYNC_URL` — optional override if the relay URL differs from the credential
3. Confirm `.cursor/environment.json` references the install and start scripts (committed in-repo).

### What happens on each agent session

**Install/build phase** (`.cursor/loaf-cloud-install.sh`):

```bash
npm ci && npm run build:go   # when native binary missing for host
export PATH="$ROOT/bin:$PATH"
export LOAF_PROJECT_ENV=1
loaf install --to cursor --yes
```

**Start phase** (`.cursor/loaf-cloud-start.sh`):

```bash
export PATH="$ROOT/bin:$PATH"
export LOAF_PROJECT_ENV=1
if [[ -n "${LOAF_CLIENT_TOKEN:-}" ]]; then
  loaf attach
fi
```

After attach succeeds, substrate commands (`loaf journal log`, `loaf issue …`, etc.) proceed normally. If `LOAF_CLIENT_TOKEN` is missing or attach fails, substrate commands refuse loudly and SessionStart context emission fails closed.

### Manual verification on a cloud agent

```bash
loaf attach --json          # should exit 0 when secrets are correct
loaf journal recent         # should succeed after attach
loaf journal context --from-hook  # SessionStart path; fails closed when unattached
```

## Amp Orbs

Amp uses `.agents/setup` (fresh orb) and `.agents/resume` (wake) instead of Cursor's environment.json.

| File | Role |
|------|------|
| `.agents/setup` | Builds CLI, runs `loaf install --to amp` |
| `.agents/resume` | Re-runs project-environment install on wake |

### Operator setup

1. In Amp project configuration, add **project secrets / environment variables**:
   - `LOAF_CLIENT_TOKEN` — bundled wire from `loaf auth link`
   - `LOAF_PROJECT_ENV` — `1`
   - `LOAF_SYNC_URL` — optional
2. Optionally add attach pre-warm to `.agents/resume` (same pattern as Cursor start):

```bash
if [[ -n "${LOAF_CLIENT_TOKEN:-}" ]]; then
  loaf attach
fi
```

The committed `.agents/resume` currently re-installs harness surfaces only; attach can run explicitly in the orb session or via the optional resume hook above.

### What happens on orb prepare

**Setup** (`.agents/setup`):

```bash
npm ci && npm run build:go   # when native binary missing
export PATH="$ROOT/bin:$PATH"
export LOAF_PROJECT_ENV=1
loaf install --to amp --yes
```

**Resume** (`.agents/resume`):

```bash
export PATH="$ROOT/bin:$PATH"
export LOAF_PROJECT_ENV=1
loaf install --to amp --yes
```

Run attach once per session after setup/resume when the secret is present:

```bash
loaf attach
```

## CI (GitHub Actions)

CI runners are ephemeral: no attach state persists between jobs. Each job that touches the substrate must install Loaf, export secrets, and attach before substrate commands.

### Repository secrets

Add these in **Settings → Secrets and variables → Actions**:

| Secret | Value |
|--------|-------|
| `LOAF_CLIENT_TOKEN` | Bundled wire from `loaf auth link` (use a `ci:<repo>` connection name) |
| `LOAF_SYNC_URL` | Optional relay override |

Do not commit credentials. Do not use the operator master key in CI.

### Example workflow fragment

```yaml
jobs:
  substrate-check:
    runs-on: ubuntu-latest
    env:
      LOAF_PROJECT_ENV: "1"
      LOAF_CLIENT_TOKEN: ${{ secrets.LOAF_CLIENT_TOKEN }}
      LOAF_SYNC_URL: ${{ secrets.LOAF_SYNC_URL }}
    steps:
      - uses: actions/checkout@v4

      - uses: actions/setup-node@v4
        with:
          node-version: "22"

      - uses: actions/setup-go@v5
        with:
          go-version-file: go.mod

      - name: Build Loaf CLI
        run: |
          npm ci
          npm run build:go
          echo "$PWD/bin" >> "$GITHUB_PATH"

      - name: Attach to substrate
        run: loaf attach --json

      - name: Run substrate command
        run: loaf journal recent --limit 5
```

Notes:

- `LOAF_PROJECT_ENV=1` keeps any in-job `loaf install` on project-local paths if needed.
- Attach runs every job; there is no persistent attach state on the runner.
- Use `loaf attach --json` in CI so failures emit structured refusals in logs.
- Jobs that only run `go test` against isolated temp state (`LOAF_DB` in tests) do not need attach unless they invoke substrate-touching CLI paths.

### CI attach failures

Common causes:

- **Missing secret** — enforcement sees no token; attach never runs; substrate commands refuse.
- **Wrong project scope** — credential `project_id` does not match `.agents/loaf.conf`; attach exits with `attach-identity-mismatch`.
- **Reach/auth** — relay unreachable or token revoked; attach exits with `attach-reach-failed` or `attach-auth-failed`.

## Failure modes

Attach refusals are machine-readable. Use `--json` for automation.

| Code | Cause | Remedy |
|------|-------|--------|
| `attach-conf-missing` | No `.agents/loaf.conf` | Ensure conf ships with the clone |
| `attach-credential-missing` | `LOAF_CLIENT_TOKEN` empty | Add project secret from `loaf auth link` |
| `attach-credential-invalid` | Malformed bundled wire | Re-mint with `loaf auth link` |
| `attach-identity-mismatch` | Conf `project_id` ≠ credential scope | Mint a project-scoped token for this checkout |
| `attach-endpoint-insecure` | Non-HTTPS endpoint | Use TLS or set `LOAF_SYNC_URL` to HTTPS relay |
| `attach-reach-failed` | Relay health check failed | Verify relay URL and network egress |
| `attach-auth-failed` | Token rejected or revoked | Re-mint or un-revoke the connection |
| `attach-hlc-skew` | Clock grossly skewed from stream | Fix host NTP; retry attach |
| `environment-unattached` | Substrate command before attach | Run `loaf attach` or fix secrets |

## Verification

**Operator machine (local):**

```bash
loaf auth list                              # connection visible with last-seen after cloud attach
go test ./internal/cli/... -run CloudEnvironment -count=1
```

**Cloud / CI (after secrets configured):**

```bash
loaf attach --json                          # exit 0
loaf journal recent                         # exit 0 (substrate reachable)
```

**Unattached refusal (negative check):**

```bash
unset LOAF_CLIENT_TOKEN
loaf journal recent                         # exit 1, environment-unattached
```

Harness bootstrap script markers are tested by `TestCloudEnvironmentBootstrapArtifactsInstallCLI` in `internal/cli/cloud_environment_test.go`.
