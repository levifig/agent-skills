# Documentation Standards

## Contents
- Core Principle
- Document Hierarchy
- Architecture Decision Records
- API Documentation
- Project Changelog
- Micro-Changelog
- Critical Rules

Project documentation conventions, ADR format, and API doc rules.

## Core Principle

**Document after shipping, not before.** API documentation reflects ONLY what is implemented and released. Future features belong in Linear issues, not docs.

## Document Hierarchy

```
docs/
├── PRD.md                # What the product should be (vision)
├── ARCHITECTURE.md       # How the product is built (design)
├── IMPLEMENTATION.md     # What the product currently is (status)
├── QUICK_REFERENCE.md    # One-page command reference
├── api/                  # API docs (implemented features only)
│   ├── openapi.yaml
│   └── endpoints/
└── decisions/            # Architecture Decision Records
    ├── ADR000-template.md
    └── ADR001-*.md
```

| Document | Purpose | Updates When |
|----------|---------|--------------|
| **PRD.md** | Product vision (timeless) | Vision changes |
| **ARCHITECTURE.md** | Technical design | Architecture changes |
| **IMPLEMENTATION.md** | Current status | Features ship |
| **API docs** | Implemented endpoints | Features ship |

## Architecture Decision Records

ADRs are owned by the `architecture` skill. Use
`content/skills/architecture/templates/adr.md` as the single template and route
ADR lifecycle, numbering, supersession, and triage-gate questions there.

**When to write:** Technology choices, architectural patterns, integration
approaches, security decisions. **Skip for:** library version updates, bug
fixes, performance tweaks, style changes.

## API Documentation

**The implemented-only rule:** `Feature Request -> Implementation -> Tests Pass -> Release -> Update API Docs`

Deprecation markers in OpenAPI:
```yaml
paths:
  /api/v1/legacy-endpoint:
    get:
      deprecated: true
      description: |
        **Deprecated since**: v1.5.0
        **Removal planned**: v2.0.0
```

## Project Changelog

`CHANGELOG.md` is release communication for humans. Loaf follows
[Common Changelog](https://common-changelog.org/) as a stricter profile of Keep
a Changelog, with one local workflow allowance: `## [Unreleased]` is a staging
section for PR and release preparation.

### File Shape

- Start with `# Changelog`.
- Keep `## [Unreleased]` at the top for unreleased, curated entries.
- Use release headings shaped as `## [VERSION] - YYYY-MM-DD`.
- Sort release sections semver-latest first.
- Match each released `VERSION` to the git tag, allowing the tag to use a `v`
  prefix.
- Copy the final release section into the GitHub Release body; do not use
  generated GitHub notes as the source of truth.

### Categories

Use Common Changelog categories in this order:

1. `Changed` for changes in existing functionality
2. `Added` for new functionality
3. `Removed` for removed functionality
4. `Fixed` for bug fixes

Avoid `Other`, `Internal`, `Migration`, and catch-all headings in curated
release notes. Express migrations or upgrade requirements as a one-sentence
notice under the release heading, or as a `Changed` / `Removed` entry with a
`**Breaking:**` prefix when the software behavior changed.

### Entries

Each entry is one list item and should:

- Start with an imperative, present-tense verb: `Add`, `Fix`, `Remove`,
  `Document`, `Refactor`, `Bump`.
- Describe the user, operator, or integrator impact without depending on the
  category heading.
- Stay on one line whenever possible.
- Include the best public reference in parentheses when available, such as a
  PR, issue, ADR, release, or commit link.
- Merge related commits into one meaningful release note.
- Sort breaking changes first, then by importance, then latest-first.

Examples:

```markdown
### Changed

- **Breaking:** resolve `.agents/` from the main worktree in linked checkouts ([ADR-013](docs/decisions/ADR-013-agentic-state-storage-model.md))
- Document release guardrails for post-merge tagging ([#50](https://github.com/levifig/loaf/pull/50))

### Added

- Add `loaf task list --status <status>` for lifecycle filtering
```

### Curation

Generated changelog text is a draft. Before release:

- Remove maintenance noise that has no release impact.
- Rewrite commit subjects into human release notes.
- Drop internal spec, task, session, branch, and review-gate identifiers.
- Keep public commands, config keys, documented file names, public ADR IDs, and
  user-visible hook names when they help the reader understand the impact.
- Skip no-op changes that were reverted before the release.

## Micro-Changelog

Track changes at the **bottom** of individual documents:

```markdown
---

## Changelog

- 2025-11-14 - Added section on agent instructions
- 2025-11-12 - Updated architecture overview
```

**Format:** `- YYYY-MM-DD - Short description`, reverse chronological. Log section additions, significant updates, restructuring, corrections. Skip typos and formatting.

## Critical Rules

**Always:** Last Updated timestamp (YYYY-MM-DD), micro-changelog at bottom, reference files not inline code, keep docs minimal.

**Never:** Document APIs before they ship, include `.agents/` links outside `.agents/` artifacts, use lengthy code samples, add planning details to docs.
