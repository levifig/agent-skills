# State Management

Modern state management patterns for TypeScript applications.

## State Decision Guide

| Scenario | Solution |
|----------|----------|
| API data | React Query |
| Global client state | Zustand |
| Shareable state (URL) | searchParams |
| Component tree | Context + Reducer |
| Single component | useState |
| Persistent | localStorage hook |

## Zustand (Client State)

```typescript
// stores/auth-store.ts
import { create } from "zustand";
import { persist } from "zustand/middleware";

interface User {
  id: string;
  name: string;
  email: string;
}

interface AuthState {
  user: User | null;
  token: string | null;
  isAuthenticated: boolean;
  login: (email: string, password: string) => Promise<void>;
  logout: () => void;
}

export const useAuthStore = create<AuthState>()(
  persist(
    (set) => ({
      user: null,
      token: null,
      isAuthenticated: false,

      login: async (email, password) => {
        const { user, token } = await api.login(email, password);
        set({ user, token, isAuthenticated: true });
      },

      logout: () => {
        set({ user: null, token: null, isAuthenticated: false });
      },
    }),
    {
      name: "auth-storage",
      partialize: (state) => ({ token: state.token }),
    }
  )
);

// Usage with selectors (performance)
const user = useAuthStore((state) => state.user);
const logout = useAuthStore((state) => state.logout);
```

### Zustand Slices Pattern

```typescript
import { create } from "zustand";

interface UISlice {
  sidebarOpen: boolean;
  theme: "light" | "dark";
  toggleSidebar: () => void;
}

interface UserSlice {
  users: User[];
  selectedUserId: string | null;
  selectUser: (id: string) => void;
}

type AppState = UISlice & UserSlice;

const createUISlice = (set: any): UISlice => ({
  sidebarOpen: true,
  theme: "light",
  toggleSidebar: () => set((s: any) => ({ sidebarOpen: !s.sidebarOpen })),
});

const createUserSlice = (set: any): UserSlice => ({
  users: [],
  selectedUserId: null,
  selectUser: (id) => set({ selectedUserId: id }),
});

export const useAppStore = create<AppState>()((...args) => ({
  ...createUISlice(...args),
  ...createUserSlice(...args),
}));
```

## React Query (Server State)

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

// Queries
export function useUsers() {
  return useQuery({
    queryKey: userKeys.lists(),
    queryFn: () => api.getUsers(),
    staleTime: 5 * 60 * 1000,
  });
}

export function useUser(id: string) {
  return useQuery({
    queryKey: userKeys.detail(id),
    queryFn: () => api.getUser(id),
    enabled: !!id,
  });
}

// Mutations with optimistic updates
export function useUpdateUser() {
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: ({ id, data }: { id: string; data: Partial<User> }) =>
      api.updateUser(id, data),
    onMutate: async ({ id, data }) => {
      await queryClient.cancelQueries({ queryKey: userKeys.detail(id) });
      const previous = queryClient.getQueryData(userKeys.detail(id));
      queryClient.setQueryData(userKeys.detail(id), (old: User) => ({ ...old, ...data }));
      return { previous };
    },
    onError: (err, { id }, context) => {
      queryClient.setQueryData(userKeys.detail(id), context?.previous);
    },
    onSettled: (_, __, { id }) => {
      queryClient.invalidateQueries({ queryKey: userKeys.detail(id) });
    },
  });
}
```

## Context + Reducer

```typescript
import { createContext, useContext, useReducer } from "react";

interface CartItem {
  id: string;
  name: string;
  price: number;
  quantity: number;
}

interface CartState {
  items: CartItem[];
  total: number;
}

type CartAction =
  | { type: "ADD_ITEM"; payload: CartItem }
  | { type: "REMOVE_ITEM"; payload: { id: string } }
  | { type: "CLEAR_CART" };

function cartReducer(state: CartState, action: CartAction): CartState {
  switch (action.type) {
    case "ADD_ITEM": {
      const existing = state.items.find((i) => i.id === action.payload.id);
      if (existing) {
        return {
          ...state,
          items: state.items.map((i) =>
            i.id === action.payload.id
              ? { ...i, quantity: i.quantity + action.payload.quantity }
              : i
          ),
          total: state.total + action.payload.price * action.payload.quantity,
        };
      }
      return {
        items: [...state.items, action.payload],
        total: state.total + action.payload.price * action.payload.quantity,
      };
    }
    case "REMOVE_ITEM": {
      const item = state.items.find((i) => i.id === action.payload.id);
      return {
        items: state.items.filter((i) => i.id !== action.payload.id),
        total: state.total - (item?.price ?? 0) * (item?.quantity ?? 0),
      };
    }
    case "CLEAR_CART":
      return { items: [], total: 0 };
  }
}

const CartContext = createContext<{
  state: CartState;
  dispatch: React.Dispatch<CartAction>;
} | undefined>(undefined);

export function CartProvider({ children }: { children: React.ReactNode }) {
  const [state, dispatch] = useReducer(cartReducer, { items: [], total: 0 });
  return (
    <CartContext.Provider value={{ state, dispatch }}>
      {children}
    </CartContext.Provider>
  );
}

export function useCart() {
  const context = useContext(CartContext);
  if (!context) throw new Error("useCart must be used within CartProvider");
  return context;
}
```

## URL State

```typescript
import { useSearchParams } from "next/navigation";
import { useCallback } from "react";

export function useUrlState<T extends string>(
  key: string,
  defaultValue: T
): [T, (value: T) => void] {
  const searchParams = useSearchParams();
  const value = (searchParams.get(key) as T) || defaultValue;

  const setValue = useCallback((newValue: T) => {
    const params = new URLSearchParams(searchParams.toString());
    params.set(key, newValue);
    window.history.pushState(null, "", `?${params.toString()}`);
  }, [key, searchParams]);

  return [value, setValue];
}

// Usage
function ProductFilters() {
  const [sortBy, setSortBy] = useUrlState<"name" | "price">("sort", "name");
  // Shareable URL: ?sort=price
}
```

## Critical Rules

### Always
- Use React Query for server state
- Keep state as local as possible
- Type all state and actions
- Use selectors in Zustand
- Consider URL state for filters

### Never
- Store API data in Zustand
- Create global state for everything
- Forget loading/error states
- Mutate state directly
- Skip cache invalidation
