# Next.js 14+ with TypeScript

Building modern web applications with Next.js App Router and Server Components.

## App Router Structure

```
app/
├── layout.tsx           # Root layout (required)
├── page.tsx            # Home page
├── loading.tsx         # Loading UI
├── error.tsx           # Error UI
├── not-found.tsx       # 404 page
│
├── (marketing)/        # Route group (no URL segment)
│   ├── layout.tsx
│   ├── about/
│   └── contact/
│
├── blog/
│   ├── page.tsx        # /blog
│   ├── [slug]/
│   │   └── page.tsx    # /blog/:slug
│   └── loading.tsx
│
└── api/
    └── posts/
        └── route.ts    # API route handler
```

## Server Components (Default)

```typescript
// app/posts/page.tsx
interface Post {
  id: string;
  title: string;
  content: string;
}

// This is a Server Component by default
export default async function PostsPage() {
  const posts = await fetch("https://api.example.com/posts", {
    next: { revalidate: 3600 }, // ISR: revalidate every hour
  }).then((res) => res.json());

  return (
    <div>
      <h1>Blog Posts</h1>
      <ul>
        {posts.map((post: Post) => (
          <li key={post.id}>
            <a href={`/posts/${post.id}`}>{post.title}</a>
          </li>
        ))}
      </ul>
    </div>
  );
}
```

## Client Components

```typescript
// app/components/counter.tsx
"use client"; // Required directive for client components

import { useState } from "react";

export function Counter({ initialCount = 0 }: { initialCount?: number }) {
  const [count, setCount] = useState(initialCount);

  return (
    <div>
      <p>Count: {count}</p>
      <button onClick={() => setCount(count + 1)}>Increment</button>
    </div>
  );
}
```

## Dynamic Routes and Params

```typescript
// app/posts/[slug]/page.tsx
interface PageProps {
  params: { slug: string };
  searchParams: Record<string, string | string[] | undefined>;
}

export default async function PostPage({ params, searchParams }: PageProps) {
  const post = await getPost(params.slug);

  return (
    <article>
      <h1>{post.title}</h1>
      <div>{post.content}</div>
    </article>
  );
}

// Generate static params at build time
export async function generateStaticParams() {
  const posts = await getPosts();
  return posts.map((post) => ({ slug: post.slug }));
}
```

## Metadata API

```typescript
import type { Metadata } from "next";

// Static metadata
export const metadata: Metadata = {
  title: "Blog Post",
  description: "Read our latest blog post",
};

// Dynamic metadata
export async function generateMetadata({ params }: PageProps): Promise<Metadata> {
  const post = await getPost(params.slug);

  return {
    title: post.title,
    description: post.excerpt,
    openGraph: {
      title: post.title,
      description: post.excerpt,
      images: [{ url: post.imageUrl }],
    },
  };
}
```

## Server Actions

```typescript
// app/posts/new/page.tsx
import { redirect } from "next/navigation";
import { revalidatePath } from "next/cache";

// Server Action - runs on the server
async function createPost(formData: FormData) {
  "use server";

  const title = formData.get("title") as string;
  const content = formData.get("content") as string;

  if (!title || !content) {
    throw new Error("Title and content are required");
  }

  await db.post.create({ data: { title, content } });

  revalidatePath("/posts");
  redirect("/posts");
}

export default function NewPostPage() {
  return (
    <form action={createPost}>
      <input name="title" type="text" required />
      <textarea name="content" required />
      <button type="submit">Create Post</button>
    </form>
  );
}
```

## Data Fetching Patterns

### Parallel Data Fetching

```typescript
export default async function DashboardPage() {
  // Fetch in parallel
  const [user, posts] = await Promise.all([getUser(), getPosts()]);

  return (
    <div>
      <UserProfile user={user} />
      <PostList posts={posts} />
    </div>
  );
}
```

### Streaming with Suspense

```typescript
import { Suspense } from "react";

async function Posts() {
  const posts = await getPosts(); // Slow data fetch
  return <PostList posts={posts} />;
}

export default function HomePage() {
  return (
    <div>
      <h1>Home</h1>
      <Suspense fallback={<PostsSkeleton />}>
        <Posts />
      </Suspense>
    </div>
  );
}
```

## Route Handlers (API Routes)

```typescript
// app/api/posts/route.ts
import { NextRequest, NextResponse } from "next/server";

export async function GET(request: NextRequest) {
  const searchParams = request.nextUrl.searchParams;
  const page = searchParams.get("page") || "1";
  const posts = await getPosts(parseInt(page));

  return NextResponse.json({ posts });
}

export async function POST(request: NextRequest) {
  const body = await request.json();

  if (!body.title) {
    return NextResponse.json({ error: "Title is required" }, { status: 400 });
  }

  const post = await createPost(body);
  return NextResponse.json({ post }, { status: 201 });
}
```

## Middleware

```typescript
// middleware.ts
import { NextResponse } from "next/server";
import type { NextRequest } from "next/server";

export function middleware(request: NextRequest) {
  const token = request.cookies.get("token")?.value;

  if (!token) {
    return NextResponse.redirect(new URL("/login", request.url));
  }

  return NextResponse.next();
}

export const config = {
  matcher: ["/dashboard/:path*", "/admin/:path*"],
};
```

## Critical Rules

### Always
- Use Server Components by default
- Add "use client" only when needed
- Type params and searchParams
- Use Server Actions for mutations
- Implement proper error boundaries

### Never
- Fetch in Client Components (use Server Components)
- Use useState for URL state (use searchParams)
- Forget to await async components
- Import server code in client components
- Skip loading and error states
