---
name: ruby-development
description: >-
  Covers Ruby and Rails 8+ development: Hotwire, Solid Queue, Minitest, and
  Rails conventions. Use when building Rails applications, writing Ruby code, or
  following Rails patterns. Follows DHH/37signals conventions. Not for database
  schema design (use database-design) or frontend outside Hotwire (use
  typescript-development).
version: 2.0.0-dev.30
---

# Ruby Development

The DHH/37signals way: convention over configuration, programmer happiness, and elegant simplicity.

## Contents
- Critical Rules
- Verification
- Quick Reference
- Topics
- Stack Overview

## Critical Rules

### Always

- Follow Ruby/Rails naming conventions exactly
- Use generators (`bin/rails g`) for scaffolding
- Write tests first (TDD with Minitest)
- Keep controllers thin (under 10 lines per action)
- Return proper HTTP status codes
- Use strong parameters for all user input
- Index foreign keys in migrations

### Never

- Fight conventions without excellent reason
- Put business logic in controllers or views
- Skip model validations
- Use `render` after successful mutations (use `redirect_to`)
- Store secrets in code (use `credentials:edit`)
- Write custom CSS when Tailwind utilities exist
- Reach for microservices before the monolith fails

## Verification

### After Editing Rails Files

**Migration Safety:**
- For migration files (`db/migrate/*.rb`), check for:
  - `remove_column` without safety checks (use strong_migrations gem)
  - `rename_column` or `rename_table` (consider add+copy+remove pattern)
  - NOT NULL constraints without default on existing tables
  - `add_index` without `algorithm: :concurrently` for large tables
  - `change_column` (may lock table and lose data)
- Deep validation (if time permits):
  - Test forward migration: `rails db:migrate:up VERSION={version}`
  - Test rollback: `rails db:migrate:down VERSION={version}`

**Testing:**
- If using Minitest: `bin/rails test {test_file}`
- If using RSpec: `bundle exec rspec {spec_file}`
- Run related tests by module: `bin/rails test test/models/` or `bundle exec rspec spec/models/`

**Linting:**
- If `standardrb` is available: `standardrb {files}` or `standardrb --fix {files}`
- If `rubocop` is available: `rubocop {files}` or `rubocop -a {files}`

**Security Scanning:**
- If `brakeman` is available: `bundle exec brakeman --quiet --format text`
- Review high/medium confidence warnings

### Before Committing

- Run StandardRB or RuboCop on changed files
- Run migration safety checks for new migrations
- Run tests for modified components
- Run Brakeman scan if reviewing or doing thorough validation
- Check CHANGELOG.md format if it exists

## Quick Reference

### Rails 8 (Default)

| Component | Default | Alternative |
|-----------|---------|-------------|
| Database | SQLite (dev/prod) | PostgreSQL |
| Background Jobs | Solid Queue | Sidekiq |
| Caching | Solid Cache | Redis |
| WebSockets | Solid Cable | Redis |
| Assets | Propshaft + Import Maps | esbuild |
| CSS | Tailwind (standalone) | Bootstrap |
| Deployment | Kamal | Capistrano |

### Beyond Rails

| Use Case | Tool |
|----------|------|
| Microservices | Sinatra, Roda |
| API-only | Grape |
| Full-stack alternative | Hanami |
| CLI tools | Thor, TTY |
| Gem development | Bundler |

## Topics

| Topic | Reference | Use When |
|-------|-----------|----------|
| Core | [core.md](references/core.md) | Writing idiomatic Ruby, organizing Rails files |
| Models | [models.md](references/models.md) | Creating models, validations, associations, migrations |
| Controllers | [controllers.md](references/controllers.md) | Building RESTful actions, strong params, filters |
| Views | [views.md](references/views.md) | Creating templates, partials, layouts, helpers |
| Hotwire | [hotwire.md](references/hotwire.md) | Adding Turbo Drive/Frames/Streams, Stimulus |
| API | [api.md](references/api.md) | Building JSON APIs, versioning, authentication |
| Jobs | [jobs.md](references/jobs.md) | Setting up Solid Queue, Active Job, recurring jobs |
| Security | [security.md](references/security.md) | Implementing authentication, authorization, CSRF |
| Testing | [testing.md](references/testing.md) | Writing Minitest, fixtures, system tests |
| Performance | [performance.md](references/performance.md) | Fixing N+1 queries, caching, indexing |
| Deployment | [deployment.md](references/deployment.md) | Deploying with Kamal, Docker, zero-downtime |
| Styling | [styling.md](references/styling.md) | Setting up Tailwind CSS, responsive design |
| Services | [services.md](references/services.md) | Creating service objects, result patterns |
| Mobile | [mobile.md](references/mobile.md) | Building Hotwire Native, Bridge Components |
| Debugging | [debugging.md](references/debugging.md) | Using binding.irb, byebug, Rails console |
