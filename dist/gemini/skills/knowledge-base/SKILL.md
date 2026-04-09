---
name: knowledge-base
description: >-
  Provides guidance for creating, updating, and reviewing project knowledge
  files. Covers frontmatter schema, naming conventions, staleness detection via
  covers: field, and the review workflow. Not for retrieval or search (use QMD
  directly), architectural decisions (use ADRs), or agent instructions (use
  CLAUDE.md).
version: 2.0.0-dev.25
---

# Knowledge Base

## Contents
- Critical Rules
- Verification
- Quick Reference
- Topics
- When to Create a Knowledge File
- The covers: Field
- Review Workflow

Conventions and workflows for managing project knowledge files -- structured
domain knowledge that lives in `docs/knowledge/` and persists across sessions.

## Critical Rules

### Always
- Include `topics` (min 1) and `last_reviewed` in frontmatter
- Use kebab-case filenames in `docs/knowledge/`
- Add `covers:` globs when the knowledge maps to specific code paths
- Run `loaf kb validate` before committing new knowledge files
- Mark files reviewed with `loaf kb review` after updating

### Never
- Duplicate code documentation in knowledge files
- Use date prefixes on knowledge filenames (unlike sessions or ideas)
- Use overly broad `covers:` globs that trigger constant staleness alerts
- Skip the `last_reviewed` field -- it is required for the lifecycle to work
- Store knowledge files outside `docs/knowledge/` (ADRs go in `docs/decisions/`)

## Verification

- Run `loaf kb validate` to confirm frontmatter is valid (required fields, correct types, valid globs)
- Run `loaf kb check` to confirm no covered code paths have changed since last review
- Verify `covers:` globs are precise enough to avoid false-positive staleness alerts

## Quick Reference

| Surface | Contains | Decision Test |
|---------|----------|---------------|
| **Code** (docstrings, types) | What the code does | Is it self-documenting? |
| **Knowledge files** | Domain rules, cross-cutting context, roadmap | Requires context beyond the code? |
| **ADRs** | Why we chose this approach | Is it an architectural decision? |
| **CLAUDE.md** | Agent instructions, conventions | Is it about how agents should behave? |
| **MEMORY.md** | User preferences, session pointers | Is it personal or ephemeral? |

CLAUDE.md may reference knowledge files but should never duplicate their content.

## Topics

| Topic | Reference | Use When |
|-------|-----------|----------|
| Frontmatter Schema | [frontmatter-schema.md](references/frontmatter-schema.md) | Creating or validating knowledge file frontmatter |
| Naming Conventions | [naming-conventions.md](references/naming-conventions.md) | Deciding where to put knowledge and what to name it |

| Template | Use When |
|----------|----------|
| [knowledge-file.md](templates/knowledge-file.md) | Creating a new knowledge file from scratch |

## When to Create a Knowledge File

Create a knowledge file when information:

1. **Requires context beyond the code** -- domain rules, business logic constraints,
   cross-cutting patterns that cannot be expressed in types or docstrings
2. **Spans multiple files or modules** -- conventions or contracts that apply across
   the codebase, not localized to one function
3. **Would be lost between sessions** -- insights that an agent needs repeatedly but
   cannot derive from code alone
4. **Serves as extended agent memory** -- roadmap context, implementation plans,
   strategic direction that informs decisions

Do NOT create knowledge files for:
- Self-documenting code (types, docstrings, comments are sufficient)
- One-off architectural decisions (write an ADR instead)
- Agent behavior instructions (put those in CLAUDE.md)
- User preferences or session pointers (those belong in MEMORY.md)

## The covers: Field

The `covers:` field is the core innovation of the knowledge management system.
It links knowledge to code paths via glob patterns, enabling automatic staleness
detection.

### How It Works

1. Author declares `covers:` globs in knowledge file frontmatter
2. Globs resolve from the git repository root
3. `loaf kb check` queries `git log --since={last_reviewed}` for matching files
4. If commits exist since last review, the knowledge file is flagged as potentially stale

### Good covers: Patterns

```yaml
covers:
  - "src/pipeline/registry.py"          # Specific file
  - "src/models/engine_*.py"            # Wildcard within directory
  - "src/thermal/**/*.py"               # Recursive directory
```

### Patterns to Avoid

```yaml
covers:
  - "src/**"                            # Too broad -- constant false positives
  - "**/*.py"                           # Covers everything, means nothing
  - "*.md"                              # Documentation changes != knowledge staleness
```

The goal is precision: cover the code paths whose changes would actually
invalidate the knowledge. Broad globs lead to alert fatigue.

### When to Omit covers:

Files without `covers:` are valid knowledge files but cannot use automated
staleness detection. Omit `covers:` when:

- The knowledge is purely conceptual (no specific code paths)
- The file covers external domain rules not tied to implementation
- You are unsure which paths to cover (add them later when the mapping is clear)

## Review Workflow

### Check What Is Stale

```bash
loaf kb check
```

Shows knowledge files where covered code paths have changed since `last_reviewed`.

### Validate Frontmatter

```bash
loaf kb validate
```

Checks all knowledge files for valid frontmatter: required fields present,
correct types, valid glob patterns.

### Review a File

```bash
loaf kb review <file>
```

After reading and updating a knowledge file, mark it as reviewed. This updates
the `last_reviewed` date to today.

### Overview

```bash
loaf kb status
```

Summary of all knowledge files: total count, stale count, files missing
`covers:`, last review dates.
