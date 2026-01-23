# Typography

## Type Scale

```typescript
// Base 16px, ratio 1.25 (Major Third)
export const typography = {
  fontSize: {
    xs: "0.75rem",     // 12px
    sm: "0.875rem",    // 14px
    base: "1rem",      // 16px (body default)
    lg: "1.125rem",    // 18px
    xl: "1.25rem",     // 20px
    "2xl": "1.5rem",   // 24px
    "3xl": "1.875rem", // 30px
    "4xl": "2.25rem",  // 36px
  },
  lineHeight: {
    tight: "1.25",
    normal: "1.5",
    relaxed: "1.625",
  },
  fontWeight: {
    normal: "400",
    medium: "500",
    semibold: "600",
    bold: "700",
  },
};
```

## Font Selection

### System Font Stack (Performance)

```css
font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto,
             "Helvetica Neue", Arial, sans-serif;
```

### Web Fonts with Fallbacks

```typescript
export const fontFamily = {
  sans: ["Inter var", "system-ui", "sans-serif"],
  serif: ["Merriweather", "Georgia", "serif"],
  mono: ["JetBrains Mono", "Consolas", "monospace"],
};
```

### Font Loading

```css
@font-face {
  font-family: "Inter var";
  font-weight: 100 900;
  font-display: swap;  /* Show fallback immediately */
  src: url("/fonts/Inter-var.woff2") format("woff2");
}
```

## Heading Hierarchy

```css
h1 { font-size: 3rem; font-weight: 700; line-height: 1.2; }
h2 { font-size: 2.25rem; font-weight: 700; line-height: 1.25; }
h3 { font-size: 1.875rem; font-weight: 600; line-height: 1.3; }
h4 { font-size: 1.5rem; font-weight: 600; line-height: 1.4; }
h5 { font-size: 1.25rem; font-weight: 500; line-height: 1.5; }
h6 { font-size: 1rem; font-weight: 500; line-height: 1.5; }
```

## Optimal Line Length

```css
/* 45-75 characters optimal, 65 ideal */
.content {
  max-width: 65ch;
  margin-inline: auto;
}
```

## Fluid Typography

```css
h1 {
  font-size: clamp(2rem, 5vw + 1rem, 4rem);
  /* Min: 32px, Preferred: 5vw + 16px, Max: 64px */
}

p {
  font-size: clamp(1rem, 1.5vw + 0.5rem, 1.25rem);
}
```

## Vertical Rhythm

All spacing should be multiples of base line height (24px).

```css
:root {
  --line-height-base: 1.5rem; /* 24px */
}

h1 {
  line-height: calc(var(--line-height-base) * 2);
  margin-bottom: var(--line-height-base);
}

p {
  line-height: var(--line-height-base);
  margin-bottom: var(--line-height-base);
}
```

## Accessibility

- Minimum font size: 16px for body text (14px for captions)
- Body text must meet 4.5:1 contrast
- Use relative units (rem) for resizable text
- Support 200% zoom without layout breaking

## OpenType Features

```css
/* Tabular figures for tables */
.table-numbers { font-variant-numeric: tabular-nums; }

/* Oldstyle figures for body text */
.body-numbers { font-variant-numeric: oldstyle-nums; }

/* Ligatures */
.with-ligatures { font-variant-ligatures: common-ligatures; }
```
