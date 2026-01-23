# Modern JavaScript (ESM)

ESM patterns and deciding between JavaScript and TypeScript.

## JS vs TS Decision Guide

| Scenario | Recommendation |
|----------|----------------|
| New project, team knows TS | TypeScript |
| Quick script/automation | JavaScript |
| Library with public API | TypeScript |
| Legacy codebase | Gradual migration |
| Prototyping | JavaScript |
| Large team | TypeScript |

### Use JavaScript for
- Quick scripts and utilities
- Prototyping
- Simple Node.js scripts

### Use TypeScript for
- Large applications
- Libraries with public APIs
- Complex business logic

## ESM Fundamentals

```json
// package.json
{
  "type": "module",
  "exports": {
    ".": {
      "import": "./src/index.js",
      "types": "./types/index.d.ts"
    }
  },
  "engines": {
    "node": ">=20.0.0"
  }
}
```

### Import/Export

```javascript
// Named exports
export function add(a, b) {
  return a + b;
}

export const PI = 3.14159;

// Default export
export default class User {
  constructor(name) {
    this.name = name;
  }
}

// Re-exports
export { formatDate } from "./formatters.js";
export * from "./utils.js";

// Import patterns
import User from "./user.js";
import { add, PI } from "./math.js";
import * as math from "./math.js";

// Dynamic imports
const module = await import("./heavy-module.js");

// IMPORTANT: Include file extensions in ESM
import { helper } from "./utils.js"; // ✅
import { helper } from "./utils";    // ❌ Error
```

## JSDoc Type Hints

```javascript
/**
 * @typedef {Object} User
 * @property {string} id
 * @property {string} name
 * @property {"admin" | "user"} role
 */

/**
 * Get user by ID
 * @param {string} id
 * @returns {Promise<User>}
 */
export async function getUser(id) {
  return db.users.findById(id);
}

/**
 * Calculate total
 * @template T
 * @param {T[]} items
 * @param {(item: T) => number} getPrice
 * @returns {number}
 */
export function calculateTotal(items, getPrice) {
  return items.reduce((sum, item) => sum + getPrice(item), 0);
}

/**
 * @param {string | number} value
 * @returns {string}
 */
export function toString(value) {
  return String(value);
}
```

## Node.js ESM Specifics

```javascript
import { fileURLToPath } from "url";
import { dirname, join } from "path";

// __dirname equivalent
const __filename = fileURLToPath(import.meta.url);
const __dirname = dirname(__filename);

// Read file relative to module
import fs from "fs/promises";
const config = await fs.readFile(join(__dirname, "config.json"), "utf-8");

// Top-level await (ESM only)
const data = await fetch("https://api.example.com").then((r) => r.json());

// Import meta
console.log(import.meta.url); // file:///path/to/file.js
```

## React Without TypeScript

```javascript
import PropTypes from "prop-types";

/**
 * @typedef {Object} ButtonProps
 * @property {React.ReactNode} children
 * @property {() => void} onClick
 * @property {"primary" | "secondary"} [variant]
 */

/**
 * @param {ButtonProps} props
 */
export function Button({ children, onClick, variant = "primary" }) {
  return (
    <button onClick={onClick} className={`btn btn-${variant}`}>
      {children}
    </button>
  );
}

Button.propTypes = {
  children: PropTypes.node.isRequired,
  onClick: PropTypes.func.isRequired,
  variant: PropTypes.oneOf(["primary", "secondary"]),
};
```

## Migration to TypeScript

### Step 1: Add JSDoc types
```javascript
/**
 * @param {string} id
 * @returns {Promise<User>}
 */
export async function getUser(id) {
  return db.users.get(id);
}
```

### Step 2: Enable TS checking
```json
// tsconfig.json
{
  "compilerOptions": {
    "allowJs": true,
    "checkJs": true,
    "noEmit": true
  },
  "include": ["src/**/*.js"]
}
```

### Step 3: Rename incrementally
```bash
mv src/utils.js src/utils.ts
mv src/user-service.js src/user-service.ts
```

### Step 4: Enable strict mode gradually
```json
{
  "compilerOptions": {
    "strict": false,
    "noImplicitAny": true,
    "strictNullChecks": true
  }
}
```

## Tree-Shaking

```javascript
// ✅ Tree-shakeable (named exports)
export function add(a, b) { return a + b; }
export function subtract(a, b) { return a - b; }

// ❌ Less tree-shakeable
export default {
  add: (a, b) => a + b,
  subtract: (a, b) => a - b,
};
```

## Critical Rules

### Always
- Use ESM (import/export)
- Include .js extensions
- Set "type": "module"
- Document with JSDoc
- Consider TS for large projects

### Never
- Mix CommonJS and ESM
- Forget file extensions
- Use require() in ESM
- Skip documentation
- Over-complicate simple scripts
