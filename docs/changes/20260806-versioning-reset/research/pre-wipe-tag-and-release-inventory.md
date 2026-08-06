---
source: docs/changes/20260806-versioning-reset/shape.md
recorded: 2026-08-06T13:50:38Z
remote: https://github.com/levifig/loaf.git
---

# Pre-wipe tag and Release inventory

Recorded before the versioning reset deletes the pre-reset tag surface. The deletion is irreversible and takes every GitHub Release with it, so this is the record that keeps the old numbering resolvable to commits afterwards: the changelog carries the renumbered history, and this file is what maps a citation of `v2.0.0-alpha.16` in a pre-reset ADR onto the commit it actually named.

Enumerated with `git ls-remote --tags origin` and `gh release list --limit 200` against `levifig/loaf` at `192dc608`.

## Totals

| | Count |
|---|---|
| Remote tags | 55 |
| GitHub Releases | 43 (all prerelease-flagged, none draft) |
| Tags carrying no Release | 12 |
| Releases with no remote tag | 0 |

Everything below is deleted by the wipe. The tag surface that survives is `v0.1.0` — a lightweight marker planted at `c7e7eb9d`, the commit `v1.17.4` points to, with no packaged Release — and `v0.2.20`, which the release ceremony creates before the wipe runs.

## Inventory

`Renumbered` applies the ADR-026 map; it names the changelog heading each tag's release now lives under, not any tag that exists or will exist. `Commit` is the peeled target, so an annotated tag resolves to the commit rather than to its tag object.

| Tag | Renumbered | Commit | Kind | Release published |
|---|---|---|---|---|
| `v1.17.4` | 0.1.0 (era) | `c7e7eb9d` | lightweight | — |
| `v2.0.0-dev.16` | 0.1.16 | `756de34a` | lightweight | — |
| `v2.0.0-dev.17` | 0.1.17 | `9ffef5ce` | annotated | — |
| `v2.0.0-dev.18` | 0.1.18 | `d891bc06` | lightweight | — |
| `v2.0.0-dev.19` | 0.1.19 | `675ba3dc` | lightweight | — |
| `v2.0.0-dev.20` | 0.1.20 | `a4a5b178` | lightweight | — |
| `v2.0.0-dev.22` | 0.1.22 | `47c5f400` | lightweight | — |
| `v2.0.0-dev.23` | 0.1.23 | `529eeb8a` | lightweight | — |
| `v2.0.0-dev.24` | 0.1.24 | `d00d3caa` | annotated | 2026-04-09 |
| `v2.0.0-dev.25` | 0.1.25 | `bc295ceb` | annotated | 2026-04-09 |
| `v2.0.0-dev.26` | 0.1.26 | `3cddae74` | annotated | 2026-04-10 |
| `v2.0.0-dev.27` | 0.1.27 | `bff3b637` | annotated | 2026-04-11 |
| `v2.0.0-dev.28` | 0.1.28 | `51e9d0e0` | annotated | 2026-04-22 |
| `v2.0.0-dev.29` | 0.1.29 | `9324c147` | annotated | 2026-04-22 |
| `v2.0.0-dev.30` | 0.1.30 | `23dd4a85` | annotated | 2026-04-24 |
| `v2.0.0-dev.31` | 0.1.31 | `60b3a86d` | annotated | 2026-04-28 |
| `v2.0.0-dev.32` | 0.1.32 | `74814a02` | annotated | 2026-04-29 |
| `v2.0.0-dev.33` | 0.1.33 | `0ab428db` | annotated | 2026-04-30 |
| `v2.0.0-dev.34` | 0.1.34 | `da687b8f` | annotated | 2026-04-30 |
| `v2.0.0-dev.35` | 0.1.35 | `ce627c81` | lightweight | 2026-04-30 |
| `v2.0.0-dev.36` | 0.1.36 | `6e79efcf` | annotated | 2026-04-30 |
| `v2.0.0-dev.37` | 0.1.37 | `d753d30d` | annotated | 2026-05-02 |
| `v2.0.0-dev.38` | 0.1.38 | `c51785d4` | lightweight | 2026-05-02 |
| `v2.0.0-dev.39` | 0.1.39 | `5cf95a34` | lightweight | 2026-05-02 |
| `v2.0.0-dev.40` | 0.1.40 | `0ba8fc3e` | lightweight | 2026-05-02 |
| `v2.0.0-dev.42` | 0.1.42 | `16676aa5` | annotated | 2026-05-19 |
| `v2.0.0-dev.43` | 0.1.43 | `eeb60af3` | annotated | 2026-05-22 |
| `v2.0.0-dev.44` | 0.1.44 | `4e178b3e` | annotated | 2026-05-22 |
| `v2.0.0-dev.45` | 0.1.45 | `62cc8051` | annotated | 2026-05-27 |
| `v2.0.0-dev.46` | 0.1.46 | `56925695` | annotated | 2026-05-28 |
| `v2.0.0-dev.47` | 0.1.47 | `61a31689` | annotated | 2026-05-28 |
| `v2.0.0-dev.49` | 0.1.49 | `8c3d156b` | annotated | 2026-05-31 |
| `v2.0.0-pre.20260614235428` | 0.1.50 | `93196d72` | lightweight | — |
| `v2.0.0-pre.20260625183349` | 0.1.51 | `7ca63bb2` | annotated | — |
| `v2.0.0-pre.20260625190923` | 0.1.52 | `f91c18a5` | annotated | — |
| `v2.0.0-pre.20260625192947` | 0.1.53 | `784c67b1` | annotated | — |
| `v2.0.0-alpha.1` | 0.2.1 | `c420d41e` | annotated | 2026-06-27 |
| `v2.0.0-alpha.2` | 0.2.2 | `6e27c77a` | annotated | 2026-07-04 |
| `v2.0.0-alpha.3` | 0.2.3 | `98fa01ce` | annotated | 2026-07-04 |
| `v2.0.0-alpha.4` | 0.2.4 | `e693b126` | annotated | 2026-07-06 |
| `v2.0.0-alpha.5` | 0.2.5 | `6cb192da` | annotated | 2026-07-06 |
| `v2.0.0-alpha.6` | 0.2.6 | `00cd974a` | annotated | 2026-07-12 |
| `v2.0.0-alpha.7` | 0.2.7 | `8994fb8f` | annotated | 2026-07-18 |
| `v2.0.0-alpha.8` | 0.2.8 | `c8d5f51a` | annotated | 2026-07-18 |
| `v2.0.0-alpha.9` | 0.2.9 | `1cd99763` | annotated | 2026-07-18 |
| `v2.0.0-alpha.10` | 0.2.10 | `43ac6a12` | annotated | 2026-07-19 |
| `v2.0.0-alpha.11` | 0.2.11 | `48582fc1` | annotated | 2026-07-19 |
| `v2.0.0-alpha.12` | 0.2.12 | `6951d0d3` | annotated | 2026-07-20 |
| `v2.0.0-alpha.13` | 0.2.13 | `9526b811` | annotated | 2026-07-24 |
| `v2.0.0-alpha.14` | 0.2.14 | `0fa87c8d` | annotated | 2026-07-25 |
| `v2.0.0-alpha.15` | 0.2.15 | `e94a24ba` | annotated | 2026-07-28 |
| `v2.0.0-alpha.16` | 0.2.16 | `87b0a67c` | annotated | 2026-07-30 |
| `v2.0.0-alpha.17` | 0.2.17 | `8174a30e` | annotated | 2026-07-30 |
| `v2.0.0-alpha.18` | 0.2.18 | `926145df` | annotated | 2026-08-01 |
| `v2.0.0-alpha.19` | 0.2.19 | `3ea263d9` | annotated | 2026-08-02 |

## Asymmetries worth keeping

The tag surface and the changelog were never a 1:1 pair, and the wipe is the last moment either can be checked against the other.

- Fifteen changelog headings have no remote tag: `0.1.1`–`0.1.9`, `0.1.11`–`0.1.15`, and `0.1.48`. Tagging began at `v2.0.0-dev.16`.
- One tag has no changelog heading: `v2.0.0-dev.25` (`bc295ceb`), which does carry a published GitHub Release. Its entry was missing before the renumbering and the gap was preserved as found.
- The dev sequence skips 10, 21, and 41 in both surfaces.
- `v1.17.4` and the four `pre.*` builds were tagged without Releases, as were `v2.0.0-dev.16`–`dev.20`, `dev.22`, and `dev.23`.
