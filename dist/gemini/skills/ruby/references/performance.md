# Rails Performance

Measure first, optimize what matters, keep solutions maintainable.

## N+1 Query Prevention

### Bullet Gem Setup

```ruby
# Gemfile (development)
gem "bullet", group: :development

# config/environments/development.rb
config.after_initialize do
  Bullet.enable = true
  Bullet.alert = true
  Bullet.rails_logger = true
end
```

### Loading Strategies

| Method | Use Case | Behavior |
|--------|----------|----------|
| `includes` | General preloading | Smart: JOIN or separate |
| `preload` | Force separate queries | Multiple SELECTs |
| `eager_load` | Force single query | LEFT OUTER JOIN |

```ruby
# BAD: N+1 queries
Post.all.each { |post| post.author.name }

# GOOD: Preload associations
Post.includes(:author).each { |post| post.author.name }

# Nested associations
Post.includes(comments: :author).find(params[:id])
```

## Database Indexing

```ruby
class AddIndexes < ActiveRecord::Migration[8.0]
  def change
    add_index :posts, :user_id                          # Foreign key
    add_index :posts, [:user_id, :published_at]         # Composite
    add_index :posts, :published_at, where: "published" # Partial
  end
end
```

**Index Guidelines:**
- Always index foreign keys
- Index columns used in WHERE and ORDER BY
- Consider composite indexes for common query patterns
- Remove unused indexes (they slow writes)

## Caching Strategies

### Fragment Caching

```erb
<% cache @post do %>
  <%= render @post %>
<% end %>

<%# Collection caching %>
<%= render partial: "posts/post", collection: @posts, cached: true %>
```

### Russian Doll Caching

```ruby
# Model with touch for auto-expiration
class Comment < ApplicationRecord
  belongs_to :post, touch: true
end
```

```erb
<% cache @post do %>
  <h1><%= @post.title %></h1>
  <% @post.comments.each do |comment| %>
    <% cache comment do %>
      <%= render comment %>
    <% end %>
  <% end %>
<% end %>
```

### Low-Level Caching

```ruby
def expensive_calculation
  Rails.cache.fetch("#{cache_key_with_version}/calc") do
    compute_something_heavy
  end
end

# With expiration
Rails.cache.fetch("stats", expires_in: 1.hour) { compute_stats }
```

## Query Optimization

```ruby
# Select only needed columns
User.select(:id, :name, :email)
User.where(active: true).pluck(:email)  # Single column array

# Batch processing for large datasets
User.find_each(batch_size: 1000) { |u| process(u) }

# Counter caches avoid COUNT queries
belongs_to :post, counter_cache: true
```

## Profiling

```ruby
# Gemfile
gem "rack-mini-profiler"

# Query analysis
User.where(active: true).explain

# Benchmarking
require "benchmark"
Benchmark.bm do |x|
  x.report("v1") { 1000.times { method_1 } }
  x.report("v2") { 1000.times { method_2 } }
end
```

## Performance Checklist

- [ ] Foreign keys indexed
- [ ] N+1 queries eliminated (use Bullet)
- [ ] Large datasets paginated
- [ ] Expensive views cached
- [ ] Counter caches for counts
- [ ] Batch processing for bulk operations
- [ ] Query plans reviewed for slow queries

## Common Patterns

```ruby
# Pagination
@posts = Post.published.page(params[:page]).per(25)

# Avoid loading all records
User.count  # Good: COUNT query
User.all.size  # Bad: loads all records

# Conditional eager loading
scope :with_details, -> { includes(:author, :comments) }
```
