---
name: typescript
description: >-
  TypeScript and JavaScript development with React ecosystem. Covers project
  setup, React 18+ components, Next.js 14+ App Router, modern ESM patterns,
  Zustand/React Query state management, React Hook Form with Zod validation,
  Tailwind CSS styling, Vitest/Playwright testing, and accessibility. Activate
  when working with .ts/.tsx/.js/.jsx files, package.json, or React/Next.js
  projects.
agent: backend-dev
allowed-tools: 'Read, Write, Edit, Bash, Glob, Grep'
---

# TypeScript Development

Comprehensive guide for modern TypeScript development with React ecosystem.

## When to Use This Skill

- Starting or configuring TypeScript projects
- Building React components and applications
- Working with Next.js 14+ and App Router
- Implementing state management patterns
- Building forms with validation
- Integrating APIs and handling data fetching
- Writing tests for TypeScript applications
- Styling with Tailwind and CVA
- Optimizing application performance
- Implementing accessibility (a11y)
- Building React Native mobile apps
- Working with modern JavaScript (ESM)

## Stack Overview

| Layer | Default | Alternatives |
|-------|---------|--------------|
| Language | TypeScript 5+ | JavaScript (ESM) |
| Runtime | Node.js 20+ | Bun, Deno |
| Framework | Next.js 14+ | Vite, Remix |
| UI Library | React 18+ | - |
| State (Client) | Zustand | Context + Reducer |
| State (Server) | React Query | SWR |
| Forms | React Hook Form + Zod | - |
| Styling | Tailwind CSS + CVA | CSS Modules |
| Testing | Vitest + RTL | Jest |
| E2E Testing | Playwright | Cypress |
| Package Manager | pnpm | npm, yarn |

## Core Philosophy

- **Strict mode always** - catch errors at compile time
- **Server Components by default** - use Client Components only when needed
- **Type inference** - let TypeScript infer when obvious
- **Server state is different** - use React Query for API data
- **Accessibility is mandatory** - not optional
- **Mobile-first responsive** - design for small screens first
- **Measure before optimizing** - profile, then fix

## Quick Reference

### Project Setup

```json
// tsconfig.json essentials
{
  "compilerOptions": {
    "target": "ES2022",
    "strict": true,
    "noUncheckedIndexedAccess": true,
    "moduleResolution": "bundler"
  }
}
```

### Component Pattern

```typescript
interface ButtonProps {
  children: React.ReactNode;
  onClick: () => void;
  variant?: "primary" | "secondary";
}

export function Button({ children, onClick, variant = "primary" }: ButtonProps) {
  return (
    <button onClick={onClick} className={`btn btn-${variant}`}>
      {children}
    </button>
  );
}
```

### Server Component (Next.js)

```typescript
// Server Component by default - no "use client"
export default async function PostsPage() {
  const posts = await fetch("https://api.example.com/posts").then(r => r.json());
  return <PostList posts={posts} />;
}
```

### Client Component

```typescript
"use client";

import { useState } from "react";

export function Counter() {
  const [count, setCount] = useState(0);
  return <button onClick={() => setCount(c => c + 1)}>Count: {count}</button>;
}
```

### Form with Zod Validation

```typescript
import { useForm } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { z } from "zod";

const schema = z.object({
  email: z.string().email(),
  password: z.string().min(8),
});

type FormData = z.infer<typeof schema>;

function LoginForm() {
  const { register, handleSubmit, formState: { errors } } = useForm<FormData>({
    resolver: zodResolver(schema),
  });
  // ...
}
```

### React Query for API Data

```typescript
import { useQuery } from "@tanstack/react-query";

export function useUsers() {
  return useQuery({
    queryKey: ["users"],
    queryFn: () => fetch("/api/users").then(r => r.json()),
    staleTime: 5 * 60 * 1000,
  });
}
```

### CVA for Type-Safe Variants

```typescript
import { cva, type VariantProps } from "class-variance-authority";

const button = cva("rounded-md font-medium", {
  variants: {
    variant: { primary: "bg-blue-600 text-white", secondary: "bg-gray-200" },
    size: { sm: "px-3 py-2 text-sm", md: "px-4 py-2", lg: "px-6 py-3" },
  },
  defaultVariants: { variant: "primary", size: "md" },
});

type ButtonProps = VariantProps<typeof button>;
```

## Topics

| Topic | Use For |
|-------|---------|
| [Core](reference/core.md) | Project setup, tsconfig, modern TS features, type utilities |
| [React](reference/react.md) | Components, hooks, Context API, performance patterns |
| [Next.js](reference/nextjs.md) | App Router, Server/Client Components, Server Actions, routing |
| [Types](reference/types.md) | Advanced types, generics, conditional types, type guards |
| [State](reference/state.md) | Zustand, React Query, Context + Reducer, URL state |
| [Forms](reference/forms.md) | React Hook Form, Zod validation, Server Actions integration |
| [API](reference/api.md) | Fetch wrappers, React Query, tRPC, GraphQL, WebSockets |
| [Testing](reference/testing.md) | Vitest, React Testing Library, MSW, Playwright E2E |
| [Styling](reference/styling.md) | Tailwind CSS, CVA variants, dark mode, responsive design |
| [Performance](reference/performance.md) | Bundle analysis, code splitting, memoization, Web Vitals |
| [Accessibility](reference/a11y.md) | WCAG compliance, ARIA, keyboard navigation, screen readers |
| [Mobile](reference/mobile.md) | React Native, Expo, navigation, platform-specific code |
| [ESM](reference/esm.md) | ESM patterns, JSDoc types, JS vs TS decision guide |

## Critical Rules

### Always

- Use strict mode in tsconfig
- Type all function parameters and returns
- Handle null/undefined explicitly
- Use Server Components by default
- Validate on both client and server
- Test with screen readers
- Measure before optimizing

### Never

- Use `any` (use `unknown` with type guards)
- Use `!` (non-null assertion) without justification
- Store server data in client state (use React Query)
- Rely on color alone for information
- Create new functions in render
- Skip error handling for API calls
- Premature optimization
