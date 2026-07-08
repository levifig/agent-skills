---
change: gh-config-dir-profiles
created: 2026-07-08
branch: gh-config-dir-profiles
---

# GitHub Identity via Config-Dir Profiles

## Problem

`gh`'s identity is one global mutable pointer, and the two mechanisms built so far each carry a flaw the owner won't ship or live with. Tier-1 convergence (PR #99, merged) self-heals hooked sessions but *writes* the shared pointer on every mismatched call — read-only commands included — so concurrent multi-identity sessions collide more often than under the old pure-read guard. The tier-3 PATH shim (PR #100) survived adversarial review with zero blockers and was **rejected after implementation anyway**: a shim inherently requires PATH-ordering machinery in front of a well-known binary, and no consent UX changes that nature. Meanwhile the native per-command selector everyone actually wants (`GH_USER` / `--account`, cli/cli#12853) was closed unmerged upstream.

What upstream *does* endorse (maintainer comment on cli/cli#12145): **`GH_CONFIG_DIR` profiles** — a config directory per identity, each with its own single-account `hosts.yml`, selected by an environment variable that is a *path, not a secret*.

Constraints carried forward from the parked Change: agents keep typing bare `gh`; no credentials on disk beyond gh's existing keychain storage; a cloned repo must never install machinery on a machine; no PATH modification.

## Hypothesis

If each GitHub identity gets its own gh config profile and `GH_CONFIG_DIR` is wired per-project through each harness's native environment config, then every `gh` invocation in an agent session resolves the project's identity with **no shared mutable state contested between sessions** — no shim, no PATH edits, no tokens in env — and the enforcement hook can retreat from convergence (global writes) to pure-read verification, restoring the pre-#99 read-only posture without its deadlock. As a bonus the shim could never offer: session-scoped env is inherited by subprocesses, so `git push` over HTTPS — whose absolute-path `gh auth git-credential` helper structurally bypasses any PATH shim — resolves the same identity.

## Scope

**In**

- **Profile management in the CLI**: create a per-identity profile directory (proposed home: `~/.config/gh-profiles/<account>/`), guide the one-time `gh auth login` into it, report profile health. Proposed surface: `loaf gh-profile create|status <account>` — final naming is a review decision (see fog).
- **Per-project env wiring, Claude Code first**: map the repo's `integrations.github.account` to the machine's profile dir by writing `GH_CONFIG_DIR` into `.claude/settings.local.json` env (machine-local, never committed — the repo names the *account*, the machine maps account→profile). Wiring is offered through the config-maintenance flow (`loaf config check --fix` or the profile command), with explicit consent per project.
- **Hook demotion to verification**: when the incoming Bash environment carries a `GH_CONFIG_DIR` whose profile resolves to the configured account, the `github-account` check passes as a pure read — no probe of the global pointer, no switch. Convergence remains the fallback when no profile mapping exists. The `gh auth` administration exemption stands.
- **Spikes as first implementation steps** (fog): keyring isolation mechanics per config dir, and empirical verification of the credential-helper inheritance claim.
- **Guidance**: hook-system.md, git-workflow, and loaf-reference configuration/troubleshooting reframed around profiles as the recommended opt-in tier; the residual matrix carried over from the parked Change and updated for the new mechanism.

**Out** (deferred, not rejected)

- Env wiring for the other harnesses (Codex, Cursor, OpenCode, Amp) — needs the per-harness env-config capability matrix; Claude Code proves the model first. Manual-export documentation covers them meanwhile.
- Bare-terminal automation (direnv/mise recipes) — documented as an optional pattern, not Loaf machinery; tier 1 remains the terminal net.
- Profile config synchronization (aliases, gh settings shared across profiles) — live with duplication until it hurts.
- GUI-launched applications — same env invisibility as every mechanism considered; documented residual.

**Cut** (explicitly rejected)

- **The PATH shim** — rejected after full implementation and clean review (PR #100, parked; branch `per-invocation-gh-identity` preserved as evidence). Grounds: mechanism nature, not correctness. First entry for the future rejection KB.
- **`GH_TOKEN` injection as the wired mechanism** (hook rewrite, or the maintainer's session-wide `export GH_TOKEN=$(gh auth token --user …)` from the second #12145 comment) — concurrency-safe (env is per-process; verified zero disk mutation), but it makes the selector a live secret inherited by every subprocess of the session, and it cannot be statically wired into harness env config (static strings can't run command substitution — the computing layer required is the hook-rewrite tier already cut). Documented instead as the zero-setup *terminal* recipe, and spiked as a possible HTTPS workaround (fog).
- **Waiting on upstream `GH_USER`** — the PR was closed; if gh ever ships native per-command selection, profiles degrade gracefully into it (both are env-driven).
- **Auto-creating profiles or auto-wiring projects** — same consent doctrine as the shim: no repo-side knob, no heuristics, nothing exists on a machine that didn't ask.

## Observable Workflow

```text
# one-time per identity, per machine:
loaf gh-profile create levifig      # creates ~/.config/gh-profiles/levifig, guides gh auth login into it

# one-time per project, explicit consent:
loaf config check --fix              # offers: wire GH_CONFIG_DIR for "levifig" into .claude/settings.local.json

# from then on, in every agent session in this project:
gh pr list                           # runs as levifig via the profile — pointer never read, never written
git push                             # HTTPS credential helper inherits the same env (verified by spike)

# concurrent session in a work repo wired to a different profile: zero shared state, nothing to race
# the github-account hook: pure-read verification when the profile env is present; convergence fallback otherwise
```

## Rabbit Holes and No-Gos

- **No profile sync engine.** Duplicated gh aliases/config between profiles is accepted cost; the moment this Change starts syncing gh settings it has become a dotfile manager.
- **No env injection beyond the harness's native mechanism.** If a harness has no per-project env config, its coverage waits for the matrix follow-up — no wrapper scripts, no shell-hook installation, no PATH games through the back door.
- **No hook-side env mutation.** The hook verifies or falls back to convergence; it never sets `GH_CONFIG_DIR` itself (a check that mutates its own check condition is the circularity class the hook system just escaped).
- **No committed machine paths.** `GH_CONFIG_DIR` values live in machine-local files only (`settings.local.json`); anything committed names accounts, never paths.

## Decisions

Provenance: 1–6 accepted 2026-07-08 in the pivot conversation following the shim rejection (user: "Let's go"), building on the parked Change's carried-forward constraints; the upstream evidence is cli/cli#12145 (maintainer endorsement of config-dir profiles) and the closed cli/cli#12853.

1. **Profiles are the opt-in tier; the shim is dead.** Per-identity `GH_CONFIG_DIR` profiles replace both the deferred tier-2 (hook rewrite) and the rejected tier-3 (shim). The tier vocabulary simplifies to: tier 1 convergence (default, zero footprint), tier 2 profiles (recommended opt-in, env-based).
2. **The selector is a path, never a secret.** `GH_CONFIG_DIR` in harness env config is committed-safe in nature but still kept machine-local, because it encodes a machine's filesystem layout and the account→profile mapping is the machine's business (trust boundary carried from the parked Change: repo names the account, machine picks the mechanism).
3. **Claude Code proves the model before the matrix.** One harness wired natively end-to-end beats five wired speculatively; the env-config capability matrix for the rest is its own follow-up.
4. **The hook retreats to reads wherever profiles reach.** Verification (env present + profile account matches) is a pure read; convergence — with its disclosed collision cost — survives only as the fallback tier. This restores the pre-#99 read-only posture without resurrecting its deadlock, because the exemption and convergence remain beneath it.
5. **Spike before build on the two load-bearing claims.** Keyring isolation per config dir and credential-helper env inheritance are the Hypothesis's foundations and are currently supported by a maintainer comment and reasoning, not evidence. Both spike first; findings write back here.
6. **The parked Change is evidence, not waste.** Its contract decisions (consent doctrine, trust boundary, no-cache reasoning), spike findings (gh ≥ 2.40.0, `GH_TOKEN` env-only precedence), residual matrix, and review record transfer; its rejection enters the Cut list with grounds, seeding the rejection-KB pattern the sweep will formalize.

## Planning Contract

### Profile layout and creation

`~/.config/gh-profiles/<account>/` (respecting `XDG_CONFIG_HOME`); `loaf gh-profile create <account>` makes the directory and prints the login step (`GH_CONFIG_DIR=<dir> gh auth login`) rather than driving the interactive flow itself — gh owns its login UX. `status` reports: dir exists, `hosts.yml` present, exactly one account, account matches the profile name, and the *active-account* token resolves (`GH_CONFIG_DIR=<dir> gh auth token` with **no `--user` flag** — the spike proved `--user` bypasses profile scoping and would vacuously pass against any machine-known account). Status also warns when a stale osxkeychain internet-password for the host coexists with gh's entries (the spike's footgun finding). `loaf doctor` gains the same checks for every profile referenced by any known project mapping.

### Wiring mechanics (Claude Code)

`.claude/settings.local.json` env gains `GH_CONFIG_DIR: <absolute profile path>`. Written only with per-project consent through the config-maintenance flow; removal is documented (delete the key). Wiring additionally offers the **repo-local conditional credential helper** (`.git/config`: helper reset + `!GH_TOKEN=$(gh auth token --user <account>) gh auth git-credential`) for projects with HTTPS remotes — the spike-adjudicated HTTPS mechanism; skipped for SSH-remote projects. One remaining inheritance check rides U2: that harness Bash sessions actually receive settings env (the documented mechanism; verify once during wiring implementation).

### Hook verification semantics

Order of evaluation in the `github-account` check: (1) `gh auth` administration → pass untouched (existing exemption); (2) `GH_CONFIG_DIR` present in the hook's view of the command env AND its profile's single account equals the configured account → pass, pure read; (3) `GH_CONFIG_DIR` present but mismatched → block with a wiring-repair message (never converge someone's explicit profile choice); (4) absent → convergence fallback as shipped in #99. Detail for the spike: what the hook process can actually see of the tool's env — the hook payload carries the command string, not the session env, so detection mechanics (reading the settings file vs. env probing) are an implementation fog item.

### Residual matrix (carried and updated)

| Path | Behavior |
|---|---|
| Agent session, project wired | profile env → correct identity; pointer unread, unwritten |
| Agent session, project not wired | tier-1 convergence (disclosed collision cost) |
| Bare terminal without env | tier-1 convergence where hooks run; global pointer otherwise (direnv recipe documented, optional) |
| `git push` over HTTPS, wired session | **ships via the conditional credential helper** (repo-local `.git/config`: `helper =` reset, then `helper = !GH_TOKEN=$(gh auth token --user <account>) gh auth git-credential`) — spike-verified to pin identity with zero ambient env and the global pointer on the other account. `GH_CONFIG_DIR` inheritance alone is unreliable (works only under favorable helper ordering — the #188559 failure mechanism); `GH_TOKEN` export stays a terminal recipe |
| Stale OS-keychain credential for github.com | a pre-existing osxkeychain internet-password (work account) coexists with gh's entries on this machine; any future helper-ordering change could resurrect it as the winning HTTPS identity — flagged by the spike, worth deleting; `loaf gh-profile status`/doctor should warn when one exists |
| GUI-launched apps | no env, no hooks — global pointer; unchanged, documented |
| Untrusted repo | names an *account*; the machine maps it to a profile only if the user wired it — strictly better than the shim's silent selection |
| `gh auth` administration | exempt, passes through (unchanged) |

### Sequencing

Spikes first (Decision 5); U-order below is likelihood-of-change. Independent of the parked branch. Docs edits re-do the tier framing on main directly (the parked branch's U4 framing never merged).

## Implementation Units

- **U1 — Spikes.** Keyring isolation per `GH_CONFIG_DIR` (two profiles, same machine, tokens resolve independently; document storage mechanics observed) and credential-helper inheritance (`git push` HTTPS from a wired env hits the profile's identity). Findings write back into this contract; a failed spike stops the Change at the cheapest point.
- **U2 — Wiring UX and consent.** The config-maintenance offer, `settings.local.json` write mechanics, removal path, per-project consent wording.
- **U3 — Profile commands.** `loaf gh-profile create|status`, doctor integration, the health checks.
- **U4 — Hook verification tier.** The evaluation order above, with tests per branch (wired-match pure-read pass asserts zero switch and zero status-probe of the global dir; wired-mismatch block; unwired convergence regression).
- **U5 — Guidance and close-out.** Tier reframing in hook docs and loaf-reference, residual matrix into troubleshooting, CLI-reference metadata for the new command, agent-help list, build and eval regeneration.

## Verification Contract

Executable:

- **V1.** `loaf change check docs/changes/20260708-gh-config-dir-profiles` — zero violations; executable before implementation.
- **V2.** Spike evidence committed under `research/`: two-profile token isolation transcript and the credential-helper inheritance transcript, each reproducible by command.
- **V3.** Hook tests: wired-match passes with the global `hosts.yml` fixture unread and unwritten; wired-mismatch blocks with the repair message; unwired falls back to convergence (existing tests keep passing).
- **V4.** `loaf gh-profile status` and `loaf doctor` correctly classify: healthy profile, missing dir, multi-account profile, account-name mismatch, unresolvable token. Full suite and build green.

Human review:

- **H1.** The per-project wiring consent names the exact file and key it writes and how to undo it.
- **H2.** The residual matrix matches shipped behavior, including the spike-gated HTTPS row.
- **H3.** Dogfood on this machine: this repo wired to the `levifig` profile; the 2026-07-07 race rerun (adversary loop flipping the global pointer) with every probe resolving `levifig` — the parked Change's DoD, inherited.

## Definition of Done

- Spikes pass and their findings are folded in; U2–U5 shipped; V1–V4 green; H1–H3 confirmed.
- This repo runs wired as the first production use.
- The parked shim branch is referenced from the Cut list; no shim code merges.
- Deferred items (harness matrix, terminal automation) recorded for harvest.

## Durable Outputs

- ADR candidate after implementation: GitHub identity via config-dir profiles — the selector-is-a-path principle, the tier retreat from writes to reads, and the shim rejection rationale.
- `docs/knowledge/` note on the identity tiers, written from shipped behavior.
- The rejection record (shim) as seed material for the sweep's rejection KB.

## Open Questions

- ~~[KU] Keyring isolation mechanics~~ → resolved by U1 spike (`research/spike-keyring-isolation.md`): **isolation is real at the pointer layer only**. Each profile's `hosts.yml` is a pointer naming its active account and holds no secret; credentials live once machine-wide in the OS keychain keyed `(gh:<host>, <username>)` — three temp-profile logins created zero new keychain entries. Commands reading the *active* pointer (`gh api`, `gh auth status`, `gh auth git-credential`) are correctly profile-scoped; `--user`-flagged commands and `GH_TOKEN` bypass profiles entirely and can pull any machine-known account's token from any profile. Consequences folded in: profile health checks must probe the active account, never `--user` (Planning Contract updated); the Hypothesis's no-shared-state claim holds for what raced (pointer writes) — the credential *store* is shared, read-mostly, and uncontested.
- ~~[KU] Credential-helper inheritance~~ → resolved by U1 spike (`research/spike-https-credential-helper.md`): **#188559 did not reproduce here, but its mechanism is real** — `GH_CONFIG_DIR` inheritance works on this machine only by config-merge ordering luck (gh's github.com-specific helper happens to layer after Homebrew's system-level `osxkeychain`); a differently-ordered machine plausibly fails exactly as reported. Adjudication: **the conditional credential helper ships** (repo-local `.git/config`, verified pinning identity with zero ambient env and the pointer on the other account); `GH_TOKEN` short-circuit works unconditionally but stays a terminal recipe (secret-in-env); `GH_CONFIG_DIR`-alone is documented as unreliable, never sufficient. Bonus finding: a stale osxkeychain internet-password for github.com (under the work account) sits on this machine as a standing footgun — added to the residual matrix.
- [KU] Hook-side detection: can the check see the tool's effective env, or must it read `.claude/settings.local.json` (and equivalents) from disk? → owned by U4; recommended: read the settings file — deterministic, no env-passing dependency on harness hook payloads.
- [KU] Command naming: `loaf gh-profile` vs a subcommand under an existing noun — does a second gh-adjacent surface (after the parked `loaf shim`) warrant a shared `loaf gh …` namespace? → grilling question at review; recommended: `loaf gh-profile`, flat, renameable before release.
- [UK] Consent/offer wording for the per-project wiring → reaction artifact at U2: drafts in `research/`, pick at review.
- [UU] What else reads gh config location or assumes the default dir (gh extensions, other tools shelling to gh, IDE integrations) → blindspot pass during U1 alongside the spikes.

## Source Inputs

- The parked Change `docs/changes/20260707-per-invocation-gh-identity/` (branch `per-invocation-gh-identity`, PR #100 closed with the park rationale) — contract decisions, spike findings, residual matrix, and two external review rounds carried forward.
- cli/cli#12145 and its maintainer comment (2025-11-14) endorsing `GH_CONFIG_DIR` profiles; cli/cli#12853 (`--account`/`GH_ACCOUNT`) closed unmerged — the native path not taken by upstream.
- GitHub Community Discussion #188559 (via devactivity.com write-up, evaluated 2026-07-08) — a **disconfirming input** for the HTTPS credential-helper row: `GH_CONFIG_DIR` respected by gh itself but reportedly not by `git push` credential resolution. Sharpened both U1 spikes; per the pilot's survivorship-bias practice, deliberately kept alongside the confirming maintainer comment.
- The second #12145 maintainer comment (session-wide `export GH_TOKEN=$(gh auth token --user …)`) — concurrency-safe and endorsed, rejected as the *wired* mechanism (secret in session env; not statically wireable for agents); kept as the zero-setup terminal recipe and an HTTPS-spike candidate.
- The third #12145 comment (simenbrekken, 2026-03-30) — the **conditional credential helper**: per-tree/per-repo git config overriding the helper with inline named-token resolution. Field-proven for git-HTTPS identity (the surface `gh`'s own mechanisms miss); complements profiles rather than competing; the likely shipped answer for the HTTPS matrix row and a U2 wiring candidate (repo-local `.git/config`).
- The 2026-07-08 pivot conversation: shim rejection grounds, the no-PATH constraint made explicit, profile mechanism evaluation, tier simplification — user-approved.
- PR #99 (merged) — tier-1 convergence with the `gh auth` exemption and the disclosed collision-frequency cost this Change lets wired projects escape.
- Journal decisions of 2026-07-07/08 under `decision(github-identity)` and `decision(hooks)`.
