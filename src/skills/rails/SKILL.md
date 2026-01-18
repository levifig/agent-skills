---
name: rails
description: Use for all Rails 8+ development. Covers models, controllers, views, Hotwire, API development, background jobs, security, testing, performance, deployment, and styling.
---

# Rails 8+ Development

Complete guide for building modern Rails applications following The Rails Way.

## Rails 8 Stack

| Component | Default | Alternative |
|-----------|---------|-------------|
| Database | SQLite (dev/prod) | PostgreSQL |
| Background Jobs | Solid Queue | Sidekiq |
| Caching | Solid Cache | Redis |
| WebSockets | Solid Cable | Redis |
| Assets | Propshaft + Import Maps | esbuild |
| CSS | Tailwind (standalone) | Bootstrap |
| Deployment | Kamal | Capistrano |

## Core Philosophy

- **Convention over configuration** - Follow Rails defaults
- **Embrace the monolith** - Most apps don't need microservices
- **Server-side rendering** - HTML over the wire via Hotwire
- **No build step** - Import Maps + Propshaft, no Node.js
- **Database-backed everything** - Solid Queue/Cache/Cable use your database
- **Test with fixtures** - Minitest + fixtures, not RSpec + factories

## Quick Reference

```bash
# New Rails 8 app
rails new myapp --database=postgresql

# Generators
bin/rails g model User name:string email:string
bin/rails g controller Posts index show
bin/rails g authentication

# Development
bin/dev                    # Start with Procfile.dev
bin/rails test             # Run tests
bin/rails db:migrate       # Run migrations

# Deployment (Kamal)
kamal setup && kamal deploy
```

## Topics

| Topic | Reference | Key Patterns |
|-------|-----------|--------------|
| Core | [core.md](reference/core.md) | Framework principles, file organization, generators |
| Models | [models.md](reference/models.md) | Active Record, validations, associations, migrations |
| Controllers | [controllers.md](reference/controllers.md) | RESTful actions, strong params, filters |
| Views | [views.md](reference/views.md) | Templates, partials, layouts, helpers |
| Hotwire | [hotwire.md](reference/hotwire.md) | Turbo Drive/Frames/Streams, Stimulus |
| API | [api.md](reference/api.md) | JSON APIs, versioning, authentication |
| Jobs | [jobs.md](reference/jobs.md) | Solid Queue, Active Job, recurring jobs |
| Security | [security.md](reference/security.md) | Authentication, authorization, CSRF, XSS |
| Testing | [testing.md](reference/testing.md) | Minitest, fixtures, system tests |
| Performance | [performance.md](reference/performance.md) | N+1 prevention, caching, indexing |
| Deployment | [deployment.md](reference/deployment.md) | Kamal, Docker, zero-downtime |
| Styling | [styling.md](reference/styling.md) | Tailwind CSS, responsive design |
| Services | [services.md](reference/services.md) | Service objects, result patterns |
| Assets | [assets.md](reference/assets.md) | Propshaft, Import Maps |
| Mobile | [mobile.md](reference/mobile.md) | Hotwire Native, Bridge Components |

## Critical Rules

### Always
- Follow Rails naming conventions exactly
- Use generators (`bin/rails g`) for scaffolding
- Write tests first (TDD with Minitest)
- Keep controllers thin (under 10 lines per action)
- Return proper HTTP status codes
- Use strong parameters for all user input
- Index foreign keys in migrations

### Never
- Fight Rails conventions without excellent reason
- Put business logic in controllers or views
- Skip model validations
- Use `render` after successful mutations (use `redirect_to`)
- Store secrets in code (use `credentials:edit`)
- Write custom CSS when Tailwind utilities exist

## File Organization

```
app/
├── controllers/     # HTTP request handling
├── models/          # Business logic and data
├── views/           # Templates and partials
├── services/        # Complex operations (app/services/users/register.rb)
├── jobs/            # Background processing
├── javascript/      # Stimulus controllers
│   └── controllers/
└── assets/
    └── stylesheets/ # Tailwind CSS
```

## Decision Guide

| Need | Solution |
|------|----------|
| CRUD operations | RESTful controllers |
| Dynamic updates | Turbo Frames/Streams |
| Complex business logic | Service objects |
| Background processing | Solid Queue jobs |
| External API | API controller + token auth |
| Mobile app | Hotwire Native |
