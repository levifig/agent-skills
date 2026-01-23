---
name: design
description: >-
  Use for UI/UX design and accessibility work. Covers design principles, color
  theory with WCAG contrast requirements, typography scales, 8pt spacing grid,
  responsive breakpoints, WCAG 2.1 accessibility compliance, motion design, and
  design system governance. Activate when working on user interfaces, design
  tokens, or accessibility audits.
---

# Design Principles

**Philosophy**: Design is problem-solving through systematic thinking, with accessibility and user needs at the center. Every design decision must be intentional, accessible, and meaningful.

## Quick Reference

| Topic | Key Principle | Reference |
|-------|--------------|-----------|
| Core | User-first, accessible by default, consistent, minimal | [reference/core.md](reference/core.md) |
| Color | Function first, 4.5:1 contrast minimum, never color-only | [reference/color.md](reference/color.md) |
| Typography | Legible, accessible, reinforce hierarchy | [reference/typography.md](reference/typography.md) |
| Spacing | 8pt grid, consistent rhythm, logical properties | [reference/spacing.md](reference/spacing.md) |
| Responsive | Mobile-first, content-out, fluid layouts | [reference/responsive.md](reference/responsive.md) |
| Accessibility | WCAG 2.1 AA minimum, keyboard navigable, screen reader compatible | [reference/a11y.md](reference/a11y.md) |
| Motion | Purposeful, natural, respect reduced motion | [reference/motion.md](reference/motion.md) |
| Systems | Tokens, components, patterns, governance | [reference/systems.md](reference/systems.md) |

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

## Critical Rules

1. **Never use pixel values directly** - Always use design tokens
2. **Never use divs for buttons** - Use semantic HTML
3. **Never skip alt text** - All images need descriptions
4. **Never use color alone** - Combine with icons/text
5. **Never ignore reduced motion** - Respect user preferences
6. **Never animate layout properties** - Use transform/opacity only
7. **Never skip focus indicators** - Minimum 3:1 contrast
8. **Never exceed 75ch line length** - Optimal readability at 65ch

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
