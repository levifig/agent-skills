# Rails Assets

No build step. No Node.js. Ship JavaScript and CSS directly to browsers.

## Core Philosophy

- **No compilation** - Browsers handle ES modules natively
- **No bundling** - HTTP/2 multiplexing makes it unnecessary
- **No transpilation** - Modern JavaScript works everywhere

## Propshaft vs Sprockets

| Feature | Sprockets | Propshaft |
|---------|-----------|-----------|
| Compilation | Yes (Sass, CoffeeScript) | No |
| Bundling | Yes | No |
| Fingerprinting | Yes | Yes |
| Complexity | High | Minimal |

**Use Propshaft** (Rails 8 default). It just serves files with fingerprinted digests.

## Import Maps

### Configuration

```ruby
# config/importmap.rb
pin "application"
pin "@hotwired/turbo-rails", to: "turbo.min.js", preload: true
pin "@hotwired/stimulus", to: "stimulus.min.js", preload: true
pin_all_from "app/javascript/controllers", under: "controllers"
```

### Adding npm Packages (No Node.js)

```bash
bin/importmap pin lodash-es              # Pin from jsDelivr CDN
bin/importmap pin lodash-es@4.17.21      # Pin specific version
bin/importmap pin lodash-es --download   # Download locally (vendor)
```

```ruby
# Or pin manually in config/importmap.rb
pin "alpinejs", to: "https://cdn.jsdelivr.net/npm/alpinejs@3.14.0/dist/module.esm.js"
pin "chart.js", to: "chart.js"  # vendored in vendor/javascript/
```

### Using Modules

```javascript
// app/javascript/application.js
import "@hotwired/turbo-rails"
import { Application } from "@hotwired/stimulus"
import { eagerLoadControllersFrom } from "@hotwired/stimulus-loading"

const application = Application.start()
eagerLoadControllersFrom("controllers", application)
```

## File Organization

```
app/
├── assets/
│   └── stylesheets/
│       ├── application.css
│       └── application.tailwind.css
├── javascript/
│   ├── application.js
│   └── controllers/
│       ├── index.js
│       └── hello_controller.js
vendor/
└── javascript/          # Vendored packages
```

## CSS with Tailwind

Rails 8 includes Tailwind via standalone CLI (no Node.js).

```bash
bin/rails tailwindcss:install
```

```css
/* app/assets/stylesheets/application.tailwind.css */
@tailwind base;
@tailwind components;
@tailwind utilities;

@layer components {
  .btn-primary {
    @apply px-4 py-2 bg-blue-600 text-white rounded-lg hover:bg-blue-700;
  }
}
```

## Layout Integration

```erb
<%# app/views/layouts/application.html.erb %>
<head>
  <%= stylesheet_link_tag "tailwind", "inter-font", "data-turbo-track": "reload" %>
  <%= javascript_importmap_tags %>
</head>
```

## Decision Guide

| Scenario | Approach |
|----------|----------|
| Popular library | Pin from CDN |
| Less common package | Download and vendor |
| CSS framework | Tailwind standalone |
| Need Sass | Add `dartsass-rails` gem |

## Common Commands

```bash
bin/importmap pin <package>             # Add from CDN
bin/importmap pin <package> --download  # Download locally
bin/importmap unpin <package>           # Remove
bin/importmap audit                     # Check for outdated
```

## Best Practices

- Pin to specific versions in production
- Preload only essential modules
- Use `data-turbo-track: "reload"` for stylesheets
- Never install Node.js for standard Rails apps
- Let browsers handle ES modules
