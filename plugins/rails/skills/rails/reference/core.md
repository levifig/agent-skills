# Rails Core Principles

Foundation for Rails development following The Rails Way.

## Framework Principles

### Database-First Design
- Active Record at the center of everything
- Database constraints enforce data integrity
- Migrations document schema evolution
- Indexes for query performance

### RESTful by Default
- Resources map directly to routes
- Seven standard actions: `index`, `show`, `new`, `create`, `edit`, `update`, `destroy`
- Nested resources when relationships exist
- Custom actions only when truly necessary

### Server-Side Rendering with Hotwire
- HTML over the wire, not JSON APIs for web UIs
- Turbo for navigation without full page reloads
- Stimulus for JavaScript sprinkles
- Progressive enhancement over SPAs

## File Organization

```
app/
├── controllers/     # HTTP request handling
├── models/          # Business logic and data
├── views/           # Templates and partials
├── services/        # Complex multi-model operations
├── jobs/            # Background processing
├── mailers/         # Email sending
├── helpers/         # View helpers
└── javascript/
    └── controllers/ # Stimulus controllers
```

## Generators

```bash
# Models
bin/rails g model User name:string email:string:uniq
bin/rails g model Post user:references title:string body:text

# Controllers
bin/rails g controller Posts index show new create

# Scaffolds (full CRUD)
bin/rails g scaffold Article title:string body:text

# Authentication (Rails 8)
bin/rails g authentication
```

## Rails 8 Defaults

Rails 8 ships with sensible defaults that work without external dependencies:

- **Solid Queue** - Database-backed job processing (no Redis)
- **Solid Cache** - Database-backed caching
- **Solid Cable** - Database-backed WebSockets
- **Propshaft** - Simple asset pipeline (no compilation)
- **Import Maps** - JavaScript without bundling
- **Kamal** - Zero-downtime Docker deployments

## Configuration

```ruby
# config/application.rb
module MyApp
  class Application < Rails::Application
    config.load_defaults 8.0
    config.time_zone = "UTC"
    config.eager_load_paths << Rails.root.join("lib")
  end
end
```

## Environment-Specific Settings

```ruby
# config/environments/production.rb
config.force_ssl = true
config.log_level = :info
config.active_record.dump_schema_after_migration = false
```

## Credentials

```bash
EDITOR="code --wait" bin/rails credentials:edit
EDITOR="code --wait" bin/rails credentials:edit --environment production
```

```ruby
# Access credentials
Rails.application.credentials.stripe[:secret_key]
Rails.application.credentials.dig(:aws, :access_key_id)
```
