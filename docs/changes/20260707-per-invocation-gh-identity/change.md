---
change: per-invocation-gh-identity
created: 2026-07-07
branch: per-invocation-gh-identity
---

# Per-Invocation GitHub Identity

## Problem

`gh`'s identity is a single global mutable pointer (`~/.config/gh/hosts.yml` active account) with no per-directory resolution — nothing analogous to git's `includeIf`. For anyone running multiple GitHub identities on one machine, concurrent sessions race that pointer: observed live on 2026-07-07, when a parallel session flipped the account between the guard's check and gh's API call, and a PR was nearly created under the wrong identity ("must be a collaborator" was the only thing that stopped it). The `github-account` guard's convergence behavior (PR #99) self-heals Loaf-hooked sessions but *mutates the same global pointer*, so the flip-war between ecosystems continues — convergence treats the symptom per session, not the disease.

Constraints that bound the solution:

- Agents are trained on bare `gh`; a wrapper command (`loaf gh`) fights model priors and was rejected.
- No new credential storage: the existing gh keychain-backed token store is the accepted secret holder.
- Other Loaf users must never have their PATH touched without explicit consent — a cloned repo must not be able to impose machinery on a machine.

## Hypothesis

If identity is resolved **per invocation** — named account from project config, token fetched for that name, injected into the child process environment only — then wrong-account operations become impossible for PATH-resolved `gh` inside identity-configured Loaf projects, on any number of concurrent sessions, because no shared mutable state remains in the invocation path. Policy stays in the repo, mechanism choice stays with the human, and agents keep typing plain `gh`.

## Scope

**In**

- **argv[0] dispatch in the loaf binary**: invoked as `gh` (via symlink), loaf walks up from CWD for `.agents/loaf.json`; outside a Loaf project or without a configured account it execs the real gh untouched; inside one it resolves `gh auth token --user <configured>` and execs the real gh with `GH_TOKEN` set.
- **Shim lifecycle owned by the CLI**: explicit opt-in enable (with a consent prompt that states exactly what changes), disable, and status; `loaf doctor` reports shim presence, health, and the resolved real-gh path.
- **The fall-through contract as a tested invariant**: non-Loaf directory, no configured account, missing keychain entry, or any resolution failure → exec real gh exactly as if the shim didn't exist, plus a stderr note only when resolution failed for a *configured* project.
- **Guard demotion**: the `github-account` hook's convergence (PR #99) becomes the documented fallback tier for bypasses and non-shim users; hook text points at the shim as the stronger mechanism.
- **Guidance**: loaf-reference (configuration/troubleshooting) and git-workflow updates describing the tiers and the residual-exposure matrix.

**Out** (deferred, not rejected)

- **Tier 2 (harness-hook command rewrite)** — PreToolUse input-modification support varies across the six harnesses; needs its own capability-matrix research spike. The shim covers strictly more ground; T2 only wins where users refuse the shim. Own follow-up if demand appears.
- **Alternative token sources** — `op`/1Password resolver and GitHub-App-minted ephemeral tokens ride behind the token-source seam later; v1 is keychain-only by explicit user acceptance.
- **Windows** — the shim mechanics (symlink, exec) are POSIX; Windows users stay on tier 1.
- **Absolute-path gh invocations** (e.g., git credential helpers calling `/opt/homebrew/bin/gh`) — structurally outside a PATH shim's reach; documented residual, covered by tier 1 where Loaf hooks run.

**Cut** (explicitly rejected)

- A `loaf gh` wrapper command — fights agent priors; the whole point is that nobody retypes anything.
- Any token cache — at ~30ms per keychain read there is nothing to amortize, and a cache would reintroduce the only shared surface this design eliminates.
- Auto-enabling the shim, ever — including "detected multi-account, enabling shim" heuristics; consent is explicit or the mechanism doesn't exist on that machine.
- Mutating `hosts.yml` from the shim path — the shim never writes gh state; convergence (tier 1) remains the only writer, and only in its own tier.

## Observable Workflow

```text
# one-time, per machine, explicit:
loaf shim enable gh        # prints what it will do (symlink + PATH line), asks, records consent user-side
loaf shim status           # or: loaf doctor — shim present, healthy, real gh at /opt/homebrew/bin/gh

# from then on, everyone — human, Claude, Codex, scripts — just types gh:
cd ~/Code/levifig/projects/loaf && gh pr list     # runs as levifig, always
cd ~/Code/enline/some-repo && gh pr list          # no loaf.json account → real gh, untouched behavior
# two of those at the same instant in two TTYs: each carries its own token; nothing shared, nothing to race

loaf shim disable gh       # one command, machinery gone
```

`gh auth switch` still works everywhere but stops mattering inside identity-configured Loaf projects — shimmed invocations never consult the active pointer.

## Rabbit Holes and No-Gos

- **No general shim framework.** This is one shim for one binary. No `loaf shim enable <anything>` plumbing beyond gh; a second shim candidate must bring its own Change. The subcommand is named `shim` for honesty, not extensibility.
- **No gh behavior emulation.** The shim resolves and execs — it never parses gh output, never intercepts subcommands, never special-cases `gh auth`. If gh semantics under `GH_TOKEN` surprise us (see fog), we document, not patch.
- **No PATH editing on the user's behalf beyond the consented line.** Enable prints the exact line and where it goes; if the user's shell setup is exotic, it instructs instead of guessing.
- **No identity fallback chains.** Configured account with no keychain entry = hard fall-through with a stderr note, never "try the active account instead" — a wrong-identity success is strictly worse than a visible failure.

## Decisions

Provenance: all accepted 2026-07-07 in the design conversation following the live account-flip race during PR #97's ceremony; user approved the converged design ("let's work on this"), keychain acceptance, and the no-wrapper constraint explicitly.

1. **Policy and mechanism split at the trust boundary.** The repo declares *which* account (`integrations.github.account`); the machine's user-level config declares *how* it's enforced. A cloned repo can never install machinery.
2. **Tiered enforcement, user-selected.** T1 convergence (PR #99, default, zero footprint), T3 opt-in PATH shim (full coverage); T2 harness rewrite deferred (Out). Same invariant at every tier; footprint and coverage scale together.
3. **Agents type bare `gh`.** The mechanism is invisible at the prompt; `loaf gh` rejected as prior-fighting.
4. **T3 is argv[0] dispatch in the loaf binary.** A symlink named `gh` to the loaf binary; Go all the way down, no shell-script shim, one artifact to version and test.
5. **v1 token source is the gh keychain, by name.** `gh auth token --user <configured>` — a read of a *named* account, never "whoever's active"; ~30ms; accepted as secure enough. The resolver sits behind a token-source seam for future `op`/app-minted sources.
6. **No cache.** Deliberate deletion of the last shared surface; resolution is a pure function per invocation.
7. **Fall-through is the hard contract.** Outside identity-configured Loaf projects the shim is behaviorally nonexistent. Tested, not promised.
8. **Consent is explicit, visible, and revocable.** Enable prompts with the exact changes; `loaf doctor` surfaces the shim forever after; disable is one command. The pattern users hate is surprise, not shims.
9. **PR #99 merges first and survives as tier 1.** Convergence is the net under every bypass (absolute paths, non-shim machines) where Loaf hooks run; this Change re-documents it as fallback rather than replacing it.

## Planning Contract

### Resolution pipeline (the dispatch path)

`main()` inspects `filepath.Base(os.Args[0])`. When it is `gh`: walk up from CWD to the git root (or filesystem root) for `.agents/loaf.json`; parse `integrations.github.account` (reuse `configuredGitHubAccount`); empty → exec real gh. Resolve the real gh path from the recorded shim config (set at enable time), never by re-walking PATH at run time (recursion guard). Token: run real-gh `auth token --hostname github.com --user <account>`; non-zero exit → fall through to real gh with one stderr line (`loaf gh-shim: token for "levifig" unavailable; running unshimmed`). Success → exec real gh with the caller's argv and env plus `GH_TOKEN` (and `GH_TOKEN` only — no other env surgery).

### Shim lifecycle

`loaf shim enable gh`: resolve real gh (PATH minus the shim dir), create `~/.local/share/loaf/shims/gh → <loaf binary>`, record `{real_gh, enabled_at}` in user-level config (XDG, not the repo), print the PATH line for the user's shell and offer to append it to the profile (explicit y/n; declining leaves instructions). `disable`: remove symlink + config entry, leave the PATH line (harmless when the dir is empty; say so). `status` + `loaf doctor`: symlink integrity, PATH ordering actually resolves the shim first, real gh still exists at the recorded path, gh version compatibility (see fog).

### Config schema

Repo side (exists): `integrations.github.account`. User side (new): `shims.gh: {real_path, enabled_at}` in `$XDG_CONFIG_HOME/loaf/config.json` (fallback `~/.config/loaf/config.json`) — a new generic user-config file, deliberately distinct from the SQLite state DB (data-home, project-scoped) and the install-target records; writes preserve unrelated keys for forward compatibility *(implementation clarification, U1)*. No repo-side knob can enable, request, or nag about the shim — the policy/mechanism boundary (Decision 1).

Walk-up semantics *(implementation clarification, U2)*: an iterative directory walk from CWD checking `.agents/loaf.json` at each level, bounded by the first `.git` entry (file or dir — worktrees included) or the filesystem root; pure `os.Lstat`, no subprocesses. The git boundary prevents an unrelated ancestor's loaf.json from leaking into a nested repo. `enable` is idempotent over both `healthy` and `path-shadowed` states — the freshly-enabled current shell hasn't sourced the new PATH line yet, and must not re-trigger the consent flow.

### Failure and residual matrix

| Path | Behavior |
|---|---|
| Non-Loaf dir | exec real gh, silent, byte-identical behavior |
| Loaf dir, no account configured | exec real gh, silent |
| Malformed `.agents/loaf.json` | exec real gh, silent fall-through; `loaf config check` owns the diagnosis |
| Configured, keychain entry missing | exec real gh + one stderr note (visible failure over wrong-identity success) |
| Configured, resolution succeeds | exec real gh with `GH_TOKEN`, global pointer untouched |
| Recorded `real_path` stale (gh moved/uninstalled) | last-resort PATH-minus-shim-dir walk; `loaf doctor` flags `real-gh-missing` |
| Absolute-path gh invocation | shim never sees it; tier 1 catches Loaf-hooked sessions |
| **git-over-HTTPS via `gh auth setup-git`'s credential helper** | absolute-path (`!/opt/homebrew/bin/gh auth git-credential`) — bypasses the shim for plain `git push/pull`; the near-universal residual trigger. SSH remotes (this user's setup) unaffected; documented in U4 |
| **GUI-launched apps / launchd** | never source shell profiles; resolve gh via `/etc/paths.d` — shim invisible to Dock-launched IDEs and git clients; inherent PATH-shim limit (same as rbenv/direnv) |
| `gh auth switch` under shim | passes through to real gh; mutates pointer (env-precedence only — spike confirmed zero disk writes from `GH_TOKEN` itself); shimmed calls keep ignoring it |
| **Untrusted repo selects the identity** | a cloned repo's committed `.agents/loaf.json` silently picks *which* of the user's own authenticated identities shimmed `gh` runs as — the policy/mechanism split's dark side (external review, round 1). Bounded: only identities the user already holds tokens for, never new credentials. Mitigation candidates (first-resolution-per-project trust prompt, doctor listing of identity-configured projects) deferred to follow-up; documented in U4 |

### Sequencing

PR #99 merges before this lands (Decision 9). This Change is content-independent of PR #97/#98, but its guidance edits (U4) touch skill files PR #97 also touches — implementation of U4 starts from whatever main holds after #97 merges, or rebases across it.

## Implementation Units

Ordered by likelihood-of-change:

- **U1 — Consent UX and config schema.** The enable/disable/status surface, prompt wording, user-level config shape, doctor integration. Most likely to be reworked in review — everything user-facing lives here.
- **U2 — Resolution pipeline.** argv[0] dispatch, config walk-up, named-token resolution, exec with injected env, recursion guard, the failure matrix's fall-through branches.
- **U3 — Contract tests.** Fall-through byte-identical behavior outside Loaf projects (stub gh records argv/env); named-account resolution never reads the active pointer; concurrent-invocation isolation (two stub projects, two accounts, parallel runs); recursion guard.
- **U4 — Guard demotion and guidance.** Hook description/docs reframe convergence as tier 1; loaf-reference configuration/troubleshooting gain the tier table and residual matrix; git-workflow note.
- **U5 — Mechanical close-out.** `loaf --help`/agent-help/CLI-reference regeneration for the new `shim` command, build, routing-eval unaffected-check.

## Verification Contract

Executable:

- **V1.** `loaf change check docs/changes/20260707-per-invocation-gh-identity` exits zero; executability derived before implementation begins.
- **V2.** Contract tests (U3) pass: outside-project invocation execs the recorded real gh with unmodified argv/env; configured-project invocation carries `GH_TOKEN` for the *named* account with the global pointer file unread (stub asserts no `auth status --active` call) and unwritten; two parallel invocations against different stub projects each receive their own token.
- **V3.** Missing-keychain-entry path falls through with the single stderr note and a zero-mutation guarantee (`hosts.yml` content identical before/after).
- **V4.** `npm run test` and `npm run build` green; `loaf doctor` reports the shim states (absent / healthy / broken symlink / PATH-shadowed) correctly in a scripted harness.

Human review:

- **H1.** The consent prompt says exactly what will change on the machine — a reviewer who reads only the prompt can predict every filesystem/profile mutation enable performs.
- **H2.** The residual-exposure matrix in the shipped docs matches implemented behavior — no documented guarantee the tests don't back.

## Definition of Done

- `loaf shim enable|disable|status gh` shipped with the dispatch pipeline; V1–V4 green; H1–H2 confirmed in review.
- PR #99 merged beforehand; hook docs reframed as tier 1.
- Dogfooded on this machine: this repo's `gh` operations run shimmed as `levifig` while a second concurrent session holds the pointer elsewhere — the live race from 2026-07-07 rerun deliberately and won.
- Deferred items (T2 matrix, op/app-minted sources, Windows) recorded in Out for harvest.

## Durable Outputs

- ADR candidate once implementation proves the model: per-invocation identity via argv[0] dispatch — the policy/mechanism trust boundary and the no-shared-state invariant.
- `docs/knowledge/` note on the gh identity tiers (which tier catches what), written from shipped behavior.
- loaf-reference configuration/troubleshooting updates (in scope, U4) — durable by living in the skill surfaces.

## Open Questions

- ~~[KU] Minimum gh version~~ → resolved by U2 spike: `--user` on `auth token`/`switch`/`status` shipped together in **gh v2.40.0** (cli/cli PR #8425, 2023-12-07). `enable` hard-refuses below 2.40.0, naming the installed version and the requirement.
- ~~[KU] `gh auth` behavior under `GH_TOKEN`~~ → resolved by U2 spike, empirically and read-only: `GH_TOKEN` is pure runtime precedence — **zero disk mutation** (`gh help environment`: "takes precedence over previously stored credentials"). With a valid named token, `gh auth status` exits 0 and shows the account source-tagged `(GH_TOKEN)` instead of `(keyring)` — cosmetic; one line in U4's troubleshooting doc covers it.
- ~~[KU] PATH-line placement per shell~~ → resolved by U1 implementation: shell detection via `$SHELL` (zsh/bash/fish), explicit y/n profile-append offer, and `loaf doctor`/`shim status` verify *effective* resolution (the `path-shadowed` state) rather than trusting the profile edit; `enable` treats `path-shadowed` as already-enabled so the not-yet-sourced current shell doesn't re-trigger consent. Review confirms the wording.
- ~~[UK] Consent-prompt wording~~ → reaction artifact delivered: three drafts in `research/consent-prompt-drafts.md`; Draft A (itemized mutation checklist + one safety paragraph, including the explicit "hosts.yml is read-only, never written" line) implemented as the recommendation. **Final pick is a veto point at PR review.**
- ~~[UU] What else resolves gh via PATH~~ → blindspot pass returned two machine-verified findings, folded into the residual matrix below: (1) `gh auth setup-git` bakes an **absolute-path** credential helper into gitconfig, so plain HTTPS `git push/pull` bypasses the shim and still races the pointer — the near-universal trigger for the absolute-path residual; (2) macOS GUI-launched apps (Dock/Spotlight) never source shell profiles and resolve gh via `/etc/paths.d` — IDEs and GUI git clients see the real gh unless launched from a shim-aware terminal. Non-login shells (cron, bare ssh) and processes started pre-enable share the same inherent PATH-shim limit, as with rbenv/direnv.

The register is empty; the consent-prompt pick and H1–H2 remain for the PR review round.

## Source Inputs

- The 2026-07-07 design conversation (this session): the live account-flip race during PR #97's ceremony, the deadlock finding that produced PR #99, the wrapper-vs-priors constraint, keychain acceptance, and the tier model — user-approved turn by turn.
- Journal decisions `decision(hooks)` and `decision(github-identity)` of 2026-07-07 (guard redesign; converged tier design).
- PR #99 (`github-account` convergence) — the tier-1 mechanism this Change demotes to fallback, and its body's documented race limitation.
- `internal/cli/github_account.go` — existing config parsing (`configuredGitHubAccount`) and named-account probing reused by the pipeline.
- gh CLI multi-account model (`hosts.yml` active pointer, keychain token storage, `GH_TOKEN` precedence, `gh auth token --user`) — the external constraint set.
- Precedent for consented PATH shims: rbenv/pyenv/mise, direnv's explicit hook line, VS Code's `code` installer.
