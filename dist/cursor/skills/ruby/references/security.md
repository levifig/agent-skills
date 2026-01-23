# Rails Security

Defense-in-depth security patterns following Rails conventions.

## Rails 8 Built-in Authentication

```bash
bin/rails generate authentication
```

Creates User model with `has_secure_password`, Session model, controllers, and views.

```ruby
class User < ApplicationRecord
  has_secure_password
  has_many :sessions, dependent: :destroy
  normalizes :email_address, with: ->(e) { e.strip.downcase }
end
```

## Authorization Patterns

```ruby
class ApplicationController < ActionController::Base
  before_action :require_authentication

  private

  def require_authentication
    resume_session || request_authentication
  end
end

# Admin-only area
class Admin::BaseController < ApplicationController
  before_action -> { redirect_to root_path unless Current.user&.admin? }
end

# Resource-level authorization
def authorize_post
  redirect_to posts_path unless @post.user == Current.user
end
```

## Strong Parameters

```ruby
def user_params
  params.require(:user).permit(:name, :email, :password, :password_confirmation)
  # Never permit :role, :admin for public signup
end

def post_params
  params.require(:post).permit(:title, :body,
    comments_attributes: [:id, :body, :_destroy])
end
```

## CSRF Protection

Enabled by default. Skip only for token-authenticated APIs:

```ruby
class Api::BaseController < ApplicationController
  skip_forgery_protection
  before_action :authenticate_api_token
end
```

## SQL Injection Prevention

```ruby
# Safe (parameterized)
User.where(email: params[:email])
User.where("email = ?", params[:email])
User.where("email = :email", email: params[:email])

# DANGEROUS - Never do this
User.where("email = '#{params[:email]}'")  # SQL injection!
```

## XSS Prevention

```erb
<%# Safe - auto-escaped %>
<%= @user.name %>

<%# Dangerous - only for trusted content %>
<%= raw @post.body %>

<%# Sanitize user HTML %>
<%= sanitize @post.body, tags: %w[p br strong em a], attributes: %w[href] %>
```

## Credentials Management

```bash
EDITOR="code --wait" bin/rails credentials:edit
EDITOR="code --wait" bin/rails credentials:edit --environment production
```

```ruby
# Access credentials
Rails.application.credentials.stripe[:secret_key]
Rails.application.credentials.dig(:aws, :access_key_id)
```

## Security Headers

```ruby
# config/initializers/content_security_policy.rb
Rails.application.configure do
  config.content_security_policy do |policy|
    policy.default_src :self
    policy.script_src  :self
    policy.style_src   :self, :unsafe_inline
    policy.img_src     :self, :data, "https:"
  end
  config.content_security_policy_nonce_generator = ->(req) { SecureRandom.base64(16) }
end

# config/environments/production.rb
config.force_ssl = true
config.ssl_options = { hsts: { subdomains: true, preload: true, expires: 1.year } }
```

## Session Security

```ruby
Rails.application.config.session_store :cookie_store,
  key: '_app_session',
  secure: Rails.env.production?,
  httponly: true,
  same_site: :lax
```

## Security Checklist

- [ ] `has_secure_password` for user authentication
- [ ] Strong parameters for all user input
- [ ] Parameterized SQL queries only
- [ ] Credentials in `credentials.yml.enc`
- [ ] Force SSL in production
- [ ] CSRF protection enabled (or token auth for APIs)
- [ ] Content Security Policy configured
