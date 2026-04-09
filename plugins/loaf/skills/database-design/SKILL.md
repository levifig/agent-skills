---
name: database-design
description: >-
  Covers schema design, migrations, query optimization, indexing strategies, and
  normalization tradeoffs. Use when designing tables, writing migrations,
  optimizing queries, or planning index strategies. Provides patterns for
  database administration ...
user-invocable: false
allowed-tools: 'Read, Write, Edit, Glob, Grep, Bash(psql:*, sqlite3:*, mysql:*)'
version: 2.0.0-dev.24
---

# Database Skill

Domain knowledge for database administration and development. Covers schema design, migrations, query optimization, and indexing strategies.

## Critical Rules

- **Data integrity first** -- constraints catch bugs that code misses; always define foreign keys, NOT NULLs, and check constraints at the database level
- **Backward compatibility** -- migrations must not break running code; use safe online operations (ADD COLUMN nullable, ADD INDEX CONCURRENTLY) over unsafe ones
- **Measure before optimizing** -- use EXPLAIN ANALYZE, not intuition; never add indexes or denormalize without evidence of a performance problem
- **Plan for scale** -- design decisions are expensive to change; consider write patterns, cardinality, and growth from the start

## Verification

- New tables have appropriate primary keys, foreign keys, and NOT NULL constraints
- Migrations use only safe online operations, or document a maintenance window if unsafe operations are required
- Query changes are validated with EXPLAIN ANALYZE showing expected plan improvements

## Quick Reference

### Primary Key Selection

| Type | Use When | Avoid When |
|------|----------|------------|
| UUID v4 | Distributed systems, no sequential exposure | Need sortability, space-constrained |
| ULID | Need sortability + uniqueness | Legacy system compatibility |
| Serial/Identity | Single database, simple use case | Distributed writes, ID exposure concerns |

### Migration Safety

```
Safe (online):     ADD COLUMN (nullable), ADD INDEX CONCURRENTLY, CREATE TABLE
Unsafe (offline):  ADD COLUMN NOT NULL (without default), DROP COLUMN, RENAME
```

### Index Decision Tree

```
High cardinality + equality lookups  → B-tree (default)
Low cardinality + many values        → Consider partial index
Array/JSON containment               → GIN
Geometric/range queries              → GiST
Exact equality only                  → Hash (rare)
```

## Topics

| Topic | Reference | Use When |
|-------|-----------|----------|
| Schema Design | [schema-design.md](references/schema-design.md) | Creating tables, choosing keys, audit patterns |
| Migrations | [migrations.md](references/migrations.md) | Writing safe, reversible migrations |
| Query Optimization | [query-optimization.md](references/query-optimization.md) | Debugging slow queries, N+1 detection |
| Indexing | [indexing.md](references/indexing.md) | Choosing index types, composite index order |

## When to Use This Skill

- Designing new tables or modifying existing schemas
- Writing or reviewing database migrations
- Optimizing slow queries
- Planning index strategies
- Evaluating normalization vs denormalization tradeoffs

## Related Skills

- See `foundations` for code style, TDD, and verification principles
- See `infrastructure` for connection pooling and deployment patterns
