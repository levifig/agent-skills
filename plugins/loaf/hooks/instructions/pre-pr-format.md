## PR format reminder

**Root:** One shippable root. Assemble related stacked children with verified ancestry and `--ff-only`; split independent roots unless a human explicitly approves one atomic landing.

**Title:** Conventional commit, <70 chars. No scope prefixes, no SPEC/TASK IDs.

**Body:** `## Summary` (2-4 bullets) + `## Test plan` (checklist).

**Merge:** Squash this shippable unit with a clean extended description (2-4 lines). Do not use an auto-generated description or an incidental merge commit.
