# Rails Deployment with Kamal

Production deployment using Kamal, Rails 8's default deployment tool.

## Kamal Basics

```bash
gem install kamal && kamal init  # Install and setup
kamal setup                       # First-time server setup
kamal deploy                      # Deploy application
kamal rollback                    # Revert to previous version
```

## Docker Configuration

```dockerfile
# Dockerfile (Rails 8 default - multi-stage build)
FROM ruby:3.3-slim AS base
WORKDIR /rails
ENV RAILS_ENV="production" BUNDLE_WITHOUT="development:test"

FROM base AS build
RUN apt-get update -qq && apt-get install -y build-essential git libpq-dev
COPY Gemfile Gemfile.lock ./
RUN bundle install && bundle exec bootsnap precompile --gemfile app/ lib/
COPY . .
RUN SECRET_KEY_BASE_DUMMY=1 ./bin/rails assets:precompile

FROM base
RUN apt-get update -qq && apt-get install -y curl libpq-dev && rm -rf /var/lib/apt/lists/*
COPY --from=build /usr/local/bundle /usr/local/bundle
COPY --from=build /rails /rails
RUN useradd rails --create-home && chown -R rails:rails db log storage tmp
USER rails:rails
EXPOSE 3000
CMD ["./bin/rails", "server"]
```

## deploy.yml Setup

```yaml
# config/deploy.yml
service: myapp
image: username/myapp

servers:
  web:
    hosts: [192.168.1.1]
    labels:
      traefik.http.routers.myapp.rule: Host(`myapp.com`)
      traefik.http.routers.myapp.tls.certresolver: letsencrypt
  job:
    hosts: [192.168.1.2]
    cmd: bundle exec solid_queue:start

proxy:
  ssl: true
  host: myapp.com

registry:
  username: username
  password: [KAMAL_REGISTRY_PASSWORD]

env:
  clear:
    RAILS_LOG_TO_STDOUT: true
  secret: [RAILS_MASTER_KEY, DATABASE_URL]

volumes:
  - "myapp_storage:/rails/storage"

healthcheck:
  path: /up
  port: 3000
```

## Environment & Secrets

```bash
# .kamal/secrets (git-ignored)
KAMAL_REGISTRY_PASSWORD=ghp_xxxxx
RAILS_MASTER_KEY=xxxxx
DATABASE_URL=postgres://user:pass@db.example.com/myapp_production

# Push secrets to servers
kamal env push
```

## Health Checks

```ruby
# Built-in: config/routes.rb
get "up" => "rails/health#show", as: :rails_health_check

# Custom check (optional)
class HealthController < ApplicationController
  skip_before_action :require_authentication

  def show
    render json: {
      status: "ok",
      db: ActiveRecord::Base.connection.active?,
      queue: SolidQueue::Job.any?
    }
  end
end
```

## Zero-Downtime Deployments

Kamal handles rolling deployments. Keep migrations backwards-compatible:

```ruby
# Step 1: Add new column (deploy)
class AddStatusToOrders < ActiveRecord::Migration[8.0]
  def change
    add_column :orders, :status, :string, default: "pending"
  end
end

# Step 2: Remove old column in subsequent deploy
class RemoveOldStatusFromOrders < ActiveRecord::Migration[8.0]
  def change
    remove_column :orders, :legacy_status
  end
end
```

## SSL/HTTPS Setup

```yaml
# config/deploy.yml - Traefik with Let's Encrypt
proxy:
  ssl: true
  host: myapp.com

traefik:
  args:
    certificatesResolvers.letsencrypt.acme.email: "admin@myapp.com"
    certificatesResolvers.letsencrypt.acme.storage: "/letsencrypt/acme.json"
```

## Common Commands

```bash
kamal app exec 'bin/rails c'    # Rails console
kamal app logs -f               # Follow logs
kamal lock release              # Release stuck lock
kamal audit                     # Check configuration
```

## Production Checklist

- [ ] Docker builds locally: `docker build .`
- [ ] `RAILS_MASTER_KEY` in `.kamal/secrets`
- [ ] Database accessible from server
- [ ] DNS pointing to server IP
- [ ] SSH access configured
- [ ] Health check returns 200
- [ ] SSL certificate configured
