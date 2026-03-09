import { AYBError } from "./errors";
import type {
  AuthResponse,
  AuthStateListener,
  BatchOperation,
  BatchResult,
  ClientOptions,
  GetParams,
  ListParams,
  ListResponse,
  OAuthOptions,
  OAuthProvider,
  RealtimeEvent,
  StorageObject,
  User,
} from "./types";

/**
 * Allyourbase JavaScript/TypeScript client.
 *
 * @example
 * ```ts
 * import { AYBClient } from "@allyourbase/js";
 *
 * const ayb = new AYBClient("http://localhost:8090");
 *
 * // List records
 * const posts = await ayb.records.list("posts", { filter: "published=true", sort: "-created_at" });
 *
 * // Auth
 * await ayb.auth.login("user@example.com", "password");
 * const me = await ayb.auth.me();
 * ```
 */
export class AYBClient {
  private baseURL: string;
  private _fetch: typeof globalThis.fetch;
  private _token: string | null = null;
  private _refreshToken: string | null = null;
  private _authListeners: Set<AuthStateListener> = new Set();

  readonly auth: AuthClient;
  readonly records: RecordsClient;
  readonly storage: StorageClient;
  readonly realtime: RealtimeClient;

  constructor(baseURL: string, options?: ClientOptions) {
    // Strip trailing slash.
    this.baseURL = baseURL.replace(/\/+$/, "");
    this._fetch = options?.fetch ?? globalThis.fetch.bind(globalThis);

    this.auth = new AuthClient(this);
    this.records = new RecordsClient(this);
    this.storage = new StorageClient(this);
    this.realtime = new RealtimeClient(this);
  }

  /** Current access token, if authenticated. */
  get token(): string | null {
    return this._token;
  }

  /** Current refresh token, if authenticated. */
  get refreshToken(): string | null {
    return this._refreshToken;
  }

  /** Manually set auth tokens (e.g. from storage). */
  setTokens(token: string, refreshToken: string): void {
    this._token = token;
    this._refreshToken = refreshToken;
  }

  /** Clear stored auth tokens. */
  clearTokens(): void {
    this._token = null;
    this._refreshToken = null;
  }

  /**
   * Authenticate with an API key instead of JWT tokens.
   * API keys are sent as Bearer tokens and validated server-side.
   * This clears any existing JWT refresh token.
   *
   * @example
   * ```ts
   * const ayb = new AYBClient("http://localhost:8090");
   * ayb.setApiKey("ayb_abc123...");
   * const posts = await ayb.records.list("posts"); // authenticated via API key
   * ```
   */
  setApiKey(apiKey: string): void {
    this._token = apiKey;
    this._refreshToken = null;
  }

  /**
   * Clear the API key (or JWT token). Equivalent to clearTokens().
   */
  clearApiKey(): void {
    this.clearTokens();
  }

  /**
   * Subscribe to auth state changes.
   * Returns an unsubscribe function.
   *
   * @example
   * ```ts
   * const unsub = ayb.onAuthStateChange((event, session) => {
   *   if (event === "SIGNED_IN") console.log("User signed in", session);
   *   if (event === "SIGNED_OUT") console.log("User signed out");
   * });
   * // Later: unsub();
   * ```
   */
  onAuthStateChange(listener: AuthStateListener): () => void {
    this._authListeners.add(listener);
    return () => {
      this._authListeners.delete(listener);
    };
  }

  /** @internal */
  emitAuthEvent(
    event: "SIGNED_IN" | "SIGNED_OUT" | "TOKEN_REFRESHED",
  ): void {
    const session =
      this._token && this._refreshToken
        ? { token: this._token, refreshToken: this._refreshToken }
        : null;
    for (const listener of this._authListeners) {
      listener(event, session);
    }
  }

  /** @internal */
  async request<T>(
    path: string,
    init?: RequestInit & { skipAuth?: boolean },
  ): Promise<T> {
    const headers: Record<string, string> = {
      ...(init?.headers as Record<string, string>),
    };
    if (!init?.skipAuth && this._token) {
      headers["Authorization"] = `Bearer ${this._token}`;
    }
    const url = `${this.baseURL}${path}`;
    const res = await this._fetch(url, { ...init, headers });
    if (!res.ok) {
      const body = await res.json().catch(() => ({ message: res.statusText }));
      throw new AYBError(
        res.status,
        body.message || res.statusText,
        undefined,
        body.data,
        body.doc_url,
      );
    }
    // Handle 204 No Content.
    if (res.status === 204) return undefined as T;
    return res.json();
  }

  /** @internal */
  setTokensInternal(token: string, refreshToken: string): void {
    this._token = token;
    this._refreshToken = refreshToken;
  }

  /**
   * Call a PostgreSQL function via the RPC endpoint.
   *
   * Void functions return `undefined`. Set-returning functions return an array.
   * Scalar functions return the unwrapped value.
   *
   * @example
   * ```ts
   * // Scalar function
   * const total = await ayb.rpc<number>("get_total_orders", { user_id: "abc" });
   *
   * // Set-returning function
   * const rows = await ayb.rpc<{ id: string; name: string }[]>("search_users", { query: "john" });
   *
   * // No-argument function
   * const version = await ayb.rpc<string>("pg_version");
   * ```
   */
  async rpc<T = unknown>(
    functionName: string,
    args?: Record<string, unknown>,
  ): Promise<T> {
    const hasArgs = args != null && Object.keys(args).length > 0;
    return this.request<T>(`/api/rpc/${functionName}`, {
      method: "POST",
      ...(hasArgs
        ? {
            headers: { "Content-Type": "application/json" },
            body: JSON.stringify(args),
          }
        : {}),
    });
  }

  /** @internal */
  getBaseURL(): string {
    return this.baseURL;
  }
}

class AuthClient {
  constructor(private client: AYBClient) {}

  /** Register a new user account. */
  async register(email: string, password: string): Promise<AuthResponse> {
    const res = await this.client.request<AuthResponse>("/api/auth/register", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ email, password }),
    });
    this.client.setTokensInternal(res.token, res.refreshToken);
    this.client.emitAuthEvent("SIGNED_IN");
    return res;
  }

  /** Log in with email and password. */
  async login(email: string, password: string): Promise<AuthResponse> {
    const res = await this.client.request<AuthResponse>("/api/auth/login", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ email, password }),
    });
    this.client.setTokensInternal(res.token, res.refreshToken);
    this.client.emitAuthEvent("SIGNED_IN");
    return res;
  }

  /** Refresh the access token using the stored refresh token. */
  async refresh(): Promise<AuthResponse> {
    const res = await this.client.request<AuthResponse>("/api/auth/refresh", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ refreshToken: this.client.refreshToken }),
    });
    this.client.setTokensInternal(res.token, res.refreshToken);
    this.client.emitAuthEvent("TOKEN_REFRESHED");
    return res;
  }

  /** Log out (revoke the refresh token). */
  async logout(): Promise<void> {
    await this.client.request<void>("/api/auth/logout", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ refreshToken: this.client.refreshToken }),
    });
    this.client.clearTokens();
    this.client.emitAuthEvent("SIGNED_OUT");
  }

  /** Get the current authenticated user. */
  async me(): Promise<User> {
    return this.client.request<User>("/api/auth/me");
  }

  /** Delete the current authenticated user's account. */
  async deleteAccount(): Promise<void> {
    await this.client.request<void>("/api/auth/me", { method: "DELETE" });
    this.client.clearTokens();
    this.client.emitAuthEvent("SIGNED_OUT");
  }

  /** Request a password reset email. */
  async requestPasswordReset(email: string): Promise<void> {
    await this.client.request<void>("/api/auth/password-reset", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ email }),
    });
  }

  /** Confirm a password reset with a token. */
  async confirmPasswordReset(token: string, password: string): Promise<void> {
    await this.client.request<void>("/api/auth/password-reset/confirm", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ token, password }),
    });
  }

  /** Verify an email address with a token. */
  async verifyEmail(token: string): Promise<void> {
    await this.client.request<void>("/api/auth/verify", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ token }),
    });
  }

  /** Resend the email verification (requires auth). */
  async resendVerification(): Promise<void> {
    await this.client.request<void>("/api/auth/verify/resend", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
    });
  }

  /**
   * Sign in with an OAuth provider using a popup + SSE flow.
   *
   * Opens a popup window immediately (to avoid Safari's popup blocker),
   * connects to SSE to get a clientId, then navigates the popup to the
   * provider's authorization page. The promise resolves when the backend
   * publishes the auth result via SSE.
   *
   * @example
   * ```ts
   * const result = await ayb.auth.signInWithOAuth("google");
   * console.log(result.user.email);
   * ```
   *
   * For redirect flow (e.g. iOS PWA where popups don't work):
   * ```ts
   * await ayb.auth.signInWithOAuth("google", {
   *   urlCallback: (url) => { window.location.href = url; }
   * });
   * ```
   */
  async signInWithOAuth(
    provider: OAuthProvider,
    options?: OAuthOptions,
  ): Promise<AuthResponse> {
    // 1. Open popup immediately (must be synchronous to avoid Safari blocker).
    let popup: Window | null = null;
    if (!options?.urlCallback) {
      popup = openPopup();
      if (!popup) {
        throw new AYBError(
          403,
          "Popup was blocked by the browser. Use urlCallback for redirect flow.",
          "oauth/popup-blocked",
        );
      }
    }

    try {
      // 2. Connect to SSE and get clientId (may reject with oauth/sse-failed).
      const { clientId, waitForAuth, close } = await this.connectOAuthSSE();

      // 3. Build OAuth URL with clientId as state.
      let oauthURL = `${this.client.getBaseURL()}/api/auth/oauth/${provider}?state=${clientId}`;
      if (options?.scopes?.length) {
        oauthURL += `&scopes=${encodeURIComponent(options.scopes.join(","))}`;
      }

      // 4. Navigate popup or call custom URL callback.
      if (options?.urlCallback) {
        await options.urlCallback(oauthURL);
      } else if (popup) {
        popup.location.href = oauthURL;
      }

      // 5. Wait for SSE event with auth result (with popup close detection).
      const result = await waitForAuth(popup);

      // 6. Store tokens.
      this.client.setTokensInternal(result.token, result.refreshToken);
      this.client.emitAuthEvent("SIGNED_IN");
      close();
      return result;
    } catch (err) {
      popup?.close();
      throw err;
    }
  }

  /**
   * Parse OAuth tokens from a URL hash fragment after a redirect-based OAuth flow.
   * Call this on your callback page when using `urlCallback` with `signInWithOAuth`.
   *
   * Returns tokens immediately and fires the `SIGNED_IN` auth state event.
   * The `user` field will be empty — call `ayb.auth.me()` to fetch the full profile.
   *
   * @example
   * ```ts
   * // On your OAuth callback page:
   * const result = ayb.auth.handleOAuthRedirect();
   * if (result) {
   *   console.log("Authenticated!", result.token);
   *   const user = await ayb.auth.me(); // fetch full user profile
   *   navigate("/dashboard");
   * }
   * ```
   *
   * @returns The auth response with tokens if found in the URL hash, or null.
   */
  handleOAuthRedirect(): AuthResponse | null {
    if (typeof window === "undefined") return null;
    const hash = window.location.hash;
    if (!hash) return null;
    const params = new URLSearchParams(hash.slice(1));
    const token = params.get("token");
    const refreshToken = params.get("refreshToken");
    if (!token || !refreshToken) return null;
    this.client.setTokensInternal(token, refreshToken);
    this.client.emitAuthEvent("SIGNED_IN");
    // Clean up the hash from the URL.
    window.history.replaceState(
      null,
      "",
      window.location.pathname + window.location.search,
    );
    return { token, refreshToken, user: {} as User };
  }

  /** @internal */
  private connectOAuthSSE(): Promise<{
    clientId: string;
    waitForAuth: (popup: Window | null) => Promise<AuthResponse>;
    close: () => void;
  }> {
    return new Promise((resolve, reject) => {
      const url = `${this.client.getBaseURL()}/api/realtime?oauth=true`;
      const es = new EventSource(url);
      let settled = false;

      const cleanup = () => {
        es.close();
      };

      es.addEventListener("connected", (e: MessageEvent) => {
        const data = JSON.parse(e.data) as { clientId: string };

        const waitForAuth = (
          popup: Window | null,
        ): Promise<AuthResponse> => {
          return new Promise<AuthResponse>((resolveAuth, rejectAuth) => {
            // 5-minute timeout.
            const timeout = setTimeout(() => {
              cleanup();
              rejectAuth(
                new AYBError(408, "OAuth sign-in timed out", "oauth/timeout"),
              );
            }, 5 * 60 * 1000);

            // Poll for popup closure (every 500ms).
            let popupPoll: ReturnType<typeof setInterval> | undefined;
            if (popup) {
              popupPoll = setInterval(() => {
                if (popup.closed) {
                  clearInterval(popupPoll);
                  clearTimeout(timeout);
                  cleanup();
                  rejectAuth(
                    new AYBError(
                      499,
                      "OAuth popup was closed by the user",
                      "oauth/popup-closed",
                    ),
                  );
                }
              }, 500);
            }

            es.addEventListener("oauth", (oauthEvt: MessageEvent) => {
              clearTimeout(timeout);
              if (popupPoll) clearInterval(popupPoll);
              popup?.close();

              const result = JSON.parse(oauthEvt.data) as {
                token?: string;
                refreshToken?: string;
                user?: User;
                error?: string;
              };

              if (result.error) {
                rejectAuth(
                  new AYBError(401, result.error, "oauth/provider-error"),
                );
                return;
              }

              if (!result.token || !result.refreshToken) {
                rejectAuth(
                  new AYBError(
                    500,
                    "OAuth response missing tokens",
                    "oauth/missing-tokens",
                  ),
                );
                return;
              }

              resolveAuth({
                token: result.token,
                refreshToken: result.refreshToken,
                user: (result.user as User) ?? ({} as User),
              });
            });
          });
        };

        resolve({ clientId: data.clientId, waitForAuth, close: cleanup });
      });

      es.onerror = () => {
        if (!settled) {
          settled = true;
          cleanup();
          reject(
            new AYBError(
              503,
              "Failed to connect to OAuth SSE channel",
              "oauth/sse-failed",
            ),
          );
        }
      };
    });
  }
}

class RecordsClient {
  constructor(private client: AYBClient) {}

  /** List records in a collection with optional filtering, sorting, and pagination. */
  async list<T = Record<string, unknown>>(
    collection: string,
    params?: ListParams,
  ): Promise<ListResponse<T>> {
    const qs = new URLSearchParams();
    if (params?.page != null) qs.set("page", String(params.page));
    if (params?.perPage != null) qs.set("perPage", String(params.perPage));
    if (params?.sort) qs.set("sort", params.sort);
    if (params?.filter) qs.set("filter", params.filter);
    if (params?.search) qs.set("search", params.search);
    if (params?.fields) qs.set("fields", params.fields);
    if (params?.expand) qs.set("expand", params.expand);
    if (params?.skipTotal) qs.set("skipTotal", "true");
    const suffix = qs.toString() ? `?${qs}` : "";
    return this.client.request(`/api/collections/${collection}${suffix}`);
  }

  /** Get a single record by primary key. */
  async get<T = Record<string, unknown>>(
    collection: string,
    id: string,
    params?: GetParams,
  ): Promise<T> {
    const qs = new URLSearchParams();
    if (params?.fields) qs.set("fields", params.fields);
    if (params?.expand) qs.set("expand", params.expand);
    const suffix = qs.toString() ? `?${qs}` : "";
    return this.client.request(`/api/collections/${collection}/${id}${suffix}`);
  }

  /** Create a new record. */
  async create<T = Record<string, unknown>>(
    collection: string,
    data: Record<string, unknown>,
  ): Promise<T> {
    return this.client.request(`/api/collections/${collection}`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(data),
    });
  }

  /** Update an existing record (partial update). */
  async update<T = Record<string, unknown>>(
    collection: string,
    id: string,
    data: Record<string, unknown>,
  ): Promise<T> {
    return this.client.request(`/api/collections/${collection}/${id}`, {
      method: "PATCH",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(data),
    });
  }

  /** Delete a record by primary key. */
  async delete(collection: string, id: string): Promise<void> {
    return this.client.request(`/api/collections/${collection}/${id}`, {
      method: "DELETE",
    });
  }

  /** Execute multiple operations in a single atomic transaction. Max 1000 operations. */
  async batch<T = Record<string, unknown>>(
    collection: string,
    operations: BatchOperation[],
  ): Promise<BatchResult<T>[]> {
    return this.client.request(`/api/collections/${collection}/batch`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ operations }),
    });
  }
}

class StorageClient {
  constructor(private client: AYBClient) {}

  /** Upload a file to a bucket. */
  async upload(
    bucket: string,
    file: Blob | File,
    name?: string,
  ): Promise<StorageObject> {
    const form = new FormData();
    form.append("file", file, name ?? (file instanceof File ? file.name : "upload"));
    // Don't set Content-Type — the browser/runtime will set it with the boundary.
    return this.client.request(`/api/storage/${bucket}`, {
      method: "POST",
      body: form,
    });
  }

  /** Get a download URL for a file. */
  downloadURL(bucket: string, name: string): string {
    return `${this.client.getBaseURL()}/api/storage/${bucket}/${name}`;
  }

  /** Delete a file from a bucket. */
  async delete(bucket: string, name: string): Promise<void> {
    return this.client.request(`/api/storage/${bucket}/${name}`, {
      method: "DELETE",
    });
  }

  /** List files in a bucket. */
  async list(
    bucket: string,
    params?: { prefix?: string; limit?: number; offset?: number },
  ): Promise<{ items: StorageObject[]; totalItems: number }> {
    const qs = new URLSearchParams();
    if (params?.prefix) qs.set("prefix", params.prefix);
    if (params?.limit != null) qs.set("limit", String(params.limit));
    if (params?.offset != null) qs.set("offset", String(params.offset));
    const suffix = qs.toString() ? `?${qs}` : "";
    return this.client.request(`/api/storage/${bucket}${suffix}`);
  }

  /** Get a signed URL for time-limited access to a file. */
  async getSignedURL(
    bucket: string,
    name: string,
    expiresIn?: number,
  ): Promise<{ url: string }> {
    return this.client.request(`/api/storage/${bucket}/${name}/sign`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ expiresIn: expiresIn ?? 3600 }),
    });
  }
}

class RealtimeClient {
  constructor(private client: AYBClient) {}

  /**
   * Subscribe to realtime events for the given tables.
   * Returns an unsubscribe function.
   *
   * @example
   * ```ts
   * const unsub = ayb.realtime.subscribe(["posts", "comments"], (event) => {
   *   console.log(event.action, event.table, event.record);
   * });
   * // Later: unsub();
   * ```
   */
  subscribe(
    tables: string[],
    callback: (event: RealtimeEvent) => void,
  ): () => void {
    const params = new URLSearchParams({ tables: tables.join(",") });
    if (this.client.token) {
      params.set("token", this.client.token);
    }
    const url = `${this.client.getBaseURL()}/api/realtime?${params}`;
    const es = new EventSource(url);

    es.onmessage = (e) => {
      try {
        const event = JSON.parse(e.data) as RealtimeEvent;
        callback(event);
      } catch {
        // Ignore parse errors for heartbeat/ping messages.
      }
    };

    return () => es.close();
  }
}

/**
 * Opens a centered popup window for OAuth. Must be called synchronously
 * in the click handler's call stack to avoid Safari's popup blocker.
 * Initially opens about:blank — the URL is set later after the SSE
 * connection provides a clientId.
 */
function openPopup(): Window | null {
  const width = 1024;
  const height = 768;
  const left = Math.max(0, (screen.width - width) / 2);
  const top = Math.max(0, (screen.height - height) / 2);
  return window.open(
    "about:blank",
    "ayb-oauth",
    `width=${width},height=${height},left=${left},top=${top},scrollbars=yes`,
  );
}
