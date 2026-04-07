# Feature Specifications

Break VISION + ARCHITECTURE + REQUIREMENTS into specific, implementable specs.

## Contents

- Philosophy
- Spec Format
- Spec Lifecycle
- Creating Specs
- Splitting Large Specs
- Archiving Specs
- Spec vs Plan
- Validation Checklist
- Example: Good vs Bad Specs

## Philosophy

**Specs are shaped solutions, not detailed designs.**

A spec defines:
- What problem we're solving
- Boundaries (in/out of scope)
- High-level approach
- Test conditions

A spec does NOT define:
- Implementation details
- Code structure
- Exact UI layouts

## Spec Format

```yaml
---
id: SPEC-001
title: "User Authentication with OAuth"
requirement: "2.1 User Authentication"
created: 2026-01-23T14:30:00Z
status: drafting  # drafting | approved | implementing | complete
---

# SPEC-001: User Authentication with OAuth

## Problem Statement

[From REQUIREMENTS 2.1 - why this matters to users and the business]

## Proposed Solution

[Shaped solution - high-level approach, not implementation details]

## Scope

### In Scope
- OAuth: Google, GitHub
- Session management with secure cookies
- Login/logout UI

### Out of Scope (Rabbit Holes)
- Custom OAuth providers (avoid this complexity)
- MFA (future spec if needed)
- Password-based auth

### No-Gos
- Passwords in plaintext
- Tokens in local storage
- Session data in URLs

## Test Conditions

- [ ] OAuth flow completes for Google
- [ ] OAuth flow completes for GitHub
- [ ] Session persists across page refresh
- [ ] Logout clears session completely
- [ ] Invalid tokens are rejected

## Implementation Notes

[Architecture references, technical constraints, relevant ADRs]

## Priority Order

Ship in order: Google OAuth -> GitHub OAuth -> session management. If blocked, drop from end.

**Go/no-go gate** after Google OAuth: does the auth flow work end-to-end? If no, stop and reshape.
```

**Create with:** `loaf spec create` or manually in `.agents/specs/`

## Spec Lifecycle

```
drafting → approved → implementing → complete
    │         │           │           │
    └─────────┴───────────┴───────────┘
              can return to drafting
```

| Status | Meaning |
|--------|---------|
| `drafting` | Being refined, not ready for work |
| `approved` | User approved, ready for task breakdown |
| `implementing` | Tasks created, work in progress |
| `complete` | All tasks done, spec archived |

## Creating Specs

### Input Required

1. **Requirement reference** - Which section of REQUIREMENTS.md
2. **Complexity size** - Small, medium, or large (see Splitting Large Specs)

### Interview Questions

Before drafting, clarify:

| Area | Questions |
|------|-----------|
| Scope | What's definitely in? What's definitely out? |
| Rabbit holes | What seems related but should be avoided? |
| No-gos | What approaches are forbidden? |
| Test conditions | How do we know it works? |
| Dependencies | What must exist first? |
| Edge cases | What could go wrong? |

### Writing the Spec

1. **Start with Problem Statement** - Not "build X" but "solve Y"
2. **Shape the solution** - Direction, not blueprint
3. **Define boundaries explicitly** - In/out/no-gos
4. **Write test conditions** - Observable outcomes
5. **Set priority order** - Ship order A->B->C with go/no-go gates

## Splitting Large Specs

When a spec is too big:

```
SPEC-001-user-auth.md (large)
        ↓ split into
SPEC-001a-oauth-integration.md (medium)
SPEC-001b-session-management.md (small)
SPEC-001c-login-ui.md (medium)
```

Each sub-spec:
- Has its own complexity size
- Can be worked independently
- References the original requirement

## Archiving Specs

When all tasks for a spec are complete:

1. Update status to `complete`
2. Archive with the CLI:

```bash
loaf spec archive SPEC-001
```

## Validation Checklist

Before approving a spec:

- [ ] Problem statement is clear and user-focused
- [ ] Scope boundaries are explicit (in/out/no-gos)
- [ ] Test conditions are observable and testable
- [ ] Priority order is defined with go/no-go gates
- [ ] Requirement reference is valid
- [ ] No implementation details (those go in plans)

## Example: Good vs Bad Specs

### Bad Spec

```markdown
# SPEC-001: Add OAuth

Add OAuth to the app.

## Requirements
- Add Google OAuth
- Add session management
- Add logout
```

Problems:
- No problem statement
- No scope boundaries
- No test conditions
- No priority order
- Too vague to implement

### Good Spec

```markdown
# SPEC-001: User Authentication with OAuth

## Problem Statement

Users currently can't access the application. We need a secure,
frictionless login flow that leverages existing identity providers
so users don't need to create yet another password.

## Proposed Solution

Implement OAuth 2.0 flow with Google and GitHub as initial providers.
Store session in secure HTTP-only cookies. Provide clear login/logout UI.

## Scope

### In Scope
- Google OAuth integration
- GitHub OAuth integration
- Session cookie management
- Login page with provider buttons
- Logout functionality

### Out of Scope
- Apple/Microsoft providers (future)
- MFA (separate spec if needed)
- Account linking (complex, avoid)

### No-Gos
- Storing tokens in localStorage
- Custom password auth
- Session data in URLs

## Test Conditions

- [ ] New user can sign in with Google
- [ ] New user can sign in with GitHub
- [ ] Session persists across browser tabs
- [ ] Session survives page refresh
- [ ] Logout clears all session data
- [ ] Expired sessions redirect to login

## Priority Order

Ship in order: Google OAuth -> GitHub OAuth -> session UI. Drop from end if scope exceeds complexity.

**Go/no-go gate** after Google OAuth: does the auth flow work end-to-end? If no, stop and reshape.
```
