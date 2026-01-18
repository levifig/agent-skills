# Rails API

Patterns for building robust, versioned JSON APIs.

## Core Principles

- Version from day one - use URL path versioning (`/api/v1`)
- Keep responses consistent and predictable
- Never break backward compatibility

## Versioning Setup

```ruby
# config/routes.rb
namespace :api do
  namespace :v1 do
    resources :posts, only: %i[index show create update destroy]
  end
end
```

## Base API Controller

```ruby
# app/controllers/api/v1/base_controller.rb
module Api::V1
  class BaseController < ActionController::API
    include ActionController::HttpAuthentication::Token::ControllerMethods

    before_action :authenticate_token!

    rescue_from ActiveRecord::RecordNotFound, with: :not_found
    rescue_from ActiveRecord::RecordInvalid, with: :unprocessable

    private

    def authenticate_token!
      authenticate_or_request_with_http_token do |token, _options|
        @current_user = User.find_by(api_token: token)
      end
    end

    def current_user
      @current_user
    end

    def not_found
      render json: { error: "Not found" }, status: :not_found
    end

    def unprocessable(exception)
      render json: { errors: exception.record.errors }, status: :unprocessable_entity
    end
  end
end
```

## Resource Controller

```ruby
# app/controllers/api/v1/posts_controller.rb
module Api::V1
  class PostsController < BaseController
    before_action :set_post, only: %i[show update destroy]

    def index
      @posts = Post.published.page(params[:page])
      render json: {
        data: @posts.as_json(only: %i[id title created_at]),
        meta: { page: @posts.current_page, total_pages: @posts.total_pages }
      }
    end

    def show
      render json: { data: @post.as_json(only: %i[id title body]) }
    end

    def create
      @post = current_user.posts.create!(post_params)
      render json: { data: @post }, status: :created
    end

    def update
      @post.update!(post_params)
      render json: { data: @post }
    end

    def destroy
      @post.destroy
      head :no_content
    end

    private

    def set_post
      @post = Post.find(params[:id])
    end

    def post_params
      params.require(:post).permit(:title, :body)
    end
  end
end
```

## HTTP Status Codes

| Action | Success | Failure |
|--------|---------|---------|
| GET | 200 OK | 404 Not Found |
| POST | 201 Created | 422 Unprocessable |
| PUT/PATCH | 200 OK | 422 Unprocessable |
| DELETE | 204 No Content | 404 Not Found |

## Token Authentication

```ruby
class User < ApplicationRecord
  before_create { self.api_token = SecureRandom.hex(32) }
end

# Request header: Authorization: Token token="abc123..."
```

## Rate Limiting

```ruby
# config/initializers/rack_attack.rb
class Rack::Attack
  throttle("api/ip", limit: 100, period: 1.minute) do |req|
    req.ip if req.path.start_with?("/api")
  end
end
```

## JSON Serialization Options

```ruby
# In model
def as_api_json
  as_json(only: %i[id title body], methods: [:author_name])
end

# With jbuilder (app/views/api/v1/posts/show.json.jbuilder)
json.data do
  json.extract! @post, :id, :title, :body
  json.author @post.user.name
end
```

## Pagination

```ruby
# Using kaminari or pagy
def index
  @posts = Post.page(params[:page]).per(25)
  render json: {
    data: @posts,
    meta: {
      current_page: @posts.current_page,
      total_pages: @posts.total_pages,
      total_count: @posts.total_count
    }
  }
end
```
