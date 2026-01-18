# API Integration

Type-safe API clients and data fetching patterns.

## Type-Safe Fetch Wrapper

```typescript
type HttpMethod = "GET" | "POST" | "PUT" | "DELETE" | "PATCH";

class ApiError extends Error {
  constructor(
    public status: number,
    public statusText: string,
    public data?: unknown
  ) {
    super(`API Error: ${status} ${statusText}`);
    this.name = "ApiError";
  }
}

class ApiClient {
  constructor(private baseUrl: string) {}

  private async request<T>(
    method: HttpMethod,
    path: string,
    options?: { body?: unknown; params?: Record<string, string> }
  ): Promise<T> {
    const url = new URL(path, this.baseUrl);
    if (options?.params) {
      Object.entries(options.params).forEach(([k, v]) =>
        url.searchParams.set(k, v)
      );
    }

    const response = await fetch(url.toString(), {
      method,
      headers: { "Content-Type": "application/json" },
      body: options?.body ? JSON.stringify(options.body) : undefined,
    });

    if (!response.ok) {
      const data = await response.json().catch(() => null);
      throw new ApiError(response.status, response.statusText, data);
    }

    return response.status === 204 ? (undefined as T) : response.json();
  }

  get<T>(path: string, params?: Record<string, string>) {
    return this.request<T>("GET", path, { params });
  }

  post<T>(path: string, body?: unknown) {
    return this.request<T>("POST", path, { body });
  }

  put<T>(path: string, body?: unknown) {
    return this.request<T>("PUT", path, { body });
  }

  delete<T>(path: string) {
    return this.request<T>("DELETE", path);
  }
}

export const api = new ApiClient(process.env.NEXT_PUBLIC_API_URL!);
```

## React Query Integration

```typescript
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";

// Query keys factory
export const userKeys = {
  all: ["users"] as const,
  lists: () => [...userKeys.all, "list"] as const,
  list: (filters: string) => [...userKeys.lists(), { filters }] as const,
  details: () => [...userKeys.all, "detail"] as const,
  detail: (id: string) => [...userKeys.details(), id] as const,
};

// Query hook
export function useUsers(filters?: { search?: string }) {
  return useQuery({
    queryKey: filters ? userKeys.list(JSON.stringify(filters)) : userKeys.lists(),
    queryFn: () => api.get<User[]>("/users", filters as any),
    staleTime: 5 * 60 * 1000,
  });
}

// Mutation with optimistic updates
export function useUpdateUser() {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: ({ id, data }: { id: string; data: Partial<User> }) =>
      api.put<User>(`/users/${id}`, data),
    onMutate: async ({ id, data }) => {
      await queryClient.cancelQueries({ queryKey: userKeys.detail(id) });
      const previous = queryClient.getQueryData<User>(userKeys.detail(id));
      queryClient.setQueryData<User>(userKeys.detail(id), (old) =>
        old ? { ...old, ...data } : undefined
      );
      return { previous };
    },
    onError: (_, { id }, context) => {
      if (context?.previous) {
        queryClient.setQueryData(userKeys.detail(id), context.previous);
      }
    },
    onSettled: (_, __, { id }) => {
      queryClient.invalidateQueries({ queryKey: userKeys.detail(id) });
    },
  });
}
```

## tRPC (End-to-End Type Safety)

```typescript
// server/routers/user.ts
import { z } from "zod";
import { router, publicProcedure } from "../trpc";

export const userRouter = router({
  list: publicProcedure
    .input(z.object({ search: z.string().optional() }).optional())
    .query(async ({ input, ctx }) => {
      return ctx.db.user.findMany({
        where: input?.search ? { name: { contains: input.search } } : undefined,
      });
    }),

  create: publicProcedure
    .input(z.object({ name: z.string(), email: z.string().email() }))
    .mutation(async ({ input, ctx }) => {
      return ctx.db.user.create({ data: input });
    }),
});

// client usage - fully type-safe!
const { data: users } = trpc.user.list.useQuery({ search: "John" });
const createUser = trpc.user.create.useMutation();
createUser.mutate({ name: "Alice", email: "alice@example.com" });
```

## WebSocket Integration

```typescript
type MessageHandler<T> = (data: T) => void;

class TypedWebSocket {
  private ws: WebSocket | null = null;
  private handlers = new Map<string, Set<MessageHandler<any>>>();

  constructor(private url: string) {}

  connect() {
    this.ws = new WebSocket(this.url);
    this.ws.onmessage = (event) => {
      const { type, data } = JSON.parse(event.data);
      this.handlers.get(type)?.forEach((h) => h(data));
    };
  }

  on<T>(type: string, handler: MessageHandler<T>): () => void {
    if (!this.handlers.has(type)) this.handlers.set(type, new Set());
    this.handlers.get(type)!.add(handler);
    return () => this.handlers.get(type)?.delete(handler);
  }

  send<T>(type: string, data: T) {
    this.ws?.send(JSON.stringify({ type, data }));
  }

  disconnect() {
    this.ws?.close();
  }
}

// React hook
function useWebSocket<T>(url: string, type: string, handler: (data: T) => void) {
  const wsRef = useRef<TypedWebSocket>();

  useEffect(() => {
    const ws = new TypedWebSocket(url);
    ws.connect();
    wsRef.current = ws;
    const unsubscribe = ws.on(type, handler);
    return () => {
      unsubscribe();
      ws.disconnect();
    };
  }, [url, type, handler]);

  return wsRef.current;
}
```

## Error Handling

```typescript
// Result type pattern
type Result<T, E = Error> =
  | { success: true; data: T }
  | { success: false; error: E };

async function safeApiCall<T>(fn: () => Promise<T>): Promise<Result<T>> {
  try {
    const data = await fn();
    return { success: true, data };
  } catch (error) {
    return { success: false, error: error as Error };
  }
}

// Usage
const result = await safeApiCall(() => api.get<User>("/users/123"));
if (result.success) {
  console.log(result.data.name);
} else {
  console.error(result.error.message);
}
```

## Critical Rules

### Always
- Type all API requests/responses
- Handle errors explicitly
- Use React Query for server state
- Implement request cancellation
- Add loading/error states

### Never
- Use any for API responses
- Skip error handling
- Store API data in local state
- Forget network failure handling
- Make requests in useEffect (use React Query)
