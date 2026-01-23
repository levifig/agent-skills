# Spacing

## 8-Point Grid System

```typescript
// Base unit: 8px
export const spacing = {
  0: "0",
  0.5: "0.125rem",  // 2px  - Hairline
  1: "0.25rem",     // 4px  - Minimal
  2: "0.5rem",      // 8px  - Base unit
  3: "0.75rem",     // 12px - Small
  4: "1rem",        // 16px - Standard
  5: "1.25rem",     // 20px - Medium
  6: "1.5rem",      // 24px - Comfortable
  8: "2rem",        // 32px - Large
  10: "2.5rem",     // 40px - Section
  12: "3rem",       // 48px - Large section
  16: "4rem",       // 64px - Hero
  20: "5rem",       // 80px - Massive
  24: "6rem",       // 96px - Layout
};
```

## Padding vs Margin

- **Padding**: Internal spacing (component owns this)
- **Margin**: External spacing (parent controls this)
- **Gap**: Prefer for flex/grid children

```css
/* Component owns internal spacing */
.card { padding: 1.5rem; }

/* Parent controls spacing between children */
.card-list { display: flex; gap: 2rem; }
```

## Vertical Rhythm

```typescript
// Base line height: 24px (1.5 x 16px)
const baseLineHeight = 24;

export const verticalRhythm = {
  1: baseLineHeight / 4,     // 6px
  2: baseLineHeight / 2,     // 12px
  4: baseLineHeight,         // 24px - 1x base
  6: baseLineHeight * 1.5,   // 36px
  8: baseLineHeight * 2,     // 48px - 2x base
};
```

## Grid Systems

### Auto-Responsive Grid

```css
.auto-grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(min(100%, 300px), 1fr));
  gap: 1.5rem;
}
```

### 12-Column Grid

```css
.grid-12 {
  display: grid;
  grid-template-columns: repeat(12, 1fr);
  gap: 1.5rem;
}

.span-6 { grid-column: span 6; }
.span-4 { grid-column: span 4; }
.span-3 { grid-column: span 3; }
```

## Component Spacing Tokens

```typescript
export const spacingTokens = {
  component: {
    padding: {
      sm: spacing[2],  // 8px
      md: spacing[4],  // 16px
      lg: spacing[6],  // 24px
    },
    gap: {
      sm: spacing[2],  // 8px
      md: spacing[4],  // 16px
      lg: spacing[6],  // 24px
    },
  },
  layout: {
    section: {
      sm: spacing[8],  // 32px
      md: spacing[12], // 48px
      lg: spacing[16], // 64px
    },
  },
};
```

## Logical Properties

Use for internationalization (RTL support).

```css
/* Adapts to RTL */
.card {
  padding-inline: 1.5rem;  /* left/right in LTR */
  padding-block: 1rem;     /* top/bottom */
  margin-inline-end: 1rem; /* right in LTR, left in RTL */
}
```

## Touch Target Spacing

```css
/* Minimum touch target: 44x44px */
button {
  min-width: 44px;
  min-height: 44px;
}

/* Minimum gap between targets */
.button-group {
  display: flex;
  gap: 0.5rem; /* 8px minimum */
}
```

## Checklist

- [ ] Uses tokens from spacing scale
- [ ] Follows 8pt or 4pt grid
- [ ] Touch targets minimum 44x44px
- [ ] Spacing scales on different viewports
- [ ] Uses logical properties for RTL
- [ ] Maintains vertical rhythm
- [ ] Sufficient whitespace for clarity
