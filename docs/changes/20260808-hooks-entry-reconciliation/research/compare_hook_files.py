#!/usr/bin/env python3
"""Predicate-free before/after comparator for hooks.json files.

Diffs two hook files at the JSON-value level, per event section, preserving
relative order: reports entries added, removed, and changed (paired by
position among value-stable neighbors). It carries NO ownership predicate —
the caller asserts that every reported difference is an expected Loaf entry
(e.g. from `loaf hooks list` or the upgrade plan output), which keeps this
tool honest across predicate changes.

Usage: compare_hook_files.py <before.json> <after.json>
Exit 0 with "identical" when no value-level differences exist; exit 1 with a
per-event report otherwise (exit 1 is a report, not a failure).
"""

import json
import sys


def canonical(entry):
    return json.dumps(entry, sort_keys=True)


def diff_event(before, after):
    b = [canonical(e) for e in before]
    a = [canonical(e) for e in after]
    b_set, a_set = {}, {}
    for s in b:
        b_set[s] = b_set.get(s, 0) + 1
    for s in a:
        a_set[s] = a_set.get(s, 0) + 1
    removed = [s for s in b if a_set.get(s, 0) == 0]
    added = [s for s in a if b_set.get(s, 0) == 0]
    # Order stability of surviving entries: shared values must appear in the
    # same relative order on both sides.
    shared_b = [s for s in b if s in a_set and s not in removed]
    shared_a = [s for s in a if s in b_set and s not in added]
    order_stable = shared_b == shared_a
    return removed, added, order_stable


def main():
    if len(sys.argv) != 3:
        sys.exit(__doc__)
    before_doc = json.load(open(sys.argv[1]))
    after_doc = json.load(open(sys.argv[2]))
    clean = True
    # Top-level fields other than "hooks" must be presence- and value-identical:
    # version, description, and any unknown field are part of the preservation
    # promise, and a present null is not the same as an absent key.
    for key in sorted((set(before_doc) | set(after_doc)) - {"hooks"}):
        in_b, in_a = key in before_doc, key in after_doc
        if in_b != in_a:
            clean = False
            print(f"top-level field: {key}")
            print(f"  before: {canonical(before_doc[key]) if in_b else '<absent>'}")
            print(f"  after:  {canonical(after_doc[key]) if in_a else '<absent>'}")
            continue
        if canonical(before_doc[key]) != canonical(after_doc[key]):
            clean = False
            print(f"top-level field: {key}")
            print(f"  before: {canonical(before_doc[key])}")
            print(f"  after:  {canonical(after_doc[key])}")
    if ("hooks" in before_doc) != ("hooks" in after_doc):
        clean = False
        print(f"top-level field: hooks {'removed' if 'hooks' in before_doc else 'added'} as a key")
    before = before_doc.get("hooks", {})
    after = after_doc.get("hooks", {})
    for event in sorted(set(before) | set(after)):
        in_b, in_a = event in before, event in after
        if in_b != in_a:
            clean = False
            print(f"event: {event} {'removed' if in_b else 'added'} as a key"
                  f" ({len(before.get(event, []))} vs {len(after.get(event, []))} entries)")
        removed, added, order_stable = diff_event(before.get(event, []), after.get(event, []))
        if removed or added or not order_stable:
            clean = False
            print(f"event: {event}")
            for s in removed:
                print(f"  removed: {s}")
            for s in added:
                print(f"  added:   {s}")
            if not order_stable:
                print("  ORDER CHANGED among surviving entries")
    if clean:
        print("identical (value-level, order-stable)")
        sys.exit(0)
    sys.exit(1)


if __name__ == "__main__":
    main()
