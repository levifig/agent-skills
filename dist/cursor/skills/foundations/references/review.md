## Universal Code Review Instructions

  Run a high-signal review for the currently relevant code in this workspace.

  Complete the review end-to-end without asking the user unless truly blocked by missing permissions/tools.

  ### Hard Rules
  - Only report **high-confidence, high-signal** issues.
  - Do not report style preferences, subjective suggestions, or “might be” concerns.
  - Do not report issues a linter would catch.
  - Do not report issues requiring context outside the diff unless you can validate with certainty.
  - Do not report pre-existing issues.
  - Do not run tests/linters/builds unless explicitly requested.
  - Subagents must **never** post comments; only the main agent posts comments.
  - Use `gh` for GitHub interactions and `git` for local diff/context.
  - Do not use AskUserQuestion or equivalent user-interrupt flow.

  ---

  ### 1) Build Review Scope

  1. Detect PR context on current branch:
     - `gh pr view --json number,title,body,baseRefName,headRefName,url`
     - If PR exists, capture title/body for intent context.

  2. Build committed diff scope using this order:
     - If PR exists: PR-style diff vs PR base merge-base.
     - If no PR exists: check if on a local branch (not detached HEAD).
       - If on a branch: run PR-style review of current branch vs repo default branch.
       - Determine default branch via:
         - `gh repo view --json defaultBranchRef --jq '.defaultBranchRef.name'`
         - fallback: `git symbolic-ref refs/remotes/origin/HEAD` parsing.
     - If neither PR scope nor branch scope is available, skip committed diff scope.

  3. Capture uncommitted local changes (staged + unstaged + untracked where possible).

  4. If committed scope is empty and uncommitted scope is empty:
     - Review recent commits as a unit (default `HEAD~5..HEAD`, or available history).

  5. Build one combined review set from all applicable scopes and deduplicate overlapping hunks.

  ---

  ### 2) Discover Applicable Policy Files

  Find all relevant `AGENTS.md` and `CLAUDE.md` files:
  - Root-level policy files, if present.
  - For each changed file, include policy files in that file’s directory ancestry.
  - Only enforce policy rules that are in scope for that file path.

  Return policy file **paths only** first, then load contents as needed.

  ---

  ### 3) Parallel Change Understanding

  In parallel:
  - Read PR title/body (if PR exists; do not read PR comments/discussion unless necessary).
  - Produce a concise summary of what changed from the combined diff.

  ---

  ### 4) Parallel Independent Reviews (4 passes)

  Launch 4 independent review passes over the same combined diff:

  1. Policy compliance pass #1 (AGENTS/CLAUDE scoped rule adherence).
  2. Policy compliance pass #2 (independent duplicate check).
  3. Bug pass #1 (runtime/logic bugs visible in changed code only).
  4. Bug pass #2 (security/correctness/regression risks in changed code only).

  All passes receive PR title/body as intent context (if available).

  Each pass must return issues in this structure:
  - `title`
  - `description`
  - `reason_flagged` (e.g. `AGENTS.md violation`, `CLAUDE.md violation`, `bug`, `security bug`)
  - `file`
  - `line` (or nearest changed line)
  - `confidence` (`high` only)
  - `evidence` (diff-based proof; for policy issues include exact quoted rule + source path/line)

  ---

  ### 5) Validate Every Candidate Issue

  For each candidate issue, run a validator subagent:
  - Policy issues: validate rule exists, rule is in scope for file path, and violation is clear.
  - Bug issues: validate defect is real from changed code with high confidence.

  Model guidance:
  - Use stronger bug-focused validators for bug/security/correctness issues.
  - Use policy-focused validators for AGENTS/CLAUDE issues.

  Reject anything uncertain or interpretive.

  ---

  ### 6) Filter to Final High-Signal Set

  - Keep only issues that pass validation with high confidence.
  - Deduplicate so each unique root issue appears once.
  - If no validated issues remain, report that explicitly.

  ---

  ### 7) Post Inline PR Comments (if PR exists)

  For each final issue, post exactly one inline comment on the PR.
  - One comment per unique issue.
  - Include:
    - concise problem statement
    - concrete impact
    - evidence
    - citation link(s): changed file location and policy rule source (for policy issues)

  If no PR exists or inline comments cannot be posted, skip posting and mark as `not posted`.

  ---

  ### 8) Final Output Format

  Produce:

  1. `Scope Reviewed`
     - PR info (if present)
     - branch-vs-default review used or not
     - uncommitted diff included or not
     - recent commit range used or not

  2. `Change Summary`
     - brief, factual

  3. `Validated Issues`
     - numbered list, highest severity first
     - each item includes:
       - title
       - why it is a real issue
       - file + line
       - comment URL/location (or `not posted`)

  4. `Dropped Candidates`
     - brief list with reason (`not reproducible`, `out of scope`, `low confidence`, `duplicate`)

  Do not mention whether fallback strategy was used unless it materially affects review quality.

  ---

  ### High-Signal Definition (strict)

  Report only:
  - Objective runtime/correctness/security bugs in changed code.
  - Clear, scoped AGENTS/CLAUDE rule violations with exact rule citation.

  Do not report:
  - Subjective quality suggestions.
  - Pedantic nits.
  - Hypotheticals without proof.
  - Pre-existing problems.
  - Linter-only findings.
  - General code quality concerns (e.g. “needs more tests”) unless explicitly required by scoped AGENTS/CLAUDE policy.
  - Issues explicitly and validly silenced in code/policy.

  ---

  ### False-Positive Rejection List (explicit)

  Do NOT flag:
  - Pre-existing issues.
  - Something that appears buggy but is actually correct.
  - Pedantic nitpicks a senior engineer would not raise.
  - Linter-catch issues.
  - General concerns without explicit scoped policy requirement.
  - Policy/lint issues explicitly and validly suppressed in code.

  ---

  ### Fallback: No Subagents Available

  If subagents are unavailable, perform the same process sequentially yourself:
  - Policy pass #1
  - Policy pass #2
  - Bug pass #1
  - Bug pass #2
  - Then self-validate each issue with the same high-confidence bar.
