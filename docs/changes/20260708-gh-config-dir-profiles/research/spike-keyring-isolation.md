# Spike: Keyring Isolation Across `GH_CONFIG_DIR` Profiles

**Date:** 2026-07-08
**Machine:** macOS, `gh` 2.96.0, Homebrew git, login.keychain-db
**Open question addressed:** "Keyring isolation mechanics: how does gh's secure storage key entries across config dirs — per-dir, or per host+user shared in the OS keychain?"

## Setup

Two accounts already authenticated in the default profile (`~/.config/gh`, untouched throughout): `levifig` (active) and `levifigueira` (inactive). Three temp profiles created under a session-scratchpad directory (deleted at the end of this spike), each authenticated via `gh auth login --with-token` fed from `gh auth token --user <name>` against the *default* profile — no interactive/browser login used anywhere.

```bash
PROFILE_A=<tmp>/profile-A-levifig            # levifig
PROFILE_B=<tmp>/profile-B-levifigueira       # levifigueira
PROFILE_C=<tmp>/profile-C-levifig-second     # levifig again, SECOND profile for the same account

TOKEN_A="$(gh auth token --user levifig)"
TOKEN_B="$(gh auth token --user levifigueira)"
printf '%s' "$TOKEN_A" | GH_CONFIG_DIR="$PROFILE_A" gh auth login --hostname github.com --with-token
printf '%s' "$TOKEN_B" | GH_CONFIG_DIR="$PROFILE_B" gh auth login --hostname github.com --with-token
printf '%s' "$TOKEN_A" | GH_CONFIG_DIR="$PROFILE_C" gh auth login --hostname github.com --with-token
```

All three logins exited 0.

## (a) Does each profile resolve its own account?

```bash
$ GH_CONFIG_DIR="$PROFILE_A" gh api user --jq .login
levifig
$ GH_CONFIG_DIR="$PROFILE_B" gh api user --jq .login
levifigueira
$ GH_CONFIG_DIR="$PROFILE_C" gh api user --jq .login
levifig
```

`gh auth status` under each `GH_CONFIG_DIR` reports the correct account as `Active account: true`. **Yes — for gh's own commands (`api`, `status`, and by extension `pr`/`issue`/etc.), the config dir's `hosts.yml` "active user" pointer correctly scopes identity.**

## (b) Where do credentials actually live?

`hosts.yml` in each temp profile (oauth_token pattern redacted defensively; none was present — storage is keyring, not file):

```
# PROFILE_A/hosts.yml
github.com:
    users:
        levifig:
    user: levifig

# PROFILE_B/hosts.yml
github.com:
    users:
        levifigueira:
    user: levifigueira

# PROFILE_C/hosts.yml
github.com:
    users:
        levifig:
    user: levifig
```

Each `hosts.yml` is a **pointer file**: it names which account is "active" for that config dir. It contains no secret material — confirmed via `gh auth status` reporting `(keyring)` next to each login, not `(file)`.

**Control test — does `--user` actually consult the keyring, or does it silently ignore the flag?**

```bash
$ GH_CONFIG_DIR="$PROFILE_A" gh auth token --user this-account-does-not-exist-xyz
no oauth token found for github.com account this-account-does-not-exist-xyz
exit: 1
```

Confirms `--user` does a real keyed lookup (fails cleanly for an unknown account) rather than ignoring the flag.

**The critical test — does Profile A (only ever authenticated as `levifig`) leak `levifigueira`'s token?**

```bash
$ GH_CONFIG_DIR="$PROFILE_A" gh auth token --user levifigueira >/dev/null 2>&1; echo $?
0
```

Exit 0. Profile A's `hosts.yml` lists **only** `levifig` under `users:` — `levifigueira` is not present in that file — yet `gh auth token --user levifigueira` succeeds from inside Profile A's config dir. Byte-for-byte confirmation:

```bash
$ [ "$(gh auth token --user levifigueira)" = "$(GH_CONFIG_DIR="$PROFILE_A" gh auth token --user levifigueira)" ] && echo "IDENTICAL token value"
IDENTICAL token value
```

**Mechanism:** `gh auth token --user <name>` does not consult the config dir's `hosts.yml` at all — it queries the OS keychain directly by `(service, account)`. The keychain is machine-global and keyed by **host + username**, not by config directory.

**Keychain inspection (metadata only — service and account names, never secret values):**

Baseline, before any temp-profile activity, `login.keychain-db` already had 3 entries with `svce = "gh:github.com"`:

```
acct="levifigueira"
acct="levifig"
acct=<NULL>          # pre-existing, unrelated to this spike, untouched throughout
```

After creating and authenticating Profiles A, B, and C:

```
acct="levifigueira"  created=20251208131618  modified=20260708003200
acct="levifig"        created=20240307183508  modified=20260708003201
acct=<NULL>            (unrelated entry, present before this spike)
```

Count of `gh:github.com` entries: **3 before, 3 after** — unchanged. Profile A's and Profile C's logins (both `levifig`) and Profile B's login (`levifigueira`) each **updated `mdat` on the same two pre-existing entries** rather than creating new ones. No new keychain item was created per config dir.

## (c) Do two profiles for the same account coexist?

Yes, functionally — Profile A and Profile C both correctly resolve `levifig`'s identity via `gh api user`. But "coexist" understates it: they are not two independent credentials that happen to agree — they are **two pointers into the exact same keychain entry**. There is only one `levifig` secret on the machine; every config dir that points to `user: levifig` reads and (on login) rewrites that single shared item.

## Verdict

**Isolation is not real at the credential-storage layer. It is real only at the pointer layer.**

- `GH_CONFIG_DIR` profiles are directories containing a `hosts.yml` that names which account is "active" for that directory (a pointer, not a credential store).
- The actual secret material lives in exactly one place on the machine: the OS keychain, in an item keyed by `(service = "gh:<host>", account = "<username>")`. This store is **shared globally** — not partitioned by `GH_CONFIG_DIR`, not partitioned by which profile created the login.
- Commands that read the *active* account (`gh api`, `gh auth status`, and — per the companion HTTPS spike — `gh auth git-credential`) are correctly scoped, because they resolve through the config dir's `hosts.yml` pointer first.
- Commands that accept an explicit `--user` (or, per the companion spike, `GH_TOKEN`) **bypass the config dir's pointer entirely** and can retrieve *any* account's token that has ever been authenticated on the machine, from *any* profile — this is a deliberate gh feature (multi-account convenience), not a bug, but it means "profile" is a UX/routing concept, not a security boundary. Any process that can invoke `gh` with `--user <name>` already has access to that account's token regardless of `GH_CONFIG_DIR`.
- This directly explains the mechanism *class* behind Discussion #188559 even before the HTTPS spike ran it down: gh's own identity-scoping depends entirely on which pointer a given code path consults, and any path that doesn't consult the active-account pointer (or that consults a *different* store, like a coexisting `osxkeychain` git credential — see the HTTPS spike) will diverge from the profile's intended identity.

**Practical implication for the Hypothesis:** the "no shared mutable state contested between sessions" claim holds for the *pointer* (each config dir's `hosts.yml` is a private file, genuinely uncontested), but does **not** hold for the underlying secret store, which is one shared machine-wide keychain. Two concurrent sessions wired to different profiles will not race on file writes to `hosts.yml`, but they are reading from — and, on any `gh auth login`, writing to — the same keychain entries. This is a nuance the contract's "no shared mutable state" framing should carry: it's true for config-file mutation races (the actual collision cost #99 discloses), not true in an absolute sense about credential storage.

## Cleanup

All three temp profile directories were `rm -rf`'d at the end of the spike (verified empty afterward, only the setup script remains under the session scratchpad). No token values were printed anywhere in this transcript or in the terminal at any point; only exit codes, lengths, equality checks, and account-name outputs from `gh api user --jq .login` were captured. The default profile (`~/.config/gh`) was read-only throughout: `gh auth status` before and after this spike shows identical state (`levifig` active, `levifigueira` present and inactive, same scopes). One observed side effect: `~/.config/gh/hosts.yml`'s mtime advanced during the spike (gh appears to rewrite/normalize the file on some read paths, e.g. via the `--user` control test run against the default profile) — content is byte-identical in substance (same accounts, same active pointer, cosmetic YAML formatting only: `levifig: {}` vs `levifigueira:`), confirmed by direct comparison before and after. No logout, no account switch, no `hosts.yml` edit was performed by this spike.
