# Motion

## Principles

1. **Purposeful**: Every animation serves feedback, guidance, or continuity
2. **Natural**: Use realistic easing, not linear motion
3. **Efficient**: Fast and non-intrusive

## Timing

```typescript
export const motion = {
  duration: {
    fast: "150ms",    // Hover, focus
    base: "300ms",    // Standard transitions
    slow: "500ms",    // Complex animations
  },
};
```

## Easing Functions

```typescript
export const easing = {
  easeIn: "cubic-bezier(0.4, 0, 1, 1)",      // Slow start
  easeOut: "cubic-bezier(0, 0, 0.2, 1)",     // Fast start (entrances)
  easeInOut: "cubic-bezier(0.4, 0, 0.2, 1)", // Symmetrical
  bounce: "cubic-bezier(0.68, -0.55, 0.265, 1.55)", // Use sparingly
};
```

### Usage Patterns

- **Entrances**: ease-out (fast start, slow end)
- **Exits**: ease-in (slow start, fast end)
- **Emphasis**: ease-in-out (symmetrical)

## Performance

```css
/* GPU-accelerated (fast) */
.performant {
  transition: transform 300ms ease-out,
              opacity 300ms ease-out;
}

/* Avoid (triggers layout) */
.slow {
  transition: width 300ms ease-out;  /* Bad */
}
```

## Common Patterns

### Fade In

```css
@keyframes fadeIn {
  from { opacity: 0; }
  to { opacity: 1; }
}

.fade-in {
  animation: fadeIn 300ms ease-out;
}
```

### Slide In

```css
@keyframes slideInUp {
  from {
    opacity: 0;
    transform: translateY(20px);
  }
  to {
    opacity: 1;
    transform: translateY(0);
  }
}
```

### Button Interaction

```css
.button {
  transition: transform 150ms ease-out,
              box-shadow 150ms ease-out;
}

.button:hover {
  transform: translateY(-1px);
  box-shadow: 0 4px 6px rgba(0, 0, 0, 0.1);
}

.button:active {
  transform: translateY(0);
}
```

## Loading States

### Skeleton

```css
@keyframes pulse {
  0%, 100% { opacity: 1; }
  50% { opacity: 0.5; }
}

.skeleton {
  background-color: #e5e7eb;
  animation: pulse 2s ease-in-out infinite;
}
```

### Spinner

```css
.spinner {
  animation: spin 1s linear infinite;
}

@keyframes spin {
  to { transform: rotate(360deg); }
}
```

## Reduced Motion

**Critical**: Always respect `prefers-reduced-motion`.

```css
@media (prefers-reduced-motion: reduce) {
  .animated {
    transition: none;
    animation: none;
  }

  /* Keep essential feedback */
  .animated.essential {
    transition: opacity 150ms ease-out;
  }
}
```

### JavaScript Detection

```typescript
function useReducedMotion(): boolean {
  const [prefersReduced, setPrefersReduced] = useState(false);

  useEffect(() => {
    const mediaQuery = window.matchMedia("(prefers-reduced-motion: reduce)");
    setPrefersReduced(mediaQuery.matches);

    const handler = () => setPrefersReduced(mediaQuery.matches);
    mediaQuery.addEventListener("change", handler);
    return () => mediaQuery.removeEventListener("change", handler);
  }, []);

  return prefersReduced;
}
```

## Checklist

- [ ] Animation serves a clear purpose
- [ ] Duration appropriate for element size
- [ ] Easing feels natural
- [ ] Only animates transform/opacity
- [ ] Respects `prefers-reduced-motion`
- [ ] No flashing (> 3 times per second)
- [ ] Performance acceptable on low-end devices
