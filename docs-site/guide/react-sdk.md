<!-- audited 2026-03-20 -->

# React SDK

The `@allyourbase/react` package provides React primitives on top of `@allyourbase/js`:

- `AYBProvider`
- `useAYBClient`
- `useAuth`
- `useQuery`
- `useRealtime`

## Install

```bash
npm install @allyourbase/js @allyourbase/react react
```

## Initialize

```tsx
import { AYBClient } from "@allyourbase/js";
import { AYBProvider } from "@allyourbase/react";

const ayb = new AYBClient("http://localhost:8090");

export function App({ children }: { children: React.ReactNode }) {
  return <AYBProvider client={ayb}>{children}</AYBProvider>;
}
```

`useAuth`, `useQuery`, and `useRealtime` must run under `AYBProvider`.

## `useAuth`

`useAuth()` tracks user/session state and wraps auth actions.

```tsx
import { useAuth } from "@allyourbase/react";

export function LoginPanel() {
  const {
    loading,
    user,
    error,
    token,
    refreshToken,
    login,
    register,
    logout,
    refresh,
  } = useAuth();

  if (loading) return <p>Loading session...</p>;

  return (
    <div>
      <p>user: {user?.email ?? "anonymous"}</p>
      <p>token: {token ? "set" : "missing"}</p>
      <p>refresh: {refreshToken ? "set" : "missing"}</p>
      {error && <p>{error.message}</p>}
      <button onClick={() => login("user@example.com", "password")}>Login</button>
      <button onClick={() => register("new@example.com", "password")}>Register</button>
      <button onClick={() => refresh()}>Refresh</button>
      <button onClick={() => logout()}>Logout</button>
    </div>
  );
}
```

## `useQuery`

`useQuery(collection, params?, options?)` wraps `client.records.list()`.

```tsx
import { useQuery } from "@allyourbase/react";

type Post = {
  id: number;
  title: string;
  published: boolean;
};

export function PostList() {
  const { data, loading, error, refetch } = useQuery<Post>(
    "posts",
    { filter: "published=true", sort: "-created_at", perPage: 20 },
    { enabled: true },
  );

  if (loading) return <p>Loading...</p>;
  if (error) return <p>{error.message}</p>;

  return (
    <div>
      <button onClick={() => refetch()}>Refresh</button>
      <ul>
        {data?.items.map((post) => (
          <li key={post.id}>{post.title}</li>
        ))}
      </ul>
    </div>
  );
}
```

### Suspense mode

```tsx
const { data } = useQuery("posts", { sort: "-created_at" }, { suspense: true });
```

When `suspense: true`, the hook throws the fetch promise/errors for a Suspense boundary.

## Mutations

`@allyourbase/react` currently ships `useQuery` (read path) and `useAYBClient` (raw client access).

```tsx
import { useState } from "react";
import { AYBClient } from "@allyourbase/js";
import { useAYBClient, useQuery } from "@allyourbase/react";

export function TodoMutations() {
  const client = useAYBClient() as AYBClient;
  const { data, refetch } = useQuery<{ id: string; title: string; done: boolean }>("todos");
  const [title, setTitle] = useState("");

  const createTodo = async () => {
    await client.records.create("todos", { title, done: false });
    setTitle("");
    await refetch();
  };

  const toggleTodo = async (id: string, done: boolean) => {
    await client.records.update("todos", id, { done: !done });
    await refetch();
  };

  const deleteTodo = async (id: string) => {
    await client.records.delete("todos", id);
    await refetch();
  };

  return (
    <div>
      <input value={title} onChange={(e) => setTitle(e.target.value)} />
      <button onClick={createTodo}>Add</button>
      <ul>
        {data?.items.map((todo) => (
          <li key={todo.id}>
            <button onClick={() => toggleTodo(todo.id, todo.done)}>
              {todo.done ? "Undo" : "Done"}
            </button>
            <button onClick={() => deleteTodo(todo.id)}>Delete</button>
            {todo.title}
          </li>
        ))}
      </ul>
    </div>
  );
}
```

## Error handling

The JS SDK throws `AYBError` for non-2xx responses. Use its fields (`status`, `code`, `data`, `docUrl`) for user-safe handling.

```tsx
import { AYBClient, AYBError } from "@allyourbase/js";
import { useAYBClient } from "@allyourbase/react";

export function SaveButton() {
  const client = useAYBClient() as AYBClient;

  const onSave = async () => {
    try {
      await client.records.create("posts", { title: "Hello" });
    } catch (err) {
      if (err instanceof AYBError) {
        if (err.status === 429) {
          alert("Rate limited. Try again shortly.");
          return;
        }

        if (err.code === "validation/failed") {
          console.error("Validation details", err.data);
        }

        console.error("AYB error", err.status, err.code, err.docUrl);
        return;
      }

      console.error("Unexpected error", err);
    }
  };

  return <button onClick={onSave}>Save</button>;
}
```

## OAuth sign-in

OAuth popup/redirect behavior is implemented in `@allyourbase/js` (`signInWithOAuth`).

```tsx
import { useState } from "react";
import { AYBClient, AYBError } from "@allyourbase/js";
import { useAYBClient } from "@allyourbase/react";

export function OAuthButtons() {
  const client = useAYBClient() as AYBClient;
  const [error, setError] = useState<string | null>(null);

  const signInGoogle = async () => {
    setError(null);
    try {
      await client.auth.signInWithOAuth("google");
    } catch (err) {
      if (err instanceof AYBError) {
        setError(`${err.code ?? "oauth/error"}: ${err.message}`);
        return;
      }
      setError("OAuth sign-in failed");
    }
  };

  return (
    <div>
      <button onClick={signInGoogle}>Continue with Google</button>
      {error && <p>{error}</p>}
    </div>
  );
}
```

If you cannot use popups (for example, native wrappers), pass a `urlCallback` and handle redirect manually:

```ts
await client.auth.signInWithOAuth("github", {
  urlCallback: async (url) => {
    window.location.assign(url);
  },
});
```

## `useRealtime`

`useRealtime(tables, callback)` subscribes to realtime events and cleans up automatically.

```tsx
import { useRealtime } from "@allyourbase/react";

export function RealtimeFeed() {
  useRealtime(["posts", "comments"], (event) => {
    console.log(event);
  });

  return <p>Watching posts/comments...</p>;
}
```

## Accessing the raw client

```tsx
import { AYBClient } from "@allyourbase/js";
import { useAYBClient } from "@allyourbase/react";

export function ApiKeyMode() {
  const ayb = useAYBClient() as AYBClient;
  ayb.setApiKey("ayb_api_key_xxx");
  return null;
}
```

## Related guides

- [JavaScript SDK](/guide/javascript-sdk)
- [Realtime](/guide/realtime)
- [Authentication](/guide/authentication)
