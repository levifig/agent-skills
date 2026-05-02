---
type: glossary
topics:
  - glossary
last_reviewed: '2026-05-02'
---
## Canonical Terms

### Skill

A domain-knowledge unit following the Agent Skills standard. Loaf's universal knowledge layer — distributed across all 6 build targets without target-specific code.

_Avoid_: module, knowledge file, doc

### Target

A build output destination: claude-code, opencode, cursor, codex, gemini, amp. Each target has its own transformer in cli/lib/build/targets/.

_Avoid_: platform, backend, tool

### Sidecar

A target-specific YAML file that extends a SKILL.md with target-only fields (e.g., SKILL.claude-code.yaml for user-invocable, argument-hint). Merged into the build output for that target only.

_Avoid_: extension, override, plugin file

### Shared Template

A markdown template at content/templates/ that is distributed to multiple skills at build time per the shared-templates registration in config/targets.yaml. Examples: session.md, adr.md, grilling.md.

_Avoid_: common template, global template

## Candidates


## Relationships


## Flagged ambiguities

