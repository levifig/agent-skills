#!/usr/bin/env python3
"""Classify every entry in the live Codex/Cursor hooks.json against 0.2.20 dist.

Ports the ownership predicates from internal/cli/install_target.go
(isLoafInstallHookForOS, codexHookOwnershipForOS, installHookSignature) so the
classification matches what `loaf upgrade` would compute, then goes further:
per-entry disposition (in-sync / modified / deleted / stale-loaf / foreign)
with field-level diffs for the modified pairs.

Evidence generator for docs/changes/20260808-hooks-entry-reconciliation.
Read-only over the live files; writes hook-entry-classification.json beside itself.

PROVENANCE: this implements the 0.2.20 (pre-change) predicates and exists to
document the starting state. It is NOT the acceptance oracle for the new
entry-level model — before/after preservation is proven by
compare_hook_files.py (predicate-free), and new-model classification lives in
the Go implementation and its fixtures.
"""

import json
import os
import re
import sys

HERE = os.path.dirname(os.path.abspath(__file__))
REPO = os.path.abspath(os.path.join(HERE, "..", "..", "..", ".."))
HOME = os.path.expanduser("~")

CODEX_SUFFIX = " journal context --from-hook --codex-hook"
CODEX_MATCHER = "startup|resume|clear|compact"
LOAF_MARKER = "loaf-managed"

LEGACY_SIGNATURES = {
    "command:loaf check --hook check-secrets|matcher:Edit|Write|Bash|if:",
    "command:loaf check --hook security-audit|matcher:Bash|if:",
    "command:loaf check --hook validate-push|matcher:Bash|if:",
    "command:loaf check --hook workflow-pre-pr|matcher:Bash|if:",
    "command:loaf check --hook validate-commit|matcher:Bash|if:",
    "command:loaf task refresh|matcher:Edit|Write|if:",
    "command:bash $HOME/.cursor/hooks/post-tool/kb-staleness-nudge.sh|matcher:Edit|Write|if:",
    "command:loaf journal log --detect-linear|matcher:Bash|if:",
    "command:loaf journal log --from-hook|matcher:Bash|if:Bash(git commit:*)",
    "command:loaf journal log --from-hook|matcher:Bash|if:Bash(gh pr create:*)",
    "command:loaf journal log --from-hook|matcher:Bash|if:Bash(gh pr merge:*)",
    "command:loaf journal context|matcher:|if:",
    "command:loaf session log --detect-linear|matcher:Bash|if:",
    "command:loaf session log --from-hook|matcher:Bash|if:Bash(git commit:*)",
    "command:loaf session log --from-hook|matcher:Bash|if:Bash(gh pr create:*)",
    "command:loaf session log --from-hook|matcher:Bash|if:Bash(gh pr merge:*)",
    "command:loaf session start|matcher:|if:",
    "command:loaf session end|matcher:|if:",
    "command:bash $HOME/.cursor/hooks/session/compact.sh|matcher:|if:",
}

LEGACY_COMMANDS = {
    "bash $HOME/.cursor/hooks/session/session-start-soul.sh",
    "bash $HOME/.cursor/hooks/session/session-start.sh",
    "bash $HOME/.cursor/hooks/session/kb-session-start.sh",
    "bash $HOME/.cursor/hooks/session/session-end.sh",
    "bash $HOME/.cursor/hooks/session/kb-session-end.sh",
    "bash $HOME/.cursor/hooks/session/pre-compact-archive.sh",
}

LEGACY_PROMPT_PREFIXES = [
    "STOP. Before running gh pr merge",
    "ADVISORY: You are about to run `git push`",
    "KNOWLEDGE BASE:",
    "POST-MERGE HOUSEKEEPING:",
    "CONTEXT COMPACTION IMMINENT:",
    "SESSION JOURNAL NUDGE:",
]


def signature(hook):
    command = hook.get("command")
    prompt = hook.get("prompt")
    matcher = hook.get("matcher", "")
    condition = hook.get("if", "")
    if isinstance(command, str):
        return f"command:{command}|matcher:{matcher}|if:{condition}"
    if isinstance(prompt, str):
        return f"prompt:{prompt}|matcher:{matcher}|if:{condition}"
    handlers = hook.get("hooks")
    if isinstance(handlers, list):
        # Codex/Cursor matcher-group form: identity from matcher + handler
        # commands, with the executable path normalized so the dist
        # placeholder pairs with an installed resolved command.
        cmds = []
        for h in handlers:
            if isinstance(h, dict):
                c = str(h.get("command", ""))
                c = re.sub(r"^(\{\{LOAF_EXECUTABLE\}\}|'[^']*')(?= )", "<exe>", c)
                cmds.append(c)
        return f"group|matcher:{matcher}|cmds:{'||'.join(cmds)}"
    return ""


def codex_ownership(hook):
    """Returns (owned, conflict) — exact port of codexHookOwnershipForOS (darwin)."""
    matcher = hook.get("matcher", "")
    handlers = hook.get("hooks")
    if not isinstance(handlers, list):
        return False, False
    contains = any(
        isinstance(h, dict)
        and (
            CODEX_SUFFIX in str(h.get("command", ""))
            or CODEX_SUFFIX in str(h.get("commandWindows", ""))
        )
        for h in handlers
    )
    if not contains:
        return False, False
    if matcher != CODEX_MATCHER or len(hook) != 2 or len(handlers) != 1:
        return False, True
    handler = handlers[0]
    if not isinstance(handler, dict) or handler.get("type") != "command":
        return False, True
    command = handler.get("command")
    if not isinstance(command, str):
        return False, True
    if len(handler) != 2 or not command.endswith(CODEX_SUFFIX):
        return False, True
    return True, False


def cursor_is_loaf(hook):
    if hook.get(LOAF_MARKER) is True:
        return True
    sig = signature(hook)
    if sig and sig in LEGACY_SIGNATURES:
        return True
    if hook.get("command") in LEGACY_COMMANDS:
        return True
    prompt = hook.get("prompt")
    if isinstance(prompt, str) and any(prompt.startswith(p) for p in LEGACY_PROMPT_PREFIXES):
        return True
    owned, conflict = codex_ownership(hook)
    return owned and not conflict


def command_stem(hook):
    """Fuzzy identity for pairing modified entries: the functional core of the command."""
    command = hook.get("command", "")
    if not isinstance(command, str) or not command:
        prompt = hook.get("prompt", "")
        return "prompt:" + str(prompt)[:40]
    m = re.search(r"--hook\s+(\S+)", command)
    if m:
        return f"loaf-check:{m.group(1)}"
    normalized = re.sub(r"\{\{LOAF_EXECUTABLE\}\}|'[^']*loaf'|\bloaf\b", "loaf", command)
    return normalized


def field_diff(desired, actual):
    keys = sorted(set(desired) | set(actual))
    out = []
    for k in keys:
        d, a = desired.get(k), actual.get(k)
        if d != a:
            out.append({"field": k, "desired": d, "actual": a})
    return out


def classify(target, desired_hooks, actual_hooks, is_loaf, event_map=None):
    rows = []
    events = sorted(set(desired_hooks) | set(actual_hooks))
    for event in events:
        desired = list(desired_hooks.get(event, []))
        actual = list(actual_hooks.get(event, []))
        desired_by_sig = {signature(h): h for h in desired}
        matched_actual, matched_desired = set(), set()

        # Pass 1: exact signature match.
        for i, hook in enumerate(actual):
            sig = signature(hook)
            if sig in desired_by_sig:
                pair = desired_by_sig[sig]
                j = desired.index(pair)
                if j in matched_desired:
                    continue
                matched_actual.add(i)
                matched_desired.add(j)
                diff = field_diff(pair, hook)
                rows.append({
                    "target": target, "event": event,
                    "disposition": "in-sync" if not diff else "modified",
                    "identity": command_stem(hook),
                    "desired": pair, "actual": hook, "diff": diff,
                })

        # Pass 2: fuzzy pairing of leftover loaf-owned entries (modified commands).
        leftover_desired = [(j, h) for j, h in enumerate(desired) if j not in matched_desired]
        for i, hook in enumerate(actual):
            if i in matched_actual or not is_loaf(hook):
                continue
            stem = command_stem(hook)
            hit = next(((j, h) for j, h in leftover_desired if command_stem(h) == stem), None)
            if hit:
                j, pair = hit
                matched_actual.add(i)
                matched_desired.add(j)
                leftover_desired = [(k, h) for k, h in leftover_desired if k != j]
                rows.append({
                    "target": target, "event": event, "disposition": "modified",
                    "identity": stem, "desired": pair, "actual": hook,
                    "diff": field_diff(pair, hook),
                })

        # Remaining desired entries: deleted from the live file.
        for j, hook in enumerate(desired):
            if j not in matched_desired:
                rows.append({
                    "target": target, "event": event, "disposition": "deleted",
                    "identity": command_stem(hook), "desired": hook,
                    "actual": None, "diff": [],
                })

        # Remaining actual entries: loaf-owned strays vs foreign.
        for i, hook in enumerate(actual):
            if i in matched_actual:
                continue
            if is_loaf(hook):
                disposition = "stale-loaf"
            else:
                owned, conflict = codex_ownership(hook)
                disposition = "conflict" if conflict else "foreign"
            rows.append({
                "target": target, "event": event, "disposition": disposition,
                "identity": command_stem(hook), "desired": None,
                "actual": hook, "diff": [],
            })
    return rows


def load(path):
    with open(path) as f:
        return json.load(f)


def main():
    cursor_desired = load(os.path.join(REPO, "dist/cursor/hooks.json"))["hooks"]
    codex_desired = load(os.path.join(REPO, "dist/codex/.codex/hooks.json"))["hooks"]
    cursor_actual = load(os.path.join(HOME, ".cursor/hooks.json"))["hooks"]
    codex_actual = load(os.path.join(HOME, ".codex/hooks.json"))["hooks"]

    rows = []
    rows += classify("cursor", cursor_desired, cursor_actual, cursor_is_loaf)

    def codex_is_loaf(hook):
        owned, _ = codex_ownership(hook)
        return owned

    rows += classify("codex", codex_desired, codex_actual, codex_is_loaf)

    summary = {}
    for r in rows:
        key = (r["target"], r["disposition"])
        summary[key] = summary.get(key, 0) + 1

    out = {
        "generated_against": "dist @ main 4edd80d6 (0.2.20)",
        "summary": [
            {"target": t, "disposition": d, "count": c}
            for (t, d), c in sorted(summary.items())
        ],
        "rows": rows,
    }
    out_path = os.path.join(HERE, "hook-entry-classification.json")
    with open(out_path, "w") as f:
        json.dump(out, f, indent=2)
    for s in out["summary"]:
        print(f"{s['target']:8} {s['disposition']:10} {s['count']}")
    print(f"\nwrote {out_path} ({len(rows)} rows)")


if __name__ == "__main__":
    main()
