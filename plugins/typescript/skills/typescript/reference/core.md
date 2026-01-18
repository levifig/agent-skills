# TypeScript Core

Foundation for modern TypeScript development with strict type safety.

## TypeScript Stack

| Component | Default | Alternative |
|-----------|---------|-------------|
| Runtime | Node.js 20+ | Bun, Deno |
| Package Manager | pnpm | npm, yarn |
| Bundler | Vite | webpack, esbuild |
| Linter | ESLint | Biome |
| Formatter | Prettier | Biome |
| Test Runner | Vitest | Jest |

## Project Structure

```
my-app/
├── src/
│   ├── components/
│   ├── lib/
│   │   ├── types/
│   │   ├── utils/
│   │   └── hooks/
│   ├── app/              # Next.js App Router
│   └── index.ts
├── tests/
├── public/
├── tsconfig.json
├── package.json
└── vitest.config.ts
```

## TSConfig Best Practices

```json
{
  "compilerOptions": {
    "target": "ES2022",
    "lib": ["ES2022", "DOM", "DOM.Iterable"],
    "module": "ESNext",
    "moduleResolution": "bundler",
    "resolveJsonModule": true,

    "strict": true,
    "noUncheckedIndexedAccess": true,
    "noUnusedLocals": true,
    "noUnusedParameters": true,
    "noFallthroughCasesInSwitch": true,

    "jsx": "preserve",
    "esModuleInterop": true,
    "skipLibCheck": true,
    "allowSyntheticDefaultImports": true,

    "baseUrl": ".",
    "paths": {
      "@/*": ["./src/*"],
      "@/components/*": ["./src/components/*"],
      "@/lib/*": ["./src/lib/*"]
    }
  },
  "include": ["src"],
  "exclude": ["node_modules", "dist"]
}
```

## Modern TypeScript Features

```typescript
// Type-safe object keys
const user = { name: "John", age: 30 } as const;
type UserKeys = keyof typeof user; // "name" | "age"

// Satisfies operator (TypeScript 4.9+)
const config = {
  host: "localhost",
  port: 3000,
  ssl: false
} satisfies Config; // Type-check without widening

// Template literal types
type HTTPMethod = "GET" | "POST";
type Endpoint = `/api/${string}`;
type Route = `${HTTPMethod} ${Endpoint}`;

// Const type parameters (TypeScript 5.0+)
function makeArray<const T>(arr: T[]) {
  return arr;
}
const arr = makeArray([1, 2, 3]); // Type: readonly [1, 2, 3]

// Using declarations (TypeScript 5.2+)
{
  using file = openFile("data.txt");
  // Automatically disposed at end of block
}
```

## Type Utilities

```typescript
// Pick and Omit
type User = { id: number; name: string; email: string; };
type UserPreview = Pick<User, "id" | "name">;
type PublicUser = Omit<User, "email">;

// Partial and Required
type PartialUser = Partial<User>; // All optional
type RequiredUser = Required<User>; // All required

// Record for mapped types
type UserRoles = Record<string, "admin" | "user" | "guest">;

// Awaited for async types
type ApiResponse = Awaited<ReturnType<typeof fetchUser>>;

// Branded types for type safety
type UserId = number & { __brand: "UserId" };
type PostId = number & { __brand: "PostId" };
```

## Strict Null Checking

```typescript
// Handle null/undefined properly
function getUser(id: string): User | null {
  const user = database.find(id);
  return user ?? null; // Nullish coalescing
}

const user = getUser("123");
if (user) {
  // Type narrowed to User
  console.log(user.name);
}

// Optional chaining
const email = user?.profile?.email;

// Non-null assertion (use sparingly)
const element = document.getElementById("app")!;
```

## Type Guards

```typescript
// Type predicates
function isUser(obj: unknown): obj is User {
  return (
    typeof obj === "object" &&
    obj !== null &&
    "id" in obj &&
    "name" in obj
  );
}

// Discriminated unions
type Success = { status: "success"; data: string };
type Error = { status: "error"; message: string };
type Result = Success | Error;

function handleResult(result: Result) {
  if (result.status === "success") {
    console.log(result.data); // Type narrowed to Success
  } else {
    console.error(result.message); // Type narrowed to Error
  }
}
```

## Critical Rules

### Always
- Use strict mode in tsconfig
- Enable noUncheckedIndexedAccess
- Type all function parameters and returns
- Use const assertions for literal types
- Handle null/undefined explicitly

### Never
- Use any (use unknown instead)
- Use ! (non-null assertion) without justification
- Disable strict checks
- Ignore TypeScript errors with @ts-ignore
- Skip type annotations on public APIs
