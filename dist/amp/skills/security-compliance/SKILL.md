---
name: security-compliance
description: >-
  Covers threat modeling, secrets management, security reviews, and compliance
  verification. Use when reviewing code for security, managing secrets,
  performing threat analysis, or running compliance audits. Not for debugging
  (use debugging) or general code review (use foundations).
version: 2.0.0-dev.27
---

# Security & Compliance

Security patterns, threat modeling, and compliance verification.

## Contents
- Critical Rules
- Verification
- Topics

## Critical Rules

- **Never** store secrets, credentials, or PII in version control — use environment variables or secret managers
- **Always** run secrets scanning before committing (pre-commit hooks)
- **Validate all inputs** at trust boundaries — never trust client-side validation alone
- **Apply least privilege** — grant minimum permissions required for each component
- **Document threat model** before implementing security-sensitive features

## Verification

- Secrets scanner passes with zero findings on all staged files
- No hardcoded credentials, API keys, or tokens in source code
- Security review checklist completed for changes touching auth, crypto, or data handling

## Topics

| Topic | Reference | Use When |
|-------|-----------|----------|
| Security | `references/security.md` | Threat modeling, managing secrets, compliance checks |
| Security Review | `references/security-review.md` | Running security review checklists, auditing code |
