# Consent-prompt drafts — `loaf shim enable gh`

Reaction artifact for the [UK] fog entry ("Consent-prompt and stderr-note
wording — recognize-on-sight"). Three drafts, all built against the same
real values a machine with gh at `/opt/homebrew/bin/gh` and zsh as the login
shell would see. H1's bar: *a reviewer who reads only the prompt can predict
every filesystem/profile mutation enable performs.*

## Contents
- Draft A — Itemized checklist (implemented)
- Draft B — Narrative
- Draft C — Diff-style
- Why A

## Draft A — Itemized checklist (implemented)

```
loaf shim enable gh will make these changes on this machine:

  1. Create symlink (replacing any existing entry at this path):
       /Users/levifig/.local/share/loaf/shims/gh -> /opt/homebrew/bin/loaf

  2. Record in /Users/levifig/.config/loaf/config.json:
       shims.gh.real_path = "/opt/homebrew/bin/gh"
       shims.gh.enabled_at = <now, UTC>

  3. Offer to add this line to your shell profile (/Users/levifig/.zshrc):
       export PATH="/Users/levifig/.local/share/loaf/shims:$PATH"

From then on, `gh` resolves to this symlink first. Inside a Loaf project with
integrations.github.account configured, it fetches a per-invocation token for
that named account from gh's own keychain and execs the real gh with it. Every-
where else it execs the real gh untouched. gh's own config and active-account
pointer (~/.config/gh/hosts.yml) are never written by the shim — only read.

Proceed? [y/N]
```

## Draft B — Narrative

```
Here's what enabling the gh shim does, in plain terms:

Right now, typing `gh` runs /opt/homebrew/bin/gh directly. After you say yes,
loaf will:
  - Symlink ~/.local/share/loaf/shims/gh to this loaf binary
  - Ask to put that shims folder at the front of your PATH (in ~/.zshrc)
  - Remember the real gh's location in ~/.config/loaf/config.json

Once that's done, every `gh` you or an agent types passes through loaf first.
Inside a project with a configured GitHub account, loaf fetches that
account's token from gh's own keychain and hands it to the real gh via
GH_TOKEN — nothing shared, nothing to race. Everywhere else, gh runs exactly
as it does today.

You can undo all of this with `loaf shim disable gh`.

Continue? [y/N]
```

## Draft C — Diff-style

```
loaf shim enable gh — mutations to apply:

  FILE   ~/.local/share/loaf/shims/gh   (new symlink -> loaf binary)
  FILE   ~/.config/loaf/config.json     (add shims.gh entry)
  FILE   ~/.zshrc                       (append 1 PATH line, if you agree)

  No other files change. gh's own state (~/.config/gh/hosts.yml, keychain
  entries) is read-only to the shim and never written.

  Effect: `gh` on PATH becomes this symlink; unconfigured directories are
  unaffected, configured Loaf projects get per-invocation GH_TOKEN injection.

Apply these changes? [y/N]
```

## Why A

All three pass H1 — each is auditable from the prompt text alone. They
differ in what a reviewer has to *do* with that text:

- **B** reads best on first encounter but interleaves the mechanical facts
  (paths, the config key, the PATH line) inside prose sentences. A reviewer
  checking "did it say exactly where the symlink goes" has to parse a
  paragraph to extract it.
- **C** is the most literally diff-like and fastest to audit line-by-line,
  but reads as terse and mechanical for what is, for most users, a
  once-per-machine trust decision — it under-explains the *effect* (why this
  is safe) in favor of the *mechanism* (what files change).
- **A** keeps the numbered mutations scannable and separable from the
  explanatory paragraph that follows them, so it satisfies H1 as directly as
  C while still giving the "why this is safe" context B provides — without
  making the reviewer choose between the two. The explicit callout that
  `hosts.yml` is read-only, never written, answers the single question a
  security-literate user will ask first.

Implemented as `printShimConsentPrompt` in `internal/cli/shim.go`, with the
literal values substituted at runtime (resolved real gh path, resolved loaf
binary path, detected shell profile). The PATH-line offer (item 3) is itself
a second, separate y/n gate at confirmation time — declining it prints the
line for the user to add by hand rather than silently skipping it.
