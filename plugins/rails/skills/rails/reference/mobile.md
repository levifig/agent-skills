# Mobile Apps with Hotwire Native

Build native iOS and Android apps from your Rails web app. One codebase, three platforms.

## Core Philosophy

- **Web content in native shells** - WebView with native navigation
- **Build once, deploy everywhere** - Web, iOS, and Android from one codebase
- **Progressive enhancement** - Start web-only, add native features gradually

## How It Works

```
[Rails App] --> [WebView] --> [Native Shell (iOS/Android)]
                    ^
                    |
            Link clicks become
            native screen transitions
```

- Web content renders in a WebView with native-feeling transitions
- Link navigation is intercepted and handled by the native adapter
- External links open in SFSafariViewController (iOS) or Custom Tabs (Android)

## Path Configuration

Define platform-specific navigation rules in JSON.

```json
{
  "settings": {
    "feature_flags": [
      { "name": "enable_native_camera", "enabled": true }
    ]
  },
  "rules": [
    {
      "patterns": ["/modal/.*"],
      "properties": { "context": "modal" }
    },
    {
      "patterns": ["/tabs/.*"],
      "properties": { "presentation": "refresh" }
    },
    {
      "patterns": ["/external/.*"],
      "properties": { "presentation": "external" }
    }
  ]
}
```

Host at `/configurations/ios_v1.json` and `/configurations/android_v1.json`.

## Bridge Components

Web-to-native communication via Stimulus controllers with native counterparts.

```javascript
// app/javascript/controllers/native_button_controller.js
import { Controller } from "@hotwired/stimulus"
import { BridgeComponent } from "@hotwired/bridge"

export default class extends Controller {
  static values = { title: String, style: String }

  connect() {
    this.bridge = new BridgeComponent("button", this)
    this.bridge.send("connect", {
      title: this.titleValue,
      style: this.styleValue || "default"
    })
  }

  handleTap(message) {
    this.element.closest("form")?.requestSubmit()
  }
}
```

```erb
<div data-controller="native-button"
     data-native-button-title-value="Save"
     data-native-button-style-value="primary">
</div>
```

**Use cases:** Top bar buttons, action sheets, camera access, push notifications.

## Navigation Patterns

```erb
<%# Replace current screen instead of pushing %>
<%= link_to "Edit", edit_path, data: { turbo_action: "replace" } %>

<%# Ensure all screens are URL-accessible %>
<%= link_to "Profile", user_path(@user) %>
```

Every screen must have a corresponding URL for proper native navigation.

## Mobile-Optimized HTML

```erb
<%# app/views/layouts/application.html.erb %>
<meta name="viewport" content="width=device-width, initial-scale=1, user-scalable=no">
<meta name="apple-mobile-web-app-capable" content="yes">
<meta name="turbo-visit-control" content="reload">
```

**Requirements:**
- Minimum tap targets: 44x44pt (iOS), 48x48dp (Android)
- Initial page load under 2 seconds
- Responsive layouts for all viewports

## Progressive Enhancement Strategy

| Stage | Approach |
|-------|----------|
| 1. Web-only | Ship features fast with responsive HTML |
| 2. Bridge Components | Add native UI elements (buttons, menus) |
| 3. Native Screens | Build fully native only where required |

Use feature flags in path configuration to enable native features gradually.

## Performance Considerations

- Minimize JavaScript to conserve battery
- Optimize images for mobile networks
- Use Turbo Frames for incremental content loading
- Consider service workers for offline capabilities

## Testing

- Test on actual devices, not just simulators
- Verify deep linking works correctly
- Test with slow network conditions
- Verify native transitions feel smooth
