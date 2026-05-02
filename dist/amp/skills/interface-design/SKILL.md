---
name: interface-design
description: >-
  Covers UI/UX design principles, accessibility (WCAG 2.1), design systems, and
  responsive patterns. Use when making design decisions, evaluating layouts, or
  ensuring accessibility compliance. Not for frontend code (use
  typescript-development) or API design (use architecture or language-specific
  skills).
version: 2.0.0-dev.37
---

# Design Principles

## Contents
- Critical Rules
- Verification
- Quick Reference
- Topics
- Core Principles
- Design Tokens
- Quality Checklist

**Philosophy**: Design is problem-solving through systematic thinking, with accessibility and user needs at the center. Every design decision must be intentional, accessible, and meaningful.

## Critical Rules

1. **Never use pixel values directly** - Always use design tokens
2. **Never use divs for buttons** - Use semantic HTML
3. **Never skip alt text** - All images need descriptions
4. **Never use color alone** - Combine with icons/text
5. **Never ignore reduced motion** - Respect user preferences
6. **Never animate layout properties** - Use transform/opacity only
7. **Never skip focus indicators** - Minimum 3:1 contrast
8. **Never exceed 75ch line length** - Optimal readability at 65ch

## Verification

### After Editing Design/Code Files

**Accessibility Validation:**
- Check for common a11y issues in component files:
  - `<div onClick>` without `role="button"` (use `<button>` instead)
  - `<img>` without `alt` attribute
  - `<input>` without associated `<label>` or `aria-label`
  - Dialogs without `aria-modal="true"` and `aria-labelledby`
  - Icon-only buttons without `aria-label`
  - Missing focus-visible styles on interactive elements

**Design Token Compliance:**
- Check for hardcoded values in style files:
  - Hardcoded colors (`#hex`, `rgb()`, `hsl()`) not using `tokens.color.*`
  - Hardcoded spacing (`padding: 16px`) not using `tokens.spacing.*`
  - Hardcoded font sizes not using `typography.fontSize.*`
  - Hardcoded border-radius not using `primitives.borderRadius.*`
  - Hardcoded box-shadow not using `primitives.shadow.*`
  - Hardcoded transition durations not using `motion.duration.*`
  - Inline styles in JSX with hardcoded values

**Accessibility Audit (if ESLint jsx-a11y is available):**
- Run: `npx eslint --plugin jsx-a11y --rule 'jsx-a11y/*: error' {files}`
- Check for:
  - Images without alt attributes
  - Empty button elements (need text or aria-label)
  - More inputs than labels
  - Missing lang attribute on html element
  - Multiple h1 elements
  - Hardcoded colors (verify contrast ratio WCAG 4.5:1)

### Before Committing

- Verify WCAG 2.1 AA compliance (4.5:1 contrast, keyboard navigation)
- Check all images have alt text
- Verify design tokens are used (no hardcoded values)
- Test with keyboard-only navigation
- Run accessibility audit tools if available

## Quick Reference

| Topic | Key Principle | Reference |
|-------|--------------|-----------|
| Core | User-first, accessible by default, consistent, minimal | [references/core.md](references/core.md) |
| Color | Function first, 4.5:1 contrast minimum, never color-only | [references/color.md](references/color.md) |
| Typography | Legible, accessible, reinforce hierarchy | [references/typography.md](references/typography.md) |
| Spacing | 8pt grid, consistent rhythm, logical properties | [references/spacing.md](references/spacing.md) |
| Responsive | Mobile-first, content-out, fluid layouts | [references/responsive.md](references/responsive.md) |
| Accessibility | WCAG 2.1 AA minimum, keyboard navigable, screen reader compatible | [references/a11y.md](references/a11y.md) |
| Accessibility Review | WCAG 2.1 AA checklist, testing tools, common issues | [references/accessibility-review.md](references/accessibility-review.md) |
| Motion | Purposeful, natural, respect reduced motion | [references/motion.md](references/motion.md) |
| Systems | Tokens, components, patterns, governance | [references/systems.md](references/systems.md) |

## Topics

| Topic | Reference | Use When |
|-------|-----------|----------|
| Core | [references/core.md](references/core.md) | Foundational design decisions, user-first principles |
| Color | [references/color.md](references/color.md) | Choosing colors, contrast ratios, color accessibility |
| Typography | [references/typography.md](references/typography.md) | Font choices, hierarchy, readability |
| Spacing | [references/spacing.md](references/spacing.md) | Layout spacing, grid systems, rhythm |
| Responsive | [references/responsive.md](references/responsive.md) | Breakpoints, mobile-first, fluid layouts |
| Accessibility | [references/a11y.md](references/a11y.md) | WCAG compliance, keyboard navigation, screen readers |
| Accessibility Review | [references/accessibility-review.md](references/accessibility-review.md) | A11y audits, testing tools, common issues |
| Motion | [references/motion.md](references/motion.md) | Animations, transitions, reduced motion |
| Systems | [references/systems.md](references/systems.md) | Design tokens, component libraries, governance |

## Core Principles

### 1. User-First
Every design decision must serve user needs and goals.
- Understand user context: Who, what, why, when, where?
- Validate assumptions with real users
- Measure outcomes that matter to users

### 2. Accessible by Default
Accessibility is not a feature - it is a requirement.
- WCAG 2.1 AA minimum, AAA as aspiration
- Keyboard navigation for all functionality
- 4.5:1 contrast for text, 3:1 for UI components
- Never use color as the sole indicator

### 3. Consistent
Consistency reduces cognitive load and builds trust.
- Design tokens as single source of truth
- Component reuse over reinvention
- Predictable naming conventions

### 4. Minimal
Every element must justify its existence.
- Progressive disclosure
- Clear visual hierarchy
- Sufficient whitespace

### 5. Delightful
Good design feels effortless.
- Smooth, purposeful interactions
- Thoughtful details
- Performance as a feature

## Design Tokens

Use semantic tokens, never hardcoded values.

```typescript
// Pattern: [category]-[property]-[variant?]-[state?]
color-text-primary
color-background-secondary
spacing-component-padding-md
```

## Quality Checklist

Every design deliverable must meet:

- [ ] WCAG 2.1 AA compliant
- [ ] Keyboard navigable
- [ ] Screen reader compatible
- [ ] Color contrast verified (4.5:1 text, 3:1 UI)
- [ ] Focus indicators visible (3:1 contrast)
- [ ] Reduced motion respected
- [ ] Uses design tokens exclusively
- [ ] Touch targets minimum 44x44px
- [ ] Mobile responsive
- [ ] Documented in component library
