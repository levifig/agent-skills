# Rails Styling with TailwindCSS

Utility-first CSS. No custom stylesheets. Let Tailwind handle the design system.

## Core Philosophy

- **Utility-first** - Compose styles from utility classes
- **Mobile-first** - Design for small screens, enhance for larger
- **No custom CSS** - Use utilities before writing new styles
- **Systematic design** - Leverage Tailwind's spacing/color scales

## Rails 8 Integration

TailwindCSS runs via standalone CLI (no Node.js required).

```bash
bin/rails tailwindcss:install
bin/dev  # Watches with Procfile.dev
```

```erb
<%= stylesheet_link_tag "tailwind", "data-turbo-track": "reload" %>
```

## Utility-First in Practice

```erb
<div class="bg-white rounded-lg shadow-md p-6 border border-gray-200">
  <h2 class="text-xl font-semibold text-gray-900 mb-2">Title</h2>
  <p class="text-gray-600 leading-relaxed">Content here.</p>
</div>
```

## Responsive Design

Mobile-first with breakpoint prefixes: `sm:`, `md:`, `lg:`, `xl:`, `2xl:`

```erb
<div class="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-6">
  <h3 class="text-lg md:text-xl lg:text-2xl">Responsive heading</h3>
</div>
```

## Dark Mode

```javascript
// tailwind.config.js
module.exports = { darkMode: 'class' }  // or 'media' for system preference
```

```erb
<div class="bg-white dark:bg-gray-900 text-gray-900 dark:text-gray-100">
  <button class="bg-blue-600 dark:bg-blue-500 hover:bg-blue-700 dark:hover:bg-blue-400">
    Works in both modes
  </button>
</div>
```

## Component Extraction

Extract only when patterns repeat 3+ times. Use `@apply` sparingly.

```css
/* app/assets/stylesheets/application.tailwind.css */
@tailwind base;
@tailwind components;
@tailwind utilities;

@layer components {
  .btn {
    @apply px-4 py-2 rounded-lg font-medium transition-colors;
  }
  .btn-primary {
    @apply btn bg-blue-600 text-white hover:bg-blue-700;
  }
  .card {
    @apply bg-white rounded-lg shadow-md border border-gray-200 p-6;
  }
}
```

**Prefer partials over `@apply`:**

```erb
<%# app/views/shared/_button.html.erb %>
<%= link_to text, path,
    class: "px-4 py-2 bg-blue-600 text-white rounded-lg hover:bg-blue-700" %>
```

## Form Styling

```erb
<%= form_with model: @post, class: "space-y-6" do |f| %>
  <div>
    <%= f.label :title,
        class: "block text-sm font-medium text-gray-700 dark:text-gray-300" %>
    <%= f.text_field :title,
        class: "mt-1 block w-full rounded-md border-gray-300 shadow-sm
               focus:border-blue-500 focus:ring-blue-500
               dark:bg-gray-800 dark:border-gray-600" %>
  </div>

  <%= f.submit "Save",
      class: "w-full bg-blue-600 text-white py-2 px-4 rounded-lg
             hover:bg-blue-700 focus:ring-2 focus:ring-blue-500 focus:ring-offset-2" %>
<% end %>
```

## Custom Configuration

```javascript
// tailwind.config.js
module.exports = {
  content: [
    './app/views/**/*.erb',
    './app/helpers/**/*.rb',
    './app/javascript/**/*.js'
  ],
  theme: {
    extend: {
      colors: {
        brand: { 500: '#6366f1', 600: '#4f46e5' }
      },
      fontFamily: {
        sans: ['Inter var', 'system-ui', 'sans-serif']
      },
    },
  },
}
```

## View Helpers for Styles

```ruby
# app/helpers/application_helper.rb
def status_badge(status)
  colors = {
    active: "bg-green-100 text-green-800",
    pending: "bg-yellow-100 text-yellow-800",
    inactive: "bg-gray-100 text-gray-800"
  }
  tag.span status.titleize,
    class: "px-2 py-1 text-xs font-medium rounded-full #{colors[status.to_sym]}"
end
```

## Best Practices

- Design mobile-first, enhance for desktop
- Use Tailwind's spacing scale (`p-4`, `mt-6`, not arbitrary values)
- Test both light and dark modes
- Use `focus:ring` for keyboard accessibility
- Use `transition-colors` for hover states
