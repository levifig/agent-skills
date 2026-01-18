# Rails Controllers

Patterns for HTTP request handling and response formatting.

## Core Principles

- Controllers handle HTTP only - no business logic
- Keep actions under 10 lines
- One controller per resource
- Seven RESTful actions maximum

## Controller Template

```ruby
class PostsController < ApplicationController
  before_action :authenticate_user!
  before_action :set_post, only: %i[show edit update destroy]

  def index
    @posts = Post.published.recent.page(params[:page])
  end

  def show; end

  def new
    @post = current_user.posts.build
  end

  def create
    @post = current_user.posts.build(post_params)
    if @post.save
      redirect_to @post, notice: "Post created."
    else
      render :new, status: :unprocessable_entity
    end
  end

  def edit; end

  def update
    if @post.update(post_params)
      redirect_to @post, notice: "Post updated."
    else
      render :edit, status: :unprocessable_entity
    end
  end

  def destroy
    @post.destroy
    redirect_to posts_path, notice: "Post deleted."
  end

  private

  def set_post
    @post = Post.find(params[:id])
  end

  def post_params
    params.require(:post).permit(:title, :body, :published)
  end
end
```

## Strong Parameters

```ruby
# Simple params
def user_params
  params.require(:user).permit(:name, :email)
end

# Nested attributes
def order_params
  params.require(:order).permit(
    :notes,
    line_items_attributes: [:id, :product_id, :quantity, :_destroy]
  )
end
```

## Response Formats

```ruby
def create
  @post = current_user.posts.build(post_params)

  respond_to do |format|
    if @post.save
      format.html { redirect_to @post, notice: "Created." }
      format.turbo_stream
      format.json { render :show, status: :created }
    else
      format.html { render :new, status: :unprocessable_entity }
      format.json { render json: @post.errors, status: :unprocessable_entity }
    end
  end
end
```

## Error Handling

```ruby
class ApplicationController < ActionController::Base
  rescue_from ActiveRecord::RecordNotFound, with: :not_found

  private

  def not_found
    respond_to do |format|
      format.html { render "errors/not_found", status: :not_found }
      format.json { render json: { error: "Not found" }, status: :not_found }
    end
  end
end
```

## Redirect vs Render

| Situation | Use | Status |
|-----------|-----|--------|
| Successful mutation | `redirect_to` | 302/303 |
| Validation failure | `render` | 422 |
| Not found | `render` | 404 |

```ruby
redirect_to @post, notice: "Saved."                    # After successful mutation
render :edit, status: :unprocessable_entity            # On validation failure
flash.now[:alert] = "Error"; render :new, status: 422  # flash.now with render
```

## Filters

```ruby
class ApplicationController < ActionController::Base
  before_action :authenticate_user!
  before_action :set_locale

  private

  def set_locale
    I18n.locale = params[:locale] || I18n.default_locale
  end
end

class Admin::BaseController < ApplicationController
  before_action :require_admin

  private

  def require_admin
    redirect_to root_path unless current_user&.admin?
  end
end
```
