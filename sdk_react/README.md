# @allyourbase/react

React integration helpers for Allyourbase.

## Quick start

```tsx
import { AYBClient } from "@allyourbase/js";
import { AYBProvider, useQuery } from "@allyourbase/react";

const client = new AYBClient("http://localhost:8090");

function Posts() {
  const { data, loading } = useQuery("posts");
  if (loading) return <div>Loading...</div>;
  return <pre>{JSON.stringify(data?.items)}</pre>;
}

export function App() {
  return (
    <AYBProvider client={client}>
      <Posts />
    </AYBProvider>
  );
}
```
