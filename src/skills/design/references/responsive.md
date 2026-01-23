# Responsive Design

## Mobile-First Approach

Design for mobile first, then enhance for larger screens.

```css
/* Mobile default */
.container { padding: 1rem; }

/* Tablet and up */
@media (min-width: 768px) {
  .container { padding: 2rem; }
}

/* Desktop and up */
@media (min-width: 1024px) {
  .container { padding: 3rem; }
}
```

## Breakpoints

```typescript
export const breakpoints = {
  sm: "640px",   // Mobile landscape
  md: "768px",   // Tablets
  lg: "1024px",  // Laptops
  xl: "1280px",  // Desktops
  "2xl": "1536px", // Large desktops
};
```

## Layout Patterns

### Fluid Grid

```css
.grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(250px, 1fr));
  gap: 1rem;
}
```

### Sidebar Layout

```css
.layout {
  display: grid;
  gap: 2rem;
}

@media (min-width: 768px) {
  .layout {
    grid-template-columns: 250px 1fr;
  }
}
```

## Container Queries

```css
.card-container {
  container-type: inline-size;
  container-name: card;
}

@container card (min-width: 400px) {
  .card { flex-direction: row; }
}
```

## Responsive Images

```html
<img
  src="image.jpg"
  srcset="image-400w.jpg 400w, image-800w.jpg 800w, image-1200w.jpg 1200w"
  sizes="(max-width: 768px) 100vw, 50vw"
  loading="lazy"
  alt="Description"
/>
```

### Picture Element for Art Direction

```html
<picture>
  <source media="(min-width: 1024px)" srcset="/hero-desktop.jpg" />
  <source media="(min-width: 768px)" srcset="/hero-tablet.jpg" />
  <img src="/hero-mobile.jpg" alt="Hero image" />
</picture>
```

## Touch Targets

```css
/* Minimum 44x44px on mobile */
button, a {
  min-width: 44px;
  min-height: 44px;
}

/* Expand hit area without changing visual size */
.icon-button {
  position: relative;
}

.icon-button::before {
  content: "";
  position: absolute;
  inset: -12px;
}
```

## Responsive Typography

```css
h1 { font-size: clamp(2rem, 5vw + 1rem, 4rem); }
h2 { font-size: clamp(1.5rem, 3vw + 1rem, 3rem); }
p { font-size: clamp(1rem, 1.5vw + 0.5rem, 1.25rem); }

.content { max-width: 65ch; }
```

## Responsive Navigation

```typescript
function Navigation() {
  return (
    <header>
      <MobileNav className="md:hidden" />
      <DesktopNav className="hidden md:flex" />
    </header>
  );
}
```

## Test Viewports

```typescript
const testViewports = [
  { name: "iPhone SE", width: 375, height: 667 },
  { name: "iPhone 14 Pro", width: 393, height: 852 },
  { name: "iPad", width: 768, height: 1024 },
  { name: "Desktop HD", width: 1920, height: 1080 },
];
```

## Checklist

- [ ] Content readable without horizontal scroll
- [ ] Touch targets minimum 44x44px on mobile
- [ ] Navigation accessible on all devices
- [ ] Images load appropriate sizes
- [ ] Typography scales appropriately
- [ ] Forms usable on mobile
- [ ] Test on real devices
