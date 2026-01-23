# Rails Views

Patterns for templates, partials, layouts, and presentation logic.

## Core Principles

- Views display data only - no business logic
- Use partials for reusable components
- Pass locals explicitly to partials
- Leverage Rails helpers over raw HTML

## Template Organization

```
app/views/
├── layouts/
│   └── application.html.erb
├── posts/
│   ├── index.html.erb
│   ├── show.html.erb
│   ├── _form.html.erb
│   └── _post.html.erb
└── shared/
    ├── _flash.html.erb
    └── _header.html.erb
```

## Partials

```erb
<%# Explicit locals (preferred) %>
<%= render "posts/post", post: @post %>

<%# Collection rendering %>
<%= render @posts %>
<%= render partial: "posts/post", collection: @posts %>
```

```erb
<%# app/views/posts/_post.html.erb %>
<article id="<%= dom_id(post) %>" class="post">
  <h2><%= link_to post.title, post %></h2>
  <p><%= truncate(post.body, length: 200) %></p>
</article>
```

## Layouts and content_for

```erb
<%# app/views/layouts/application.html.erb %>
<!DOCTYPE html>
<html>
<head>
  <title><%= content_for(:title) || "App" %></title>
  <%= csrf_meta_tags %>
  <%= stylesheet_link_tag "tailwind", "data-turbo-track": "reload" %>
  <%= javascript_importmap_tags %>
  <%= yield :head %>
</head>
<body>
  <%= render "shared/flash" %>
  <%= yield %>
</body>
</html>
```

```erb
<%# In view template %>
<% content_for :title, @post.title %>
<% content_for :head do %>
  <%= tag.meta name: "description", content: @post.excerpt %>
<% end %>
```

## View Helpers

```ruby
# app/helpers/posts_helper.rb
module PostsHelper
  def post_status_badge(post)
    status = post.published? ? "published" : "draft"
    tag.span status.titleize, class: "badge badge--#{status}"
  end

  def reading_time(text)
    "#{(text.split.size / 200.0).ceil} min read"
  end
end
```

## Form Helpers

```erb
<%= form_with model: post do |f| %>
  <% if post.errors.any? %>
    <div class="errors">
      <% post.errors.full_messages.each do |msg| %>
        <p><%= msg %></p>
      <% end %>
    </div>
  <% end %>

  <div>
    <%= f.label :title %>
    <%= f.text_field :title, required: true %>
  </div>

  <div>
    <%= f.label :category_id %>
    <%= f.collection_select :category_id, Category.all, :id, :name, prompt: "Select..." %>
  </div>

  <div>
    <%= f.check_box :published %>
    <%= f.label :published, "Publish now" %>
  </div>

  <%= f.submit %>
<% end %>
```

## Rendering Patterns

```ruby
render :show                                    # View template
render partial: "form"                          # Partial
render json: @post                              # JSON
render turbo_stream: turbo_stream.remove(@post) # Turbo Stream
```

## Keeping Logic Out of Views

| Bad | Good |
|-----|------|
| `<% if current_user && current_user.admin? %>` | `<% if admin? %>` (helper) |
| `<%= post.created_at.strftime("%B %d") %>` | `<%= format_date(post.created_at) %>` |
| `<%= Post.where(published: true).count %>` | `<%= @published_count %>` (controller) |

For complex display logic, use presenters (SimpleDelegator subclass).
