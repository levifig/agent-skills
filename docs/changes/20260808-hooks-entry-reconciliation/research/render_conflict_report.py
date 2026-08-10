#!/usr/bin/env python3
"""Render hook-entry-classification.json as an adjudication report (HTML).

Every non-in-sync entry gets a stable ID (X* codex, L* cursor legacy,
F* cursor third-party) so rulings can be given one by one in conversation.
Reads the classifier output beside itself; writes hook-conflict-report.html.
"""

import html
import json
import os
import datetime

HERE = os.path.dirname(os.path.abspath(__file__))
HOME = os.path.expanduser("~")

DUPLICATES = {
    "foundations-check-secrets.sh": "loaf check --hook check-secrets",
    "foundations-security-audit.sh": "loaf check --hook security-audit",
    "foundations-validate-push.sh": "loaf check --hook validate-push --advisory",
    "orchestration-validate-commit.py": "loaf check --hook validate-commit",
    "orchestration-detect-linear-magic.py": "loaf journal log --detect-linear",
}

FAMILIES = ["foundations", "orchestration", "python", "rails", "typescript", "infra", "design"]


def esc(s):
    return html.escape(str(s), quote=True)


def script_path(cmd):
    for tok in cmd.replace("'", " ").split():
        if "/" in tok:
            return tok.replace("$HOME", HOME)
    return None


def script_meta(cmd):
    p = script_path(cmd)
    if not p:
        return None, None
    if not os.path.exists(p):
        return p, "missing"
    return p, datetime.datetime.fromtimestamp(os.stat(p).st_mtime).strftime("%Y-%m-%d")


def family_of(cmd):
    name = os.path.basename(script_path(cmd) or "")
    for fam in FAMILIES:
        if name.startswith(fam):
            return fam
    return "other"


def json_block(obj):
    return f'<pre class="code">{esc(json.dumps(obj, indent=2))}</pre>'


def main():
    data = json.load(open(os.path.join(HERE, "hook-entry-classification.json")))
    rows = data["rows"]

    codex = [r for r in rows if r["target"] == "codex"]
    cursor_in_sync = [r for r in rows if r["target"] == "cursor" and r["disposition"] == "in-sync"]
    cursor_foreign = [r for r in rows if r["target"] == "cursor" and r["disposition"] == "foreign"]

    legacy, third_party = [], []
    for r in cursor_foreign:
        cmd = r["actual"].get("command", "")
        (legacy if family_of(cmd) != "other" else third_party).append(r)

    legacy.sort(key=lambda r: (FAMILIES.index(family_of(r["actual"]["command"])), r["event"],
                               os.path.basename(script_path(r["actual"]["command"]) or "")))

    # ---- build sections ----
    parts = []

    # Codex cards
    codex_cards = []
    xid = 0
    for r in sorted(codex, key=lambda r: r["disposition"]):  # deleted first
        xid += 1
        rid = f"X{xid}"
        if r["disposition"] == "deleted":
            codex_cards.append(f"""
<article class="card" id="{rid}">
  <header><span class="rid">{rid}</span><span class="chip chip-deleted">deleted</span>
    <span class="ev">SessionStart</span></header>
  <p>The Loaf journal-context entry is absent from the live file — removed 2026-08-02, journaled as
     deliberate disable-intent (<code>journal:dcf9875d</code>). This is the entry whose absence makes
     every <code>loaf upgrade</code> refuse the whole file today.</p>
  <p class="label">Entry Loaf would ship (0.2.20 dist)</p>
  {json_block(r["desired"])}
  <p class="ruling">Proposed ruling: <strong>absorb as disabled</strong> — record hook-enablement
     state <em>disabled</em>, never re-add without an explicit <code>loaf hooks enable</code>.</p>
</article>""")
        else:
            codex_cards.append(f"""
<article class="card" id="{rid}">
  <header><span class="rid">{rid}</span><span class="chip chip-foreign">foreign</span>
    <span class="ev">{esc(r["event"])}</span></header>
  <p>Third-party entry (herdr, script dated 2026-08-07). Under the entry model this is untouchable
     by construction — reconciliation never counts it against Loaf's digest again.</p>
  {json_block(r["actual"])}
  <p class="ruling">Proposed ruling: <strong>keep</strong> — operator-owned, invisible to Loaf.</p>
</article>""")
    parts.append(f"""
<section>
  <p class="eyebrow">Codex · ~/.codex/hooks.json</p>
  <h2>Two entries, two rulings</h2>
  {''.join(codex_cards)}
</section>""")

    # Cursor third-party
    f_cards = []
    for i, r in enumerate(third_party, 1):
        rid = f"F{i}"
        f_cards.append(f"""
<article class="card" id="{rid}">
  <header><span class="rid">{rid}</span><span class="chip chip-foreign">foreign</span>
    <span class="ev">{esc(r["event"])}</span></header>
  {json_block(r["actual"])}
  <p class="ruling">Proposed ruling: <strong>keep</strong> — third-party (herdr), untouchable by construction.</p>
</article>""")

    # Cursor legacy table rows, grouped by family
    fam_sections = []
    lid = 0
    for fam in FAMILIES:
        fam_rows = [r for r in legacy if family_of(r["actual"]["command"]) == fam]
        if not fam_rows:
            continue
        trs = []
        for r in fam_rows:
            lid += 1
            hook = r["actual"]
            cmd = hook.get("command", "")
            name = os.path.basename(script_path(cmd) or "")
            _, mtime = script_meta(cmd)
            dup = DUPLICATES.get(name)
            dup_html = (f'<span class="dup">duplicates <code>{esc(dup)}</code></span>'
                        if dup else '<span class="nodup">—</span>')
            trs.append(f"""
<tr id="L{lid:02}">
  <td class="rid-cell">L{lid:02}</td>
  <td class="ev">{esc(r["event"])}</td>
  <td><code>{esc(cmd)}</code></td>
  <td class="ev">{esc(hook.get("matcher", ""))}</td>
  <td>{dup_html}</td>
  <td class="num">{esc(mtime or "?")}</td>
</tr>""")
        fam_sections.append(f"""
<h3>{esc(fam)} <span class="count">{len(fam_rows)}</span></h3>
<div class="scroll"><table>
  <thead><tr><th>ID</th><th>Event</th><th>Command</th><th>Matcher</th><th>Overlap with shipped entry</th><th>Script date</th></tr></thead>
  <tbody>{''.join(trs)}</tbody>
</table></div>""")

    in_sync_lis = "".join(
        f'<li><span class="ev">{esc(r["event"])}</span> <code>{esc(r["actual"].get("command", r["actual"].get("prompt", "?")))}</code></li>'
        for r in cursor_in_sync)

    parts.append(f"""
<section>
  <p class="eyebrow">Cursor · ~/.cursor/hooks.json</p>
  <h2>Shipped entries are pristine; a 2026-03-25 generation rides along</h2>
  <p>All <strong>17</strong> Loaf-shipped entries match 0.2.20 byte-for-byte — no deleted, no modified.
     The remaining 33 are unclaimed: one is genuinely third-party, and <strong>32</strong> form a single
     installation event (scripts dated 2026-03-25, one 2026-04-07) whose names map one-to-one onto Loaf
     skill families. The current legacy-recognition maps do not claim them, so today Loaf treats them as
     foreign and they run in every Cursor session. Five functionally duplicate a shipped
     <code>loaf check</code> entry — double enforcement on every matching tool call.</p>
  <details><summary>The 17 in-sync entries (no action needed)</summary><ul class="insync">{in_sync_lis}</ul></details>
  {''.join(f_cards)}
  <h2 class="h2-tight">The legacy generation, by family</h2>
  <p class="ruling">Proposed rulings per row: <strong>retire</strong> (claim as legacy Loaf; reconciliation
     removes entry and script) · <strong>keep</strong> (operator-owned; becomes foreign-by-construction,
     never touched) · <strong>disable</strong> (claim, but record disabled instead of removing).</p>
  {''.join(fam_sections)}
</section>""")

    total_chips = f"""
<div class="chips">
  <span class="chip chip-sync">17 in-sync</span>
  <span class="chip chip-deleted">1 deleted</span>
  <span class="chip chip-legacy">32 legacy-suspect</span>
  <span class="chip chip-foreign">2 third-party</span>
  <span class="chip chip-plain">0 modified</span>
</div>"""

    doc = f"""<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>Hooks Reconciliation — Live Conflict Report</title>
<style>
:root {{
  --bg: #FAFAF8; --surface: #FFFFFF; --ink: #23211C; --muted: #6F6B60;
  --line: #E4E1D8; --accent: #B4690E;
  --sync: #2E7D4F; --sync-bg: #E6F2EA;
  --del: #B03A48; --del-bg: #F7E8EA;
  --legacy: #9A6A00; --legacy-bg: #F5EDDB;
  --foreign: #4A5E8A; --foreign-bg: #E9EDF5;
  --code-bg: #F2F0EA;
}}
@media (prefers-color-scheme: dark) {{
  :root:not([data-theme="light"]) {{
    --bg: #1B1917; --surface: #232019; --ink: #EDEAE4; --muted: #A29D90;
    --line: #38342C; --accent: #E09A3E;
    --sync: #7CC79A; --sync-bg: #24352B;
    --del: #E08B96; --del-bg: #3D272B;
    --legacy: #D9B25F; --legacy-bg: #383021;
    --foreign: #9FB2D9; --foreign-bg: #272E3D;
    --code-bg: #26231D;
  }}
}}
:root[data-theme="dark"] {{
  --bg: #1B1917; --surface: #232019; --ink: #EDEAE4; --muted: #A29D90;
  --line: #38342C; --accent: #E09A3E;
  --sync: #7CC79A; --sync-bg: #24352B;
  --del: #E08B96; --del-bg: #3D272B;
  --legacy: #D9B25F; --legacy-bg: #383021;
  --foreign: #9FB2D9; --foreign-bg: #272E3D;
  --code-bg: #26231D;
}}
* {{ box-sizing: border-box; }}
body {{
  margin: 0; background: var(--bg); color: var(--ink);
  font: 15px/1.6 -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif;
}}
main {{ max-width: 880px; margin: 0 auto; padding: 40px 24px 80px; }}
code, .code, .rid, .rid-cell {{ font-family: ui-monospace, "SF Mono", Menlo, monospace; }}
.eyebrow {{
  font-size: 11px; letter-spacing: 0.14em; text-transform: uppercase;
  color: var(--accent); font-weight: 650; margin: 40px 0 4px;
}}
h1 {{ font-size: 26px; line-height: 1.25; margin: 4px 0 8px; text-wrap: balance; }}
h2 {{ font-size: 19px; margin: 8px 0 12px; text-wrap: balance; }}
.h2-tight {{ margin-top: 36px; }}
h3 {{ font-size: 14px; margin: 28px 0 8px; text-transform: uppercase; letter-spacing: 0.08em; color: var(--muted); }}
h3 .count {{ color: var(--accent); margin-left: 6px; }}
.meta {{ color: var(--muted); font-size: 13.5px; margin: 0 0 16px; }}
.meta code {{ font-size: 12.5px; }}
.chips {{ display: flex; flex-wrap: wrap; gap: 8px; margin: 20px 0 8px; }}
.chip {{
  display: inline-block; padding: 2px 10px; border-radius: 999px;
  font-size: 12.5px; font-weight: 600; font-variant-numeric: tabular-nums;
}}
.chip-sync {{ background: var(--sync-bg); color: var(--sync); }}
.chip-deleted {{ background: var(--del-bg); color: var(--del); }}
.chip-legacy {{ background: var(--legacy-bg); color: var(--legacy); }}
.chip-foreign {{ background: var(--foreign-bg); color: var(--foreign); }}
.chip-plain {{ background: var(--code-bg); color: var(--muted); }}
.card {{
  background: var(--surface); border: 1px solid var(--line); border-radius: 8px;
  padding: 16px 20px; margin: 16px 0;
}}
.card header {{ display: flex; align-items: center; gap: 10px; margin-bottom: 8px; }}
.rid {{ font-weight: 700; font-size: 14px; color: var(--accent); }}
.rid-cell {{ font-weight: 700; color: var(--accent); white-space: nowrap; }}
.ev {{ color: var(--muted); font-size: 12.5px; font-family: ui-monospace, "SF Mono", Menlo, monospace; }}
.label {{
  font-size: 11px; letter-spacing: 0.1em; text-transform: uppercase;
  color: var(--muted); margin: 14px 0 4px; font-weight: 650;
}}
.code {{
  background: var(--code-bg); border: 1px solid var(--line); border-radius: 6px;
  padding: 12px 14px; font-size: 12.5px; line-height: 1.5; overflow-x: auto; margin: 6px 0;
}}
.ruling {{ font-size: 13.5px; color: var(--muted); }}
.ruling strong {{ color: var(--ink); }}
.scroll {{ overflow-x: auto; border: 1px solid var(--line); border-radius: 8px; background: var(--surface); }}
table {{ border-collapse: collapse; width: 100%; font-size: 13px; }}
th {{
  text-align: left; font-size: 11px; letter-spacing: 0.08em; text-transform: uppercase;
  color: var(--muted); padding: 10px 12px; border-bottom: 1px solid var(--line); white-space: nowrap;
}}
td {{ padding: 8px 12px; border-bottom: 1px solid var(--line); vertical-align: top; }}
tbody tr:last-child td {{ border-bottom: none; }}
td code {{ font-size: 12px; }}
.num {{ font-variant-numeric: tabular-nums; white-space: nowrap; color: var(--muted); }}
.dup {{ color: var(--del); font-size: 12.5px; font-weight: 600; }}
.dup code {{ font-weight: 400; }}
.nodup {{ color: var(--muted); }}
details {{ margin: 14px 0; }}
summary {{ cursor: pointer; color: var(--accent); font-weight: 600; font-size: 13.5px; }}
summary:focus-visible {{ outline: 2px solid var(--accent); outline-offset: 2px; }}
.insync {{ list-style: none; padding: 10px 0 0 4px; margin: 0; }}
.insync li {{ padding: 3px 0; font-size: 13px; }}
section + section {{ border-top: 1px solid var(--line); margin-top: 44px; }}
a {{ color: var(--accent); }}
</style>
</head>
<body>
<main>
  <p class="eyebrow">Loaf · Change 20260808-hooks-entry-reconciliation</p>
  <h1>Live hook-entry conflict report</h1>
  <p class="meta">Every entry in <code>~/.codex/hooks.json</code> and <code>~/.cursor/hooks.json</code> on this
     machine, classified against the 0.2.20 dist (<code>main@4edd80d6</code>) with the same ownership predicates
     <code>loaf upgrade</code> uses. Rule on items by ID — <code>X1</code>, <code>F1</code>, <code>L01</code>…</p>
  {total_chips}
  <p class="meta">No <em>modified</em> Loaf entry exists anywhere today — the mutation-semantics question
     stays a policy call, not a live conflict.</p>
  {''.join(parts)}
</main>
</body>
</html>"""

    out = os.path.join(HERE, "hook-conflict-report.html")
    with open(out, "w") as f:
        f.write(doc)
    print(f"wrote {out} ({len(doc)} bytes, last ID L{lid:02})")


if __name__ == "__main__":
    main()
