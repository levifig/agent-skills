# Color

## Color Palette Structure

### Full Spectrum (50-900)

```typescript
export const colors = {
  gray: {
    50: "#f9fafb", 100: "#f3f4f6", 200: "#e5e7eb",
    300: "#d1d5db", 400: "#9ca3af", 500: "#6b7280",
    600: "#4b5563", 700: "#374151", 800: "#1f2937",
    900: "#111827",
  },
  blue: {
    50: "#eff6ff", 100: "#dbeafe", 200: "#bfdbfe",
    300: "#93c5fd", 400: "#60a5fa", 500: "#3b82f6",
    600: "#2563eb", 700: "#1d4ed8", 800: "#1e40af",
    900: "#1e3a8a",
  },
};
```

### Semantic Colors

```typescript
export const semanticColors = {
  brand: { primary: colors.blue[600], hover: colors.blue[700] },
  text: {
    primary: colors.gray[900],    // Body text
    secondary: colors.gray[600],  // Supporting text
    inverse: colors.gray[50],     // On dark backgrounds
  },
  status: {
    success: colors.green[600],
    warning: colors.yellow[600],
    danger: colors.red[600],
    info: colors.blue[600],
  },
};
```

## Contrast Requirements

| Level | Normal Text | Large Text | UI Components |
|-------|-------------|------------|---------------|
| AA    | 4.5:1       | 3:1        | 3:1           |
| AAA   | 7:1         | 4.5:1      | N/A           |

- **Normal text**: < 18pt or < 14pt bold
- **Large text**: >= 18pt or >= 14pt bold

## Dark Mode

```typescript
export const darkMode = {
  background: {
    primary: "#0a0a0a",    // True dark
    secondary: "#1a1a1a",  // Elevated surfaces
    tertiary: "#2a2a2a",   // More elevated
  },
  text: {
    primary: "#f5f5f5",    // 15:1 on dark bg
    secondary: "#a3a3a3",  // 7:1 on dark bg
  },
};
```

### Implementation

```css
:root {
  --color-bg-primary: #ffffff;
  --color-text-primary: #1a1a1a;
}

[data-theme="dark"] {
  --color-bg-primary: #0a0a0a;
  --color-text-primary: #f5f5f5;
}
```

## Color Blindness

Design for ~8% of men and ~0.5% of women with color vision deficiency.

### Safe Combinations

```typescript
export const colorBlindSafe = {
  success: { color: "#0d9488", icon: "check" },  // Teal
  warning: { color: "#f59e0b", icon: "warning" }, // Amber
  danger: { color: "#dc2626", icon: "x" },        // Red
};
```

### Never Use Color Alone

```typescript
// Bad
<span className="text-red-600">Error</span>

// Good
<span className="text-red-600">
  <XCircleIcon aria-hidden="true" />
  <span>Error: Invalid email</span>
</span>
```

## Color Utilities

```typescript
function getContrast(color1: string, color2: string): number {
  const lum1 = getLuminance(...hexToRgb(color1));
  const lum2 = getLuminance(...hexToRgb(color2));
  const lighter = Math.max(lum1, lum2);
  const darker = Math.min(lum1, lum2);
  return (lighter + 0.05) / (darker + 0.05);
}
```

## Checklist

- [ ] Text meets 4.5:1 contrast (AA)
- [ ] Large text meets 3:1 contrast
- [ ] Interactive elements meet 3:1 contrast
- [ ] Focus indicators meet 3:1 contrast
- [ ] Color is not sole indicator of meaning
- [ ] Works in light and dark modes
- [ ] Distinguishable by colorblind users
- [ ] All colors from design tokens
