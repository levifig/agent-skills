# Styling with Tailwind CSS

Modern styling with utility classes and type-safe variants.

## Tailwind Configuration

```typescript
// tailwind.config.ts
import type { Config } from "tailwindcss";

const config: Config = {
  content: [
    "./app/**/*.{js,ts,jsx,tsx}",
    "./components/**/*.{js,ts,jsx,tsx}",
  ],
  darkMode: "class",
  theme: {
    extend: {
      colors: {
        brand: {
          50: "#f0f9ff",
          500: "#0ea5e9",
          900: "#0c4a6e",
        },
      },
      fontFamily: {
        sans: ["Inter", "system-ui", "sans-serif"],
      },
    },
  },
  plugins: [
    require("@tailwindcss/forms"),
    require("@tailwindcss/typography"),
  ],
};

export default config;
```

## cn Utility

```typescript
// lib/utils.ts
import { clsx, type ClassValue } from "clsx";
import { twMerge } from "tailwind-merge";

export function cn(...inputs: ClassValue[]) {
  return twMerge(clsx(inputs));
}

// Usage
<div className={cn(
  "rounded-lg p-4",
  variant === "primary" && "bg-blue-500 text-white",
  variant === "secondary" && "bg-gray-200",
  className
)} />
```

## Class Variance Authority (CVA)

```typescript
import { cva, type VariantProps } from "class-variance-authority";

const buttonVariants = cva(
  "inline-flex items-center justify-center rounded-md font-medium transition-colors focus:ring-2 focus:ring-offset-2 disabled:opacity-50",
  {
    variants: {
      variant: {
        primary: "bg-blue-600 text-white hover:bg-blue-700 focus:ring-blue-500",
        secondary: "bg-gray-200 text-gray-900 hover:bg-gray-300 focus:ring-gray-500",
        destructive: "bg-red-600 text-white hover:bg-red-700 focus:ring-red-500",
        outline: "border border-gray-300 bg-transparent hover:bg-gray-100",
        ghost: "hover:bg-gray-100",
      },
      size: {
        sm: "h-9 px-3 text-sm",
        md: "h-10 px-4",
        lg: "h-11 px-8",
        icon: "h-10 w-10",
      },
    },
    defaultVariants: {
      variant: "primary",
      size: "md",
    },
  }
);

interface ButtonProps
  extends React.ButtonHTMLAttributes<HTMLButtonElement>,
    VariantProps<typeof buttonVariants> {}

export function Button({ className, variant, size, ...props }: ButtonProps) {
  return (
    <button className={buttonVariants({ variant, size, className })} {...props} />
  );
}

// Usage - fully type-safe!
<Button variant="primary" size="lg">Click me</Button>
<Button variant="outline" size="sm">Cancel</Button>
```

### Compound Variants

```typescript
const badge = cva("inline-flex items-center rounded-full font-semibold", {
  variants: {
    variant: {
      default: "bg-gray-100 text-gray-900",
      success: "bg-green-100 text-green-900",
      error: "bg-red-100 text-red-900",
    },
    size: {
      sm: "px-2 py-0.5 text-xs",
      md: "px-2.5 py-1 text-sm",
    },
    outline: {
      true: "bg-transparent border",
    },
  },
  compoundVariants: [
    { variant: "default", outline: true, className: "border-gray-300" },
    { variant: "success", outline: true, className: "border-green-300" },
    { variant: "error", outline: true, className: "border-red-300" },
  ],
  defaultVariants: { variant: "default", size: "md" },
});
```

## Dark Mode

```typescript
// ThemeProvider
"use client";

import { createContext, useContext, useEffect, useState } from "react";

type Theme = "light" | "dark" | "system";

const ThemeContext = createContext<{
  theme: Theme;
  setTheme: (theme: Theme) => void;
}>({ theme: "system", setTheme: () => {} });

export function ThemeProvider({ children }: { children: React.ReactNode }) {
  const [theme, setTheme] = useState<Theme>("system");

  useEffect(() => {
    const root = document.documentElement;
    const resolved =
      theme === "system"
        ? window.matchMedia("(prefers-color-scheme: dark)").matches
          ? "dark"
          : "light"
        : theme;

    root.classList.remove("light", "dark");
    root.classList.add(resolved);
    localStorage.setItem("theme", theme);
  }, [theme]);

  return (
    <ThemeContext.Provider value={{ theme, setTheme }}>
      {children}
    </ThemeContext.Provider>
  );
}

export const useTheme = () => useContext(ThemeContext);

// Dark mode styles
<div className="bg-white dark:bg-gray-900 text-gray-900 dark:text-gray-100" />
```

## Responsive Design

```typescript
// Mobile-first breakpoints
// sm: 640px, md: 768px, lg: 1024px, xl: 1280px, 2xl: 1536px

<div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4">
  {items.map((item) => <Card key={item.id} />)}
</div>

// Responsive navigation
<nav className="flex items-center justify-between">
  <Logo />
  {/* Desktop nav */}
  <div className="hidden md:flex md:space-x-6">
    <NavLink href="/">Home</NavLink>
    <NavLink href="/about">About</NavLink>
  </div>
  {/* Mobile menu button */}
  <button className="md:hidden">
    <MenuIcon />
  </button>
</nav>
```

## Animations

```typescript
// Transitions
<button className="transition-all duration-200 hover:scale-105 active:scale-95">
  Click
</button>

// Loading spinner
<svg className="h-5 w-5 animate-spin" viewBox="0 0 24 24">
  <circle className="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" strokeWidth="4" />
  <path className="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0..." />
</svg>

// Custom animation
// tailwind.config.ts
{
  theme: {
    extend: {
      animation: {
        "fade-in": "fadeIn 0.3s ease-out",
      },
      keyframes: {
        fadeIn: {
          "0%": { opacity: "0" },
          "100%": { opacity: "1" },
        },
      },
    },
  },
}
```

## Critical Rules

### Always
- Use utility classes
- Create variants with CVA
- Support dark mode
- Design mobile-first
- Extract repeated patterns

### Never
- Use inline styles
- Hardcode colors/spacing
- Skip responsive breakpoints
- Forget dark mode variants
- Ignore focus states
