# Performance Optimization

Optimizing React and Next.js applications for speed and efficiency.

## Performance Stack

| Tool | Purpose |
|------|---------|
| @next/bundle-analyzer | Visualize bundle size |
| React.lazy | Code splitting |
| next/image | Optimized images |
| web-vitals | Performance metrics |
| React DevTools | Identify bottlenecks |

## Bundle Analysis

```typescript
// next.config.js
const withBundleAnalyzer = require("@next/bundle-analyzer")({
  enabled: process.env.ANALYZE === "true",
});

module.exports = withBundleAnalyzer({});

// Run: ANALYZE=true next build
```

### Reducing Bundle Size

```typescript
// ❌ Bad: Import entire library
import _ from "lodash";
const result = _.debounce(fn, 300);

// ✅ Good: Import only what you need
import debounce from "lodash/debounce";
const result = debounce(fn, 300);

// ❌ Bad: Import all icons
import * as Icons from "lucide-react";

// ✅ Good: Import specific icons
import { Search, Menu } from "lucide-react";
```

## Code Splitting

```typescript
import { lazy, Suspense } from "react";

const HeavyComponent = lazy(() => import("@/components/heavy"));

function Page() {
  return (
    <Suspense fallback={<Skeleton />}>
      <HeavyComponent />
    </Suspense>
  );
}

// Next.js dynamic import (no SSR)
import dynamic from "next/dynamic";

const Chart = dynamic(() => import("@/components/chart"), {
  loading: () => <p>Loading...</p>,
  ssr: false,
});
```

## Image Optimization

```typescript
import Image from "next/image";

function ProductCard({ product }: { product: Product }) {
  return (
    <Image
      src={product.imageUrl}
      alt={product.name}
      width={400}
      height={300}
      placeholder="blur"
      blurDataURL={product.blurDataUrl}
      sizes="(max-width: 768px) 100vw, (max-width: 1200px) 50vw, 33vw"
      priority={false} // true for above-the-fold
    />
  );
}

// Responsive hero
<Image
  src="/hero.jpg"
  alt="Hero"
  fill
  className="object-cover"
  sizes="100vw"
  priority
/>
```

## React Memoization

```typescript
import { memo, useMemo, useCallback } from "react";

// memo - Prevent re-renders
const UserCard = memo(({ user, onDelete }: Props) => {
  return (
    <div>
      <h3>{user.name}</h3>
      <button onClick={() => onDelete(user.id)}>Delete</button>
    </div>
  );
});

// useMemo - Expensive computations
function ProductList({ products }: { products: Product[] }) {
  const sorted = useMemo(
    () => products.slice().sort((a, b) => a.price - b.price),
    [products]
  );
  return <ul>{sorted.map((p) => <li key={p.id}>{p.name}</li>)}</ul>;
}

// useCallback - Stable function references
function Parent() {
  const [count, setCount] = useState(0);

  const handleClick = useCallback(() => {
    console.log("Clicked!");
  }, []); // Empty deps = never changes

  return (
    <>
      <button onClick={() => setCount(c => c + 1)}>Count: {count}</button>
      <ExpensiveChild onClick={handleClick} />
    </>
  );
}
```

### When to Memoize

```typescript
// ❌ Don't memoize cheap computations
const doubled = useMemo(() => count * 2, [count]); // Overkill

// ✅ Memoize expensive computations
const filtered = useMemo(() => {
  return largeArray
    .filter((item) => item.active)
    .map((item) => processItem(item))
    .sort((a, b) => a.priority - b.priority);
}, [largeArray]);

// ✅ Memoize for referential equality
const options = useMemo(() => ({ sort: "asc" }), []); // Stable reference
```

## Web Vitals

```typescript
// app/web-vitals.tsx
"use client";

import { useReportWebVitals } from "next/web-vitals";

export function WebVitals() {
  useReportWebVitals((metric) => {
    console.log(metric);
    // Send to analytics
  });
  return null;
}

// Targets:
// LCP (Largest Contentful Paint) < 2.5s
// FID (First Input Delay) < 100ms
// CLS (Cumulative Layout Shift) < 0.1
```

### Optimization Tips

```typescript
// LCP - Preload critical assets
<link rel="preload" href="/hero.jpg" as="image" />
<Image src="/hero.jpg" priority />

// CLS - Reserve space for dynamic content
<div className="min-h-[200px]">
  {loading ? <Skeleton /> : <Content />}
</div>

// Always set image dimensions
<Image src="/image.jpg" width={400} height={300} />
```

## Virtualization

```typescript
import { FixedSizeList } from "react-window";

function UserList({ users }: { users: User[] }) {
  return (
    <FixedSizeList
      height={600}
      itemCount={users.length}
      itemSize={50}
      width="100%"
      itemData={users}
    >
      {({ index, style, data }) => (
        <div style={style}>{data[index].name}</div>
      )}
    </FixedSizeList>
  );
}
```

## Server Components

```typescript
// ✅ Server Component - No JS sent to client
async function ProductList() {
  const products = await fetchProducts();
  return <ul>{products.map((p) => <li key={p.id}>{p.name}</li>)}</ul>;
}

// ✅ Client only when needed
"use client";

function AddToCartButton({ productId }: { productId: string }) {
  return <button onClick={() => addToCart(productId)}>Add</button>;
}
```

## Critical Rules

### Always
- Measure before optimizing
- Use bundle analyzer
- Implement code splitting
- Optimize images with next/image
- Monitor Web Vitals

### Never
- Premature optimization
- Skip performance budgets
- Over-memoize simple components
- Ignore bundle size warnings
- Forget to measure impact
