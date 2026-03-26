# @allyourbase/ssr

Server-side cookie/session helpers for Allyourbase.

## Quick start

```ts
import { loadServerSession } from "@allyourbase/ssr";

const result = await loadServerSession({
  cookieHeader: request.headers.get("cookie") ?? "",
  client: ayb,
});
```
