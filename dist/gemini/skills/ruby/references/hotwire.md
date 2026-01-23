# Hotwire Guide

HTML over the wire. SPA-like speed without JavaScript complexity.

## Core Philosophy

- **Send HTML, not JSON** - Server renders, browser displays
- **Progressive enhancement** - Works without JavaScript
- **Minimal client-side logic** - Keep complexity on the server

## Turbo Drive (Automatic)

Intercepts all clicks and form submissions automatically.

```erb
<%# Opt out for external links %>
<%= link_to "External", url, data: { turbo: false } %>

<%# Persist elements across navigation %>
<audio id="player" data-turbo-permanent>...</audio>

<%# Replace instead of push %>
<%= link_to "Replace", path, data: { turbo_action: "replace" } %>
```

## Turbo Frames

Scope navigation to specific page sections.

```erb
<%# Basic frame %>
<%= turbo_frame_tag "posts" do %>
  <%= render @posts %>
  <%= link_to "Load More", posts_path(page: 2) %>
<% end %>

<%# Lazy loading %>
<%= turbo_frame_tag "comments", src: comments_path, loading: :lazy do %>
  <p>Loading...</p>
<% end %>

<%# Break out of frame %>
<%= link_to "View All", posts_path, data: { turbo_frame: "_top" } %>
```

## Turbo Streams

Update multiple elements in one response.

```ruby
# Controller
def create
  @comment = @post.comments.create!(comment_params)
  respond_to do |format|
    format.turbo_stream
    format.html { redirect_to @post }
  end
end
```

```erb
<%# app/views/comments/create.turbo_stream.erb %>
<%= turbo_stream.append "comments", @comment %>
<%= turbo_stream.update "comment_count", @post.comments.count %>
```

**Stream Actions:** `append`, `prepend`, `replace`, `update`, `remove`

### Real-Time Broadcasts

```ruby
class Comment < ApplicationRecord
  after_create_commit -> { broadcast_append_to post, target: "comments" }
  after_destroy_commit -> { broadcast_remove_to post }
end
```

```erb
<%= turbo_stream_from @post %>
<div id="comments"><%= render @post.comments %></div>
```

## Stimulus Controllers

Declarative JavaScript behavior via data attributes.

```javascript
// app/javascript/controllers/toggle_controller.js
import { Controller } from "@hotwired/stimulus"

export default class extends Controller {
  static targets = ["content"]
  static values = { open: Boolean }

  toggle() {
    this.openValue = !this.openValue
  }

  openValueChanged() {
    this.contentTarget.classList.toggle("hidden", !this.openValue)
  }
}
```

```erb
<div data-controller="toggle">
  <button data-action="toggle#toggle">Toggle</button>
  <div data-toggle-target="content" class="hidden">Content</div>
</div>
```

**Naming Convention:**
- Controller: `clipboard_controller.js` -> `data-controller="clipboard"`
- Target: `sourceTarget` -> `data-clipboard-target="source"`
- Action: `copy()` -> `data-action="click->clipboard#copy"`

## Decision Guide

| Need | Solution |
|------|----------|
| Faster navigation | Turbo Drive (automatic) |
| Update one section | Turbo Frame |
| Update multiple elements | Turbo Stream |
| Real-time from server | Turbo Stream + broadcasts |
| Client-side interactivity | Stimulus |

## Common Patterns

**Infinite Scroll:**
```erb
<%= turbo_frame_tag "posts_page_#{@page}" do %>
  <%= render @posts %>
  <% if @posts.any? %>
    <%= turbo_frame_tag "posts_page_#{@page + 1}",
        src: posts_path(page: @page + 1), loading: :lazy %>
  <% end %>
<% end %>
```

**Flash Messages:**
```erb
<%= turbo_stream.update "flash", partial: "shared/flash" %>
```
