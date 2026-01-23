# Accessibility Review Checklist

Comprehensive accessibility review checklist aligned with WCAG 2.1 AA.

## Quick Accessibility Check

For every UI change:

- [ ] Color contrast meets 4.5:1 ratio
- [ ] Interactive elements keyboard accessible
- [ ] Images have alt text
- [ ] Focus indicator visible

## WCAG 2.1 AA Checklist

### 1. Perceivable

#### 1.1 Text Alternatives

- [ ] **Images**: Alt text describes content or purpose
- [ ] **Decorative images**: Empty alt (`alt=""`)
- [ ] **Complex images**: Long description available
- [ ] **Icons**: Accessible name via aria-label or text

```tsx
// Informative image
<img src="chart.png" alt="Sales grew 25% in Q4 2023" />

// Decorative image
<img src="decoration.png" alt="" role="presentation" />

// Icon button
<button aria-label="Close dialog">
  <CloseIcon aria-hidden="true" />
</button>
```

#### 1.2 Time-based Media

- [ ] **Video**: Captions provided
- [ ] **Audio**: Transcript available
- [ ] **Auto-play**: Can be paused or stopped

#### 1.3 Adaptable

- [ ] **Semantic HTML**: Proper elements (button, nav, main)
- [ ] **Heading hierarchy**: h1 > h2 > h3 (no skipping)
- [ ] **Lists**: ul/ol for list content
- [ ] **Tables**: Headers marked with th, scope defined
- [ ] **Landmarks**: main, nav, aside, footer present

```tsx
// Semantic structure
<main>
  <h1>Page Title</h1>
  <nav aria-label="Main navigation">...</nav>
  <article>
    <h2>Section Title</h2>
    <p>Content...</p>
  </article>
</main>
```

#### 1.4 Distinguishable

- [ ] **Color contrast**: 4.5:1 for normal text, 3:1 for large text
- [ ] **Color not sole indicator**: Icons, text, or patterns supplement
- [ ] **Resize**: Content usable at 200% zoom
- [ ] **Text spacing**: Works with 1.5 line height, 2x letter spacing
- [ ] **Content on hover/focus**: Dismissible, hoverable, persistent

### 2. Operable

#### 2.1 Keyboard Accessible

- [ ] **All functions keyboard accessible**
- [ ] **No keyboard traps**: Can navigate away
- [ ] **Focus order logical**: Tab order follows visual order
- [ ] **Focus visible**: Clear focus indicator

```tsx
// Custom focus styles
<button className="focus-visible:ring-2 focus-visible:ring-blue-500 focus-visible:ring-offset-2">
  Click me
</button>

// Focus management for modals
<dialog ref={dialogRef} onOpen={() => dialogRef.current?.focus()}>
  <button autoFocus>First focusable element</button>
</dialog>
```

#### 2.2 Enough Time

- [ ] **Adjustable timing**: Can extend time limits
- [ ] **Pause/stop**: Moving content can be paused
- [ ] **No timing**: Essential functions don't time out

#### 2.3 Seizures and Physical Reactions

- [ ] **No flashing**: Content doesn't flash > 3 times/second
- [ ] **Animation reducible**: Respects prefers-reduced-motion

```css
@media (prefers-reduced-motion: reduce) {
  *,
  *::before,
  *::after {
    animation-duration: 0.01ms !important;
    animation-iteration-count: 1 !important;
    transition-duration: 0.01ms !important;
  }
}
```

#### 2.4 Navigable

- [ ] **Skip link**: Skip to main content
- [ ] **Page titled**: Descriptive title
- [ ] **Focus order**: Meaningful sequence
- [ ] **Link purpose**: Clear from text or context

```tsx
// Skip link
<a href="#main" className="skip-link">Skip to main content</a>

// Link with clear purpose
<a href="/report.pdf">Download 2023 Annual Report (PDF, 2MB)</a>
```

#### 2.5 Input Modalities

- [ ] **Pointer gestures**: Single pointer alternative available
- [ ] **Touch targets**: 44x44 CSS pixels minimum
- [ ] **Motion actuation**: Can disable or has alternative

### 3. Understandable

#### 3.1 Readable

- [ ] **Language defined**: html lang attribute set
- [ ] **Language of parts**: lang attribute for foreign text

```html
<html lang="en">
  <body>
    <p>Welcome to our site. <span lang="es">Bienvenidos</span></p>
  </body>
</html>
```

#### 3.2 Predictable

- [ ] **On focus**: No unexpected changes
- [ ] **On input**: No unexpected context changes
- [ ] **Consistent navigation**: Same order across pages
- [ ] **Consistent identification**: Same function = same name

#### 3.3 Input Assistance

- [ ] **Error identification**: Errors clearly described
- [ ] **Labels**: Form inputs have visible labels
- [ ] **Error suggestions**: Help text for fixing errors
- [ ] **Error prevention**: Confirmation for important actions

```tsx
<div>
  <label htmlFor="email">Email Address</label>
  <input
    id="email"
    type="email"
    aria-invalid={hasError}
    aria-describedby={hasError ? "email-error" : "email-hint"}
  />
  <span id="email-hint" className="hint">We'll never share your email.</span>
  {hasError && (
    <span id="email-error" role="alert" className="error">
      Please enter a valid email address, like name@example.com
    </span>
  )}
</div>
```

### 4. Robust

#### 4.1 Compatible

- [ ] **Valid HTML**: No parsing errors
- [ ] **Name, role, value**: Custom widgets have ARIA
- [ ] **Status messages**: aria-live for dynamic updates

```tsx
// Custom toggle with ARIA
<button
  role="switch"
  aria-checked={isOn}
  onClick={() => setIsOn(!isOn)}
>
  Dark mode
</button>

// Status message
<div aria-live="polite" aria-atomic="true">
  {message && <p>{message}</p>}
</div>
```

## Testing Tools

| Tool | Use |
|------|-----|
| axe DevTools | Automated testing |
| WAVE | Visual feedback |
| Lighthouse | General audit |
| VoiceOver/NVDA | Screen reader testing |
| Keyboard only | Tab through interface |

## Common Issues

### Forms

- Missing labels
- Error not announced
- Required not indicated
- Autocomplete missing

### Navigation

- No skip link
- Focus not visible
- Tab order illogical
- Keyboard traps

### Content

- Missing alt text
- Color-only feedback
- Low contrast
- Content hidden from AT

### Interactive Elements

- Non-focusable buttons (div, span)
- Missing ARIA on custom widgets
- Auto-play without controls
- Timeout without warning

## Testing Workflow

1. **Automated scan** - axe DevTools
2. **Keyboard navigation** - Tab through everything
3. **Screen reader** - Test with VoiceOver or NVDA
4. **Zoom** - Test at 200% zoom
5. **Color** - Check contrast, disable color

---

*Reference: [WCAG 2.1](https://www.w3.org/WAI/WCAG21/quickref/), [a11y project](https://www.a11yproject.com/checklist/)*
