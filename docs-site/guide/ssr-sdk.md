<!-- audited 2026-03-20 -->

# SSR SDK

The `@allyourbase/ssr` package provides server-side session and cookie helpers for frameworks like Next.js, SvelteKit, and Remix.

## Install

```bash
npm install @allyourbase/js @allyourbase/ssr
```

## Initialize

Create a regular JS client first:

```ts
import { AYBClient } from "@allyourbase/js";

export const ayb = new AYBClient("http://localhost:8090");
```

Then use SSR helpers with incoming request cookies.

## Load server session

`loadServerSession`:

- reads access/refresh tokens from cookies
- verifies with `auth.me()`
- attempts refresh on auth failure (`401`/`403`)
- returns `setCookieHeaders` you should write on the response

```ts
import { loadServerSession } from "@allyourbase/ssr";

const result = await loadServerSession({
  cookieHeader: request.headers.get("cookie") ?? "",
  client: ayb,
});

if (result.session) {
  console.log(result.session.user);
}

// Apply result.setCookieHeaders to your HTTP response.
```

## Protecting routes

### Next.js middleware

```ts
import { NextRequest, NextResponse } from "next/server";
import { AYBClient } from "@allyourbase/js";
import { applyNextSetCookies, loadServerSession, nextCookieHeader } from "@allyourbase/ssr";

const ayb = new AYBClient(process.env.AYB_URL ?? "http://localhost:8090");

export async function middleware(request: NextRequest) {
  const result = await loadServerSession({
    cookieHeader: nextCookieHeader(request),
    client: ayb,
  });

  if (!result.session) {
    const response = NextResponse.redirect(new URL("/login", request.url));
    applyNextSetCookies(response, result.setCookieHeaders);
    return response;
  }

  const response = NextResponse.next();
  applyNextSetCookies(response, result.setCookieHeaders);
  return response;
}
```

### SvelteKit handle hook

```ts
import { type Handle } from "@sveltejs/kit";
import { AYBClient } from "@allyourbase/js";
import { applySvelteKitSetCookies, loadServerSession, svelteKitCookieHeader } from "@allyourbase/ssr";

const ayb = new AYBClient("http://localhost:8090");

export const handle: Handle = async ({ event, resolve }) => {
  const result = await loadServerSession({
    cookieHeader: svelteKitCookieHeader(event),
    client: ayb,
  });

  if (!result.session && event.url.pathname.startsWith("/app")) {
    const response = new Response(null, {
      status: 302,
      headers: { location: "/login" },
    });
    applySvelteKitSetCookies(response.headers, result.setCookieHeaders);
    return response;
  }

  const response = await resolve(event);
  applySvelteKitSetCookies(response.headers, result.setCookieHeaders);
  return response;
};
```

### Remix loader

```ts
import { json, redirect, type LoaderFunctionArgs } from "@remix-run/node";
import { AYBClient } from "@allyourbase/js";
import { loadServerSession, remixCookieHeader, remixSetCookiesHeaders } from "@allyourbase/ssr";

const ayb = new AYBClient("http://localhost:8090");

export async function loader({ request }: LoaderFunctionArgs) {
  const result = await loadServerSession({
    cookieHeader: remixCookieHeader(request),
    client: ayb,
  });

  if (!result.session) {
    throw redirect("/login", {
      headers: remixSetCookiesHeaders(result.setCookieHeaders),
    });
  }

  return json(
    { user: result.session.user },
    { headers: remixSetCookiesHeaders(result.setCookieHeaders) },
  );
}
```

## Token refresh in SSR

`loadServerSession` (implemented in `sdk_ssr/src/session.ts`) runs this sequence:

1. Parse access/refresh cookies (`getSessionTokens`).
2. Call `client.auth.me()`.
3. If auth fails with `401`/`403`, call `client.auth.refresh()`.
4. On refresh success, return a session plus two `Set-Cookie` headers for rotated tokens.
5. On refresh failure, clear client tokens and return cookie-clearing headers (`Max-Age=0`).

This means your SSR boundary should always forward `setCookieHeaders` to the response, even when the user is redirected.

## Error handling

`loadServerSession` treats authentication failures as session state transitions and rethrows non-auth errors.

```ts
import { loadServerSession } from "@allyourbase/ssr";

try {
  const result = await loadServerSession({
    cookieHeader: request.headers.get("cookie") ?? "",
    client: ayb,
  });

  if (!result.session) {
    // unauthenticated (or refresh failed): redirect and clear cookies
    return { status: 302, location: "/login", setCookieHeaders: result.setCookieHeaders };
  }

  return { status: 200, user: result.session.user, setCookieHeaders: result.setCookieHeaders };
} catch (err) {
  // non-auth infrastructure errors (network outage, upstream failure, etc.)
  console.error("SSR auth check failed", err);
  return { status: 503 };
}
```

## Load server user only

```ts
import { loadServerUser } from "@allyourbase/ssr";

const user = await loadServerUser({
  cookieHeader: request.headers.get("cookie") ?? "",
  client: ayb,
});
```

## Cookie helpers

```ts
import {
  parseCookieHeader,
  getSessionTokens,
  serializeCookie,
  clearSessionCookies,
} from "@allyourbase/ssr";

const parsed = parseCookieHeader("a=1; b=2");
const { token, refreshToken } = getSessionTokens(request.headers.get("cookie") ?? "");

const accessCookie = serializeCookie("ayb_token", token ?? "", {
  secure: true,
  httpOnly: true,
  sameSite: "lax",
});

const clearCookies = clearSessionCookies();
```

You can customize cookie names/options with `cookieOptions` passed to `loadServerSession`.

Default cookie options: `accessTokenName: "ayb_token"`, `refreshTokenName: "ayb_refresh_token"`,
`path: "/"`, `secure: true`, `httpOnly: true`, `sameSite: "lax"`, `maxAge: 2592000` (30 days).

## Adapter helpers

`@allyourbase/ssr` includes framework adapters:

- Next.js: `nextCookieHeader`, `applyNextSetCookies`
- SvelteKit: `svelteKitCookieHeader`, `applySvelteKitSetCookies`
- Remix: `remixCookieHeader`, `remixSetCookiesHeaders`

## Related guides

- [Authentication](/guide/authentication)
- [JavaScript SDK](/guide/javascript-sdk)
