# Feature Specifications

Break VISION + ARCHITECTURE + REQUIREMENTS into specific, implementable specs.

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
appetite: "1 week"
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

## Circuit Breaker

At 50% appetite: if OAuth integration is problematic, simplify to single provider (Google only).
```

**Location:** `.agents/specs/SPEC-001-feature-name.md`

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
2. **Appetite** - How much time is it worth (from Shape Up)

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
5. **Set circuit breaker** - When to stop and reassess

## Appetite Levels

| Appetite | Size | Suitable For |
|----------|------|--------------|
| Small | 1-2 days | Bug fixes, minor enhancements |
| Medium | 3-5 days | Feature additions, refactors |
| Large | 1-2 weeks | Major features (rare) |

If a spec can't fit the appetite, it needs more shaping or should be split.

## Splitting Large Specs

When a spec is too big:

```
SPEC-001-user-auth.md (large, 2 weeks)
        ↓ split into
SPEC-001a-oauth-integration.md (medium, 3 days)
SPEC-001b-session-management.md (small, 2 days)
SPEC-001c-login-ui.md (medium, 3 days)
```

Each sub-spec:
- Has its own appetite
- Can be worked independently
- References the original requirement

## Archiving Specs

When all tasks for a spec are complete:

1. Update status to `complete`
2. Add completion date to frontmatter
3. Move to `.agents/specs/archive/`

```bash
mv .agents/specs/SPEC-001-user-auth.md .agents/specs/archive/
```

## Spec vs Plan

| Spec | Plan |
|------|------|
| **What** to build | **How** to build it |
| Lives in `.agents/specs/` | Lives in `.agents/plans/` |
| Survives sessions | Tied to session |
| User-facing | Implementation detail |
| Shape Up shaped | Tactical steps |

Specs define the target. Plans define the route to get there.

## Validation Checklist

Before approving a spec:

- [ ] Problem statement is clear and user-focused
- [ ] Scope boundaries are explicit (in/out/no-gos)
- [ ] Test conditions are observable and testable
- [ ] Appetite is realistic for the scope
- [ ] Circuit breaker is defined
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
- No appetite
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

## Circuit Breaker

At 50% appetite: if both providers are problematic, ship with Google only.
GitHub can be a follow-up spec.
```
