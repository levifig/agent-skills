# Advanced TypeScript Types

Mastering TypeScript's type system for maximum type safety.

## Utility Types

```typescript
// Partial - Make all properties optional
type User = { id: string; name: string; email: string };
type PartialUser = Partial<User>;

// Required - Make all properties required
type RequiredUser = Required<User>;

// Readonly - Make all properties readonly
type ReadonlyUser = Readonly<User>;

// Pick - Select specific properties
type UserPreview = Pick<User, "id" | "name">;

// Omit - Remove specific properties
type PublicUser = Omit<User, "email">;

// Record - Create object type with key/value
type UserRoles = Record<string, "admin" | "user">;

// Parameters - Extract function parameters
type FnParams = Parameters<typeof someFunction>;

// ReturnType - Extract return type
type FnReturn = ReturnType<typeof someFunction>;

// Awaited - Unwrap Promise type
type Data = Awaited<Promise<User>>; // User
```

## Conditional Types

```typescript
// Basic conditional
type IsString<T> = T extends string ? true : false;

// Extract string types
type StringsOnly<T> = T extends string ? T : never;
type Result = StringsOnly<"a" | "b" | 1 | 2>; // "a" | "b"

// Infer keyword
type GetReturnType<T> = T extends (...args: any[]) => infer R ? R : never;

type UnwrapPromise<T> = T extends Promise<infer U> ? U : T;
type Awaited<T> = T extends Promise<infer U> ? Awaited<U> : T;

// Array element type
type ArrayElement<T> = T extends (infer U)[] ? U : never;
type Item = ArrayElement<string[]>; // string
```

## Template Literal Types

```typescript
// Basic template literal
type Greeting = `Hello, ${string}`;

// From union types
type HTTPMethod = "GET" | "POST" | "PUT" | "DELETE";
type Endpoint = "/users" | "/posts";
type Route = `${HTTPMethod} ${Endpoint}`;
// "GET /users" | "GET /posts" | "POST /users" | ...

// Type-safe event names
type EventName<T extends string> = `on${Capitalize<T>}`;
type ClickEvent = EventName<"click">; // "onClick"

// Getters and Setters
type Getters<T> = {
  [K in keyof T as `get${Capitalize<string & K>}`]: () => T[K];
};

type UserGetters = Getters<User>;
// { getName: () => string; getEmail: () => string; }
```

## Mapped Types

```typescript
// Basic mapped type
type Optional<T> = {
  [K in keyof T]?: T[K];
};

// With key remapping
type Prefixed<T, P extends string> = {
  [K in keyof T as `${P}${Capitalize<string & K>}`]: T[K];
};

// Filtering keys
type OnlyStrings<T> = {
  [K in keyof T as T[K] extends string ? K : never]: T[K];
};

// Making specific keys required
type RequireKeys<T, K extends keyof T> = Omit<T, K> &
  Required<Pick<T, K>>;
```

## Discriminated Unions

```typescript
// State machine pattern
type RequestState<T> =
  | { status: "idle" }
  | { status: "loading" }
  | { status: "success"; data: T }
  | { status: "error"; error: Error };

function handleRequest<T>(state: RequestState<T>) {
  switch (state.status) {
    case "idle":
      return "Ready to fetch";
    case "loading":
      return "Loading...";
    case "success":
      return `Data: ${state.data}`; // Type narrowed
    case "error":
      return `Error: ${state.error.message}`; // Type narrowed
  }
}

// API response pattern
type ApiResponse<T> =
  | { success: true; data: T }
  | { success: false; error: string };
```

## Branded Types

```typescript
// Create unique types for primitives
type UserId = string & { __brand: "UserId" };
type PostId = string & { __brand: "PostId" };

function createUserId(id: string): UserId {
  return id as UserId;
}

function getUser(id: UserId) {
  // Only accepts UserId, not plain string or PostId
}

const userId = createUserId("123");
const postId = "456" as PostId;

getUser(userId); // OK
// getUser(postId); // Error: PostId not assignable to UserId
// getUser("789"); // Error: string not assignable to UserId
```

## Type Guards

```typescript
// Type predicate
function isUser(value: unknown): value is User {
  return (
    typeof value === "object" &&
    value !== null &&
    "id" in value &&
    "name" in value &&
    "email" in value
  );
}

// Assertion function
function assertIsUser(value: unknown): asserts value is User {
  if (!isUser(value)) {
    throw new Error("Value is not a User");
  }
}

// Narrowing with in operator
function handleResponse(response: SuccessResponse | ErrorResponse) {
  if ("data" in response) {
    // response is SuccessResponse
    return response.data;
  } else {
    // response is ErrorResponse
    throw new Error(response.error);
  }
}

// Discriminated union narrowing
function processResult(result: Result) {
  if (result.success) {
    return result.data; // TypeScript knows this is success case
  }
  return result.error; // TypeScript knows this is error case
}
```

## Generic Constraints

```typescript
// Basic constraint
function getProperty<T, K extends keyof T>(obj: T, key: K): T[K] {
  return obj[key];
}

// Multiple constraints
interface Comparable<T> {
  compareTo(other: T): number;
}

function sort<T extends Comparable<T>>(items: T[]): T[] {
  return items.sort((a, b) => a.compareTo(b));
}

// Conditional with constraints
type KeysOfType<T, V> = {
  [K in keyof T]: T[K] extends V ? K : never;
}[keyof T];

type StringKeys = KeysOfType<User, string>; // "id" | "name" | "email"
```

## Recursive Types

```typescript
// JSON type
type JSONValue =
  | string
  | number
  | boolean
  | null
  | JSONValue[]
  | { [key: string]: JSONValue };

// Deep partial
type DeepPartial<T> = T extends object
  ? { [K in keyof T]?: DeepPartial<T[K]> }
  : T;

// Tree structure
type TreeNode<T> = {
  value: T;
  children: TreeNode<T>[];
};

// Flatten array type
type Flatten<T> = T extends (infer U)[] ? Flatten<U> : T;
```

## Critical Rules

### Always
- Use discriminated unions for state
- Prefer type inference when obvious
- Create branded types for IDs
- Use const assertions for literals
- Document complex types

### Never
- Use any (use unknown + type guards)
- Create overly complex types
- Ignore TypeScript errors
- Skip type annotations on APIs
- Use type assertions without guards
