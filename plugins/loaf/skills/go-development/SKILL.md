---
name: go-development
description: >-
  Covers idiomatic Go development: project structure, concurrency patterns,
  error handling, and testing. Use when writing Go services, CLIs, or libraries.
  Follows Effective Go principles and community conventions. Not for database
  schema design (use...
user-invocable: false
allowed-tools: 'Read, Write, Edit, Bash, Glob, Grep'
version: 2.0.0-dev.31
---

# Go Skill

Patterns and best practices for Go development, grounded in Effective Go principles and community conventions.

## Contents
- Critical Rules
- Verification
- Quick Reference
- Topics
- Naming Conventions
- Key Decisions

## Critical Rules

- Always return errors; check them immediately -- never ignore with `_`
- Run `gofmt` on all Go files before committing -- formatting is non-negotiable
- Prefer standard library over external dependencies
- Define interfaces at the consumer, not the provider
- Keep interfaces small (1-3 methods)

## Verification

- `go vet ./...` passes with no warnings
- `gofmt -l .` produces no output (all files formatted)
- `go test ./...` passes for all modified packages

## Quick Reference

| Element | Convention | Example |
|---------|------------|---------|
| Packages | lowercase, short, no underscores | `http`, `bytes`, `strconv` |
| Exported | PascalCase | `ReadFile`, `HTTPClient` |
| Unexported | camelCase | `parseConfig`, `internalState` |
| Interfaces (single method) | Method + "er" | `Reader`, `Stringer`, `Handler` |
| Getters | No "Get" prefix | `user.Name()` not `user.GetName()` |
| Acronyms | All caps when exported | `HTTPServer`, `xmlParser` |

## Topics

| File | Use When |
|------|----------|
| `references/core.md` | Setting up projects, naming variables, formatting code |
| `references/concurrency.md` | Working with goroutines, channels, or sync primitives |
| `references/errors.md` | Handling errors, creating custom errors, using panic/recover |
| `references/testing.md` | Writing table-driven tests, benchmarks, or examples |
| `references/idioms.md` | Using guard clauses, defer, embedding, or method receivers |

## Naming Conventions

| Element | Convention | Example |
|---------|------------|---------|
| Packages | lowercase, short, no underscores | `http`, `bytes`, `strconv` |
| Exported | PascalCase | `ReadFile`, `HTTPClient` |
| Unexported | camelCase | `parseConfig`, `internalState` |
| Interfaces (single method) | Method + "er" | `Reader`, `Stringer`, `Handler` |
| Getters | No "Get" prefix | `user.Name()` not `user.GetName()` |
| Acronyms | All caps when exported | `HTTPServer`, `xmlParser` |

## Key Decisions

| Area | Decision |
|------|----------|
| Error handling | Return errors, check immediately, no exceptions |
| Formatting | `gofmt` is non-negotiable |
| Concurrency | Channels for communication, goroutines are cheap |
| Interfaces | Small (1-3 methods), defined by consumer |
| Dependencies | Standard library first, minimize external deps |
| Generics | Use when type safety matters; prefer interfaces when behavior matters |
