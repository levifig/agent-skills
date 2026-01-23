# Form Patterns

Type-safe, accessible forms with React Hook Form and Zod.

## Form Stack

| Component | Tool | Purpose |
|-----------|------|---------|
| Form Library | React Hook Form | Type-safe forms |
| Validation | Zod | Schema validation |
| Submission | Server Actions | Server-side handling |

## React Hook Form Basics

```typescript
import { useForm } from "react-hook-form";
import { zodResolver } from "@hookform/resolvers/zod";
import { z } from "zod";

const loginSchema = z.object({
  email: z.string().email("Invalid email"),
  password: z.string().min(8, "Min 8 characters"),
  rememberMe: z.boolean().optional(),
});

type LoginFormData = z.infer<typeof loginSchema>;

export function LoginForm() {
  const {
    register,
    handleSubmit,
    formState: { errors, isSubmitting },
  } = useForm<LoginFormData>({
    resolver: zodResolver(loginSchema),
  });

  const onSubmit = async (data: LoginFormData) => {
    await api.login(data);
  };

  return (
    <form onSubmit={handleSubmit(onSubmit)}>
      <div>
        <label htmlFor="email">Email</label>
        <input id="email" type="email" {...register("email")} aria-invalid={!!errors.email} />
        {errors.email && <span role="alert">{errors.email.message}</span>}
      </div>

      <div>
        <label htmlFor="password">Password</label>
        <input id="password" type="password" {...register("password")} />
        {errors.password && <span role="alert">{errors.password.message}</span>}
      </div>

      <label>
        <input type="checkbox" {...register("rememberMe")} />
        Remember me
      </label>

      <button type="submit" disabled={isSubmitting}>
        {isSubmitting ? "Loading..." : "Login"}
      </button>
    </form>
  );
}
```

## Zod Schema Patterns

```typescript
import { z } from "zod";

// Basic types
const userSchema = z.object({
  name: z.string().min(1, "Required"),
  email: z.string().email(),
  age: z.number().int().min(18),
  website: z.string().url().optional(),
});

// Enums
const roleSchema = z.enum(["admin", "user", "guest"]);

// Discriminated unions
const notificationSchema = z.discriminatedUnion("type", [
  z.object({ type: z.literal("email"), email: z.string().email() }),
  z.object({ type: z.literal("sms"), phone: z.string() }),
]);

// Custom validation
const passwordSchema = z
  .string()
  .min(8)
  .regex(/[A-Z]/, "Must contain uppercase")
  .regex(/[0-9]/, "Must contain number");

// Refinements for cross-field validation
const signupSchema = z
  .object({
    password: z.string(),
    confirmPassword: z.string(),
  })
  .refine((data) => data.password === data.confirmPassword, {
    message: "Passwords don't match",
    path: ["confirmPassword"],
  });

// Transform data
const dateSchema = z.string().transform((str) => new Date(str));
```

## Server Actions Integration

```typescript
// app/users/actions.ts
"use server";

import { z } from "zod";
import { revalidatePath } from "next/cache";
import { redirect } from "next/navigation";

const createUserSchema = z.object({
  name: z.string().min(1),
  email: z.string().email(),
  role: z.enum(["admin", "user"]),
});

export type FormState = {
  errors?: Record<string, string[]>;
  success?: boolean;
};

export async function createUser(
  prevState: FormState,
  formData: FormData
): Promise<FormState> {
  const result = createUserSchema.safeParse({
    name: formData.get("name"),
    email: formData.get("email"),
    role: formData.get("role"),
  });

  if (!result.success) {
    return { errors: result.error.flatten().fieldErrors };
  }

  try {
    await db.user.create({ data: result.data });
    revalidatePath("/users");
    redirect("/users");
  } catch {
    return { errors: { _form: ["Failed to create user"] } };
  }
}

// Client component
"use client";

import { useFormState, useFormStatus } from "react-dom";
import { createUser } from "./actions";

function SubmitButton() {
  const { pending } = useFormStatus();
  return (
    <button type="submit" disabled={pending}>
      {pending ? "Creating..." : "Create"}
    </button>
  );
}

export default function NewUserForm() {
  const [state, formAction] = useFormState(createUser, {});

  return (
    <form action={formAction}>
      <input name="name" required />
      {state.errors?.name && <span>{state.errors.name}</span>}

      <input name="email" type="email" required />
      {state.errors?.email && <span>{state.errors.email}</span>}

      <select name="role">
        <option value="user">User</option>
        <option value="admin">Admin</option>
      </select>

      <SubmitButton />
    </form>
  );
}
```

## File Uploads

```typescript
const fileSchema = z.object({
  file: z
    .instanceof(File)
    .refine((f) => f.size <= 5_000_000, "Max 5MB")
    .refine(
      (f) => ["image/jpeg", "image/png"].includes(f.type),
      "Only .jpg/.png"
    ),
});

function FileUploadForm() {
  const { register, handleSubmit } = useForm<{ file: File }>({
    resolver: zodResolver(fileSchema),
  });

  const onSubmit = async (data: { file: File }) => {
    const formData = new FormData();
    formData.append("file", data.file);
    await fetch("/api/upload", { method: "POST", body: formData });
  };

  return (
    <form onSubmit={handleSubmit(onSubmit)}>
      <input type="file" {...register("file")} accept="image/jpeg,image/png" />
      <button type="submit">Upload</button>
    </form>
  );
}
```

## Multi-Step Forms

```typescript
function useMultiStepForm<T>(steps: number) {
  const [currentStep, setCurrentStep] = useState(0);
  const [formData, setFormData] = useState<Partial<T>>({});

  return {
    currentStep,
    formData,
    next: (data: Partial<T>) => {
      setFormData((prev) => ({ ...prev, ...data }));
      setCurrentStep((s) => Math.min(s + 1, steps - 1));
    },
    prev: () => setCurrentStep((s) => Math.max(s - 1, 0)),
    isFirst: currentStep === 0,
    isLast: currentStep === steps - 1,
  };
}

// Usage
function WizardForm() {
  const { currentStep, formData, next, prev, isLast } = useMultiStepForm<CompleteData>(3);

  return (
    <div>
      {currentStep === 0 && <Step1 onNext={next} defaultValues={formData} />}
      {currentStep === 1 && <Step2 onNext={next} onPrev={prev} defaultValues={formData} />}
      {currentStep === 2 && <Step3 onSubmit={handleFinal} onPrev={prev} defaultValues={formData} />}
    </div>
  );
}
```

## Critical Rules

### Always
- Use Zod for schema validation
- Validate on both client and server
- Include proper ARIA attributes
- Handle submission states
- Use uncontrolled inputs by default

### Never
- Trust client-side validation only
- Skip error messages
- Forget disabled state during submission
- Ignore accessibility
- Use controlled inputs unnecessarily
