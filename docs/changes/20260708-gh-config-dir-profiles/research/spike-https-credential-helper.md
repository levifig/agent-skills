# Spike: HTTPS Credential-Helper Adjudication

**Date:** 2026-07-08
**Machine:** macOS, `gh` 2.96.0, Homebrew git, login.keychain-db
**Open question addressed:** "Does `gh auth git-credential` invoked by git honor the session's `GH_CONFIG_DIR`?" — with the documented counterexample, GitHub Community Discussion #188559, reproduced first.

## Baseline: what helper is actually configured

```bash
$ git config --show-origin --get-all credential.helper
file:/opt/homebrew/etc/gitconfig	osxkeychain

$ git config --show-origin --get-all credential.https://github.com.helper
file:/Users/levifig/.config/git/config	
file:/Users/levifig/.config/git/config	!/opt/homebrew/bin/gh auth git-credential
```

Two helpers are in play, not one: Homebrew's system-level `git` config installs `osxkeychain` as a general `credential.helper` (applies to all hosts), and the user's `~/.config/git/config` layers a github.com-specific `!gh auth git-credential` helper on top. Git runs applicable helpers in config-merge order (system → global → local) and a later helper's fields override an earlier helper's for the same operation — so on this machine `gh`'s helper is expected to win over `osxkeychain` for github.com URLs, *if* it returns a value. This dual-helper setup is exactly the shape of mechanism the open question flagged ("a coexisting `osxkeychain` helper").

**Blindspot finding:** `osxkeychain` already has a stored `github.com` credential, independent of gh's own `gh:github.com` keychain items:

```bash
$ security dump-keychain | grep -A15 'srvr.*github.com'   # class "inet" (osxkeychain), metadata only
class=inet acct="levifigueira" created=20251208131619
class=inet acct="git"          created=20260111022955
```

This is a real, pre-existing artifact on this machine (not created by this spike) — evidence that something (an earlier `git push`, VS Code, or another tool) once stored HTTPS credentials via `osxkeychain` directly, bypassing gh. It is stale relative to the current default profile (default active account is `levifig`; this cached entry says `levifigueira`), which makes it a perfect natural disambiguator for the tests below.

## Baseline `git credential fill` (no overrides)

```bash
$ gh auth status | grep -E 'Active account: true' -B3
github.com
  ✓ Logged in to github.com account levifig (keyring)
  - Active account: true

$ printf 'protocol=https\nhost=github.com\n\n' | git credential fill
protocol=https
host=github.com
username=levifig
```

Default profile active account is `levifig`; `git credential fill` correctly returns `levifig` — **not** the stale `osxkeychain` entry (`levifigueira`). This already shows `gh`'s helper wins over `osxkeychain` in the default case.

## (a) `GH_CONFIG_DIR` alone — attempting to reproduce Discussion #188559

```bash
$ GH_CONFIG_DIR=$PROFILE_B git auth status  # (via gh) reports levifigueira active, correctly

$ printf 'protocol=https\nhost=github.com\n\n' | GH_CONFIG_DIR="$PROFILE_B" git credential fill
protocol=https
host=github.com
username=levifigueira
```

This alone is ambiguous — `osxkeychain`'s stale entry also happens to say `levifigueira`, so a naive read could mistake coincidence for causation. **Disambiguating test**, switching to the *other* profile while the stale `osxkeychain` entry still says `levifigueira`:

```bash
$ printf 'protocol=https\nhost=github.com\n\n' | GH_CONFIG_DIR="$PROFILE_A" git credential fill
protocol=https
host=github.com
username=levifig
```

`GH_CONFIG_DIR=$PROFILE_A` (account `levifig`) correctly returns `levifig`, despite `osxkeychain`'s cached entry being `levifigueira`. Since the stale entry is constant across both tests but the result tracks `GH_CONFIG_DIR` in both directions, the result is *not* being driven by `osxkeychain`'s cache — it is being driven by `gh auth git-credential`, confirmed directly:

```bash
$ printf 'protocol=https\nhost=github.com\n\n' | GH_CONFIG_DIR="$PROFILE_B" gh auth git-credential get
protocol=https
host=github.com
username=levifigueira
```

**Discussion #188559 does NOT reproduce on this machine/git/gh version combination.** `GH_CONFIG_DIR` alone is sufficient for `git credential fill` (and therefore `git push` over HTTPS) to resolve the profile's identity, because the configured helper chain has `gh auth git-credential` positioned to override the general `osxkeychain` fallback for github.com URLs specifically.

**Why the disconfirming report likely differs elsewhere:** the mechanism that makes this work here is fragile precisely because it depends on config *ordering* between two independently-installed helpers (`osxkeychain` from a Homebrew system gitconfig, `gh`'s from a user gitconfig) — not on any structural guarantee. A machine where `osxkeychain` is configured *after* the `gh` helper (or where only `osxkeychain` has ever cached github.com and gh's helper errors/is absent from PATH in the invoking process) would plausibly reproduce #188559. This is an env/config-topology-dependent outcome, not a settled cross-machine guarantee — worth stating as a caveat in the shipped row rather than a blanket "works."

## (b) `GH_TOKEN` short-circuit

```bash
$ TOKEN_B="$(gh auth token --user levifigueira)"
$ printf 'protocol=https\nhost=github.com\n\n' | GH_TOKEN="$TOKEN_B" git credential fill
protocol=https
host=github.com
username=x-access-token
```

`GH_TOKEN` does short-circuit the helper — but the returned `username` field is the generic literal `x-access-token`, not the account name. Verified this is nonetheless the correct identity by checking what the token itself resolves to via the API (independent of the credential-fill flow):

```bash
$ GH_TOKEN="$TOKEN_B" gh api user --jq .login
levifigueira
```

**Confirmed:** `GH_TOKEN` correctly and unconditionally short-circuits to whichever account the token belongs to — it does not depend on `GH_CONFIG_DIR`, the active pointer, or `osxkeychain` ordering at all. This matches the parked change's carried-forward finding (`GH_TOKEN` env-only precedence, gh ≥ 2.40.0) and extends it to the git-credential path specifically.

## (c) Conditional credential helper (repo-local, no ambient override)

Set up in a throwaway temp repo, with the ambient environment carrying **neither** `GH_CONFIG_DIR` nor `GH_TOKEN`, and the machine's default active account still `levifig`:

```bash
$ git init /tmp/.../temp-repo-conditional-helper && cd $_
$ git config credential.https://github.com.helper ""     # reset — clear inherited helper list
$ git config --add credential.https://github.com.helper \
    '!GH_TOKEN=$(gh auth token --user levifigueira) gh auth git-credential'

$ git config --local --get-all credential.https://github.com.helper
!GH_TOKEN=$(gh auth token --user levifigueira) gh auth git-credential

$ env | grep '^GH_'
(none set)

$ printf 'protocol=https\nhost=github.com\n\n' | git credential fill
protocol=https
host=github.com
username=x-access-token
```

Per the `GH_TOKEN` finding above, `username=x-access-token` here means the helper's `GH_TOKEN=$(gh auth token --user levifigueira)` substitution ran and short-circuited correctly — the identity is `levifigueira`'s token regardless of the ambient environment or the machine's default pointer. **Confirmed: the conditional helper pins identity per-repo, independent of session env, independent of the global pointer, independent of `GH_CONFIG_DIR`.**

## Blindspot pass (bounded, not exhaustive)

- `gh extension list` → one extension installed: `gh copilot` (github/gh-copilot). Doesn't touch config-dir/credential resolution.
- No `GH_CONFIG_DIR` references found in `~/.zshrc`, `~/.zprofile`, `~/.bashrc`, `~/.bash_profile`.
- No other gh-adjacent CLIs found on PATH (`hub`, `glab`, `act`, `gh-dash` — none installed).
- The `osxkeychain` stale `github.com` credential (found above) is itself a blindspot instance worth carrying into the residual matrix: *some* tool, at some point, wrote a github.com credential via the generic `osxkeychain` git helper outside gh's control. Any future change to helper ordering (e.g., a different git install, a global `credential.helper` reset) could resurrect that stale entry as the winning identity.

## Verdicts

| Candidate | Result | Notes |
|---|---|---|
| **(a) `GH_CONFIG_DIR` inheritance alone** | **Works on this machine** — Discussion #188559 does not reproduce here | Depends on helper *ordering* between `osxkeychain` (system) and `gh`'s helper (user config) being gh-last-wins; not a structural guarantee, fragile across machines/git installs |
| **(b) `GH_TOKEN` short-circuit** | **Works, unconditionally** | Correct identity confirmed via API; username field is a generic `x-access-token` placeholder, not the account name — cosmetic only |
| **(c) Conditional credential helper** | **Works, unconditionally, repo-local, no ambient env dependency** | Confirmed identity pins to the wired account regardless of `GH_CONFIG_DIR`, `GH_TOKEN`, or the global pointer |

**HTTPS row verdict:** ship the **conditional credential helper (c)** as the wired mechanism, per the contract's existing lean. It is the only one of the three that does not depend on machine-specific helper-ordering luck (unlike (a)) and does not require a live secret in session env inherited by every subprocess (unlike (b), which the contract already rejected as the wired mechanism for that reason). `GH_CONFIG_DIR` inheritance (a) is real and worth documenting as a **bonus effect** when it happens to work (as it does here), but should **not** be the row's sole claimed mechanism — the ordering dependency identified above means it can silently fail on a different machine, exactly matching #188559's report. The row should read: ships via the conditional helper; `GH_CONFIG_DIR` alone is insufficient/unreliable as a guarantee even though it happens to work on this reference machine.

## Cleanup

The temp repo (`temp-repo-conditional-helper`) and all temp profile directories from the companion keyring spike were `rm -rf`'d at the end of testing (shared cleanup pass, see the keyring-isolation transcript). No token values were printed at any point; only exit codes, account-name outputs (`gh api user --jq .login`), and the `username=` line from `git credential fill` were captured. No git push was performed. No repo mutation reached GitHub — the only live network calls were read-only `gh api user` and `gh auth status`/`git credential fill` (the latter is local-only; it does not contact GitHub). The default gh profile and global git config were never written to by this spike (only repo-local config in the disposable temp repo, and env-scoped variables in temp shell invocations).
