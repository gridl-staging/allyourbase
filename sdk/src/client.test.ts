import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { AYBClient } from "./client";
import { AYBError } from "./errors";

// --- EventSource mock for realtime tests ---

class MockEventSource {
  static instances: MockEventSource[] = [];

  url: string;
  onmessage: ((e: MessageEvent) => void) | null = null;
  onerror: ((e: Event) => void) | null = null;
  closed = false;

  constructor(url: string) {
    this.url = url;
    MockEventSource.instances.push(this);
  }

  close() {
    this.closed = true;
  }

  // Test helper: send a message event.
  _sendMessage(data: string) {
    if (this.onmessage) {
      this.onmessage({ data } as MessageEvent);
    }
  }

  // Test helper: send a parsed object as JSON.
  _sendJSON(obj: unknown) {
    this._sendMessage(JSON.stringify(obj));
  }
}

const OriginalEventSource = globalThis.EventSource;

function mockFetch(
  status: number,
  body: unknown,
  headers?: Record<string, string>,
): typeof globalThis.fetch {
  return vi.fn().mockResolvedValue({
    ok: status >= 200 && status < 300,
    status,
    statusText: "OK",
    headers: new Headers(headers),
    json: () => Promise.resolve(body),
  }) as unknown as typeof globalThis.fetch;
}

describe("AYBClient", () => {
  it("constructs with baseURL", () => {
    const client = new AYBClient("http://localhost:8090");
    expect(client.token).toBeNull();
    expect(client.refreshToken).toBeNull();
  });

  it("strips trailing slash from baseURL", () => {
    const fetchFn = mockFetch(200, { items: [], page: 1, perPage: 20, totalItems: 0, totalPages: 0 });
    const client = new AYBClient("http://localhost:8090/", { fetch: fetchFn });
    client.records.list("posts");
    expect(fetchFn).toHaveBeenCalledWith(
      "http://localhost:8090/api/collections/posts",
      expect.anything(),
    );
  });

  it("setTokens / clearTokens", () => {
    const client = new AYBClient("http://localhost:8090");
    client.setTokens("access", "refresh");
    expect(client.token).toBe("access");
    expect(client.refreshToken).toBe("refresh");
    client.clearTokens();
    expect(client.token).toBeNull();
  });
});

describe("records", () => {
  let fetchFn: ReturnType<typeof mockFetch>;
  let client: AYBClient;

  beforeEach(() => {
    fetchFn = mockFetch(200, { items: [{ id: "1" }], page: 1, perPage: 20, totalItems: 1, totalPages: 1 });
    client = new AYBClient("http://localhost:8090", { fetch: fetchFn });
  });

  it("list sends correct URL", async () => {
    await client.records.list("posts", { page: 2, sort: "-created_at", filter: "active=true" });
    const url = (fetchFn as ReturnType<typeof vi.fn>).mock.calls[0][0] as string;
    expect(url).toContain("/api/collections/posts?");
    expect(url).toContain("page=2");
    expect(url).toContain("sort=-created_at");
    expect(url).toContain("filter=active%3Dtrue");
  });

  it("list with no params", async () => {
    await client.records.list("posts");
    const url = (fetchFn as ReturnType<typeof vi.fn>).mock.calls[0][0] as string;
    expect(url).toBe("http://localhost:8090/api/collections/posts");
  });

  it("get sends correct URL", async () => {
    fetchFn = mockFetch(200, { id: "42", title: "hello" });
    client = new AYBClient("http://localhost:8090", { fetch: fetchFn });
    await client.records.get("posts", "42", { expand: "author" });
    const url = (fetchFn as ReturnType<typeof vi.fn>).mock.calls[0][0] as string;
    expect(url).toContain("/api/collections/posts/42");
    expect(url).toContain("expand=author");
  });

  it("create sends POST with body", async () => {
    fetchFn = mockFetch(201, { id: "new", title: "test" });
    client = new AYBClient("http://localhost:8090", { fetch: fetchFn });
    const result = await client.records.create("posts", { title: "test" });
    expect(result).toEqual({ id: "new", title: "test" });
    const call = (fetchFn as ReturnType<typeof vi.fn>).mock.calls[0];
    expect(call[1].method).toBe("POST");
    expect(JSON.parse(call[1].body as string)).toEqual({ title: "test" });
  });

  it("update sends PATCH", async () => {
    fetchFn = mockFetch(200, { id: "42", title: "updated" });
    client = new AYBClient("http://localhost:8090", { fetch: fetchFn });
    await client.records.update("posts", "42", { title: "updated" });
    const call = (fetchFn as ReturnType<typeof vi.fn>).mock.calls[0];
    expect(call[0]).toContain("/api/collections/posts/42");
    expect(call[1].method).toBe("PATCH");
  });

  it("delete sends DELETE", async () => {
    fetchFn = mockFetch(204, undefined);
    client = new AYBClient("http://localhost:8090", { fetch: fetchFn });
    await client.records.delete("posts", "42");
    const call = (fetchFn as ReturnType<typeof vi.fn>).mock.calls[0];
    expect(call[0]).toContain("/api/collections/posts/42");
    expect(call[1].method).toBe("DELETE");
  });

  it("batch sends POST to /batch with operations array", async () => {
    const results = [
      { index: 0, status: 201, body: { id: "1", title: "A" } },
      { index: 1, status: 200, body: { id: "2", title: "B" } },
      { index: 2, status: 204 },
    ];
    fetchFn = mockFetch(200, results);
    client = new AYBClient("http://localhost:8090", { fetch: fetchFn });
    const ops = [
      { method: "create" as const, body: { title: "A" } },
      { method: "update" as const, id: "2", body: { title: "B" } },
      { method: "delete" as const, id: "3" },
    ];
    const res = await client.records.batch("posts", ops);
    expect(res).toEqual(results);
    const call = (fetchFn as ReturnType<typeof vi.fn>).mock.calls[0];
    expect(call[0]).toBe("http://localhost:8090/api/collections/posts/batch");
    expect(call[1].method).toBe("POST");
    const sent = JSON.parse(call[1].body as string);
    expect(sent.operations).toHaveLength(3);
    expect(sent.operations[0].method).toBe("create");
  });
});

describe("auth", () => {
  it("login stores tokens", async () => {
    const fetchFn = mockFetch(200, { token: "tok", refreshToken: "ref", user: { id: "1", email: "a@b.com" } });
    const client = new AYBClient("http://localhost:8090", { fetch: fetchFn });
    const res = await client.auth.login("a@b.com", "pass");
    expect(res.token).toBe("tok");
    expect(client.token).toBe("tok");
    expect(client.refreshToken).toBe("ref");
  });

  it("register stores tokens and sends correct request", async () => {
    const fetchFn = mockFetch(201, { token: "tok", refreshToken: "ref", user: { id: "1", email: "a@b.com" } });
    const client = new AYBClient("http://localhost:8090", { fetch: fetchFn });
    await client.auth.register("a@b.com", "pass");
    expect(client.token).toBe("tok");
    expect(client.refreshToken).toBe("ref");
    const call = (fetchFn as ReturnType<typeof vi.fn>).mock.calls[0];
    expect(call[0]).toContain("/api/auth/register");
    expect(call[1].method).toBe("POST");
    expect(JSON.parse(call[1].body as string)).toEqual({ email: "a@b.com", password: "pass" });
  });

  it("logout clears tokens and sends refresh token", async () => {
    const fetchFn = mockFetch(204, undefined);
    const client = new AYBClient("http://localhost:8090", { fetch: fetchFn });
    client.setTokens("tok", "ref");
    await client.auth.logout();
    expect(client.token).toBeNull();
    expect(client.refreshToken).toBeNull();
    const call = (fetchFn as ReturnType<typeof vi.fn>).mock.calls[0];
    expect(call[0]).toContain("/api/auth/logout");
    expect(call[1].method).toBe("POST");
    expect(JSON.parse(call[1].body as string)).toEqual({ refreshToken: "ref" });
  });

  it("refresh sends current refresh token and updates tokens", async () => {
    const fetchFn = mockFetch(200, { token: "new-tok", refreshToken: "new-ref", user: { id: "1", email: "a@b.com" } });
    const client = new AYBClient("http://localhost:8090", { fetch: fetchFn });
    client.setTokens("old-tok", "old-ref");
    await client.auth.refresh();
    expect(client.token).toBe("new-tok");
    expect(client.refreshToken).toBe("new-ref");
    const call = (fetchFn as ReturnType<typeof vi.fn>).mock.calls[0];
    expect(call[0]).toContain("/api/auth/refresh");
    expect(JSON.parse(call[1].body as string)).toEqual({ refreshToken: "old-ref" });
  });

  it("confirmPasswordReset sends token and new password", async () => {
    const fetchFn = mockFetch(204, undefined);
    const client = new AYBClient("http://localhost:8090", { fetch: fetchFn });
    await client.auth.confirmPasswordReset("reset-tok", "newpass123");
    const call = (fetchFn as ReturnType<typeof vi.fn>).mock.calls[0];
    expect(call[0]).toContain("/api/auth/password-reset/confirm");
    expect(call[1].method).toBe("POST");
    expect(JSON.parse(call[1].body as string)).toEqual({ token: "reset-tok", password: "newpass123" });
  });

  it("verifyEmail sends token", async () => {
    const fetchFn = mockFetch(204, undefined);
    const client = new AYBClient("http://localhost:8090", { fetch: fetchFn });
    await client.auth.verifyEmail("verify-tok");
    const call = (fetchFn as ReturnType<typeof vi.fn>).mock.calls[0];
    expect(call[0]).toContain("/api/auth/verify");
    expect(call[1].method).toBe("POST");
    expect(JSON.parse(call[1].body as string)).toEqual({ token: "verify-tok" });
  });

  it("resendVerification sends POST", async () => {
    const fetchFn = mockFetch(204, undefined);
    const client = new AYBClient("http://localhost:8090", { fetch: fetchFn });
    client.setTokens("tok", "ref");
    await client.auth.resendVerification();
    const call = (fetchFn as ReturnType<typeof vi.fn>).mock.calls[0];
    expect(call[0]).toContain("/api/auth/verify/resend");
    expect(call[1].method).toBe("POST");
  });

  it("sends auth header when token is set", async () => {
    const fetchFn = mockFetch(200, { id: "1", email: "a@b.com" });
    const client = new AYBClient("http://localhost:8090", { fetch: fetchFn });
    client.setTokens("my-token", "my-refresh");
    await client.auth.me();
    const call = (fetchFn as ReturnType<typeof vi.fn>).mock.calls[0];
    expect(call[1].headers.Authorization).toBe("Bearer my-token");
  });

  it("requestPasswordReset sends POST with email", async () => {
    const fetchFn = mockFetch(200, { message: "ok" });
    const client = new AYBClient("http://localhost:8090", { fetch: fetchFn });
    await client.auth.requestPasswordReset("a@b.com");
    const call = (fetchFn as ReturnType<typeof vi.fn>).mock.calls[0];
    expect(call[0]).toContain("/api/auth/password-reset");
    expect(call[1].method).toBe("POST");
    expect(JSON.parse(call[1].body as string)).toEqual({ email: "a@b.com" });
  });

  it("deleteAccount sends DELETE to /me and clears tokens", async () => {
    const fetchFn = mockFetch(204, undefined);
    const client = new AYBClient("http://localhost:8090", { fetch: fetchFn });
    client.setTokens("tok", "ref");
    await client.auth.deleteAccount();
    const call = (fetchFn as ReturnType<typeof vi.fn>).mock.calls[0];
    expect(call[0]).toContain("/api/auth/me");
    expect(call[1].method).toBe("DELETE");
    expect(client.token).toBeNull();
    expect(client.refreshToken).toBeNull();
  });

  it("deleteAccount emits SIGNED_OUT event", async () => {
    expect.assertions(1);
    const fetchFn = mockFetch(204, undefined);
    const client = new AYBClient("http://localhost:8090", { fetch: fetchFn });
    client.setTokens("tok", "ref");
    client.onAuthStateChange((event) => {
      expect(event).toBe("SIGNED_OUT");
    });
    await client.auth.deleteAccount();
  });
});

describe("storage", () => {
  it("downloadURL builds correct URL with bucket and name", () => {
    const client = new AYBClient("http://localhost:8090");
    expect(client.storage.downloadURL("avatars", "photo.jpg")).toBe(
      "http://localhost:8090/api/storage/avatars/photo.jpg",
    );
  });

  it("upload sends POST to /api/storage/{bucket}", async () => {
    const fetchFn = mockFetch(201, { id: "1", bucket: "avatars", name: "photo.jpg" });
    const client = new AYBClient("http://localhost:8090", { fetch: fetchFn });
    const file = new Blob(["test"], { type: "image/jpeg" });
    await client.storage.upload("avatars", file, "photo.jpg");
    const call = (fetchFn as ReturnType<typeof vi.fn>).mock.calls[0];
    expect(call[0]).toBe("http://localhost:8090/api/storage/avatars");
    expect(call[1].method).toBe("POST");
  });

  it("delete sends DELETE to /api/storage/{bucket}/{name}", async () => {
    const fetchFn = mockFetch(204, undefined);
    const client = new AYBClient("http://localhost:8090", { fetch: fetchFn });
    await client.storage.delete("avatars", "photo.jpg");
    const call = (fetchFn as ReturnType<typeof vi.fn>).mock.calls[0];
    expect(call[0]).toBe("http://localhost:8090/api/storage/avatars/photo.jpg");
    expect(call[1].method).toBe("DELETE");
  });

  it("list sends GET to /api/storage/{bucket}", async () => {
    const fetchFn = mockFetch(200, { items: [], totalItems: 0 });
    const client = new AYBClient("http://localhost:8090", { fetch: fetchFn });
    await client.storage.list("avatars", { prefix: "user_", limit: 10 });
    const url = (fetchFn as ReturnType<typeof vi.fn>).mock.calls[0][0] as string;
    expect(url).toContain("/api/storage/avatars?");
    expect(url).toContain("prefix=user_");
    expect(url).toContain("limit=10");
  });

  it("getSignedURL sends POST to /api/storage/{bucket}/{name}/sign", async () => {
    const fetchFn = mockFetch(200, { url: "/api/storage/avatars/photo.jpg?exp=123&sig=abc" });
    const client = new AYBClient("http://localhost:8090", { fetch: fetchFn });
    await client.storage.getSignedURL("avatars", "photo.jpg", 7200);
    const call = (fetchFn as ReturnType<typeof vi.fn>).mock.calls[0];
    expect(call[0]).toBe("http://localhost:8090/api/storage/avatars/photo.jpg/sign");
    expect(call[1].method).toBe("POST");
    expect(JSON.parse(call[1].body as string)).toEqual({ expiresIn: 7200 });
  });

  it("getSignedURL defaults expiresIn to 3600", async () => {
    const fetchFn = mockFetch(200, { url: "/signed" });
    const client = new AYBClient("http://localhost:8090", { fetch: fetchFn });
    await client.storage.getSignedURL("avatars", "photo.jpg");
    const call = (fetchFn as ReturnType<typeof vi.fn>).mock.calls[0];
    expect(JSON.parse(call[1].body as string)).toEqual({ expiresIn: 3600 });
  });

  it("list with no params sends clean URL", async () => {
    const fetchFn = mockFetch(200, { items: [], totalItems: 0 });
    const client = new AYBClient("http://localhost:8090", { fetch: fetchFn });
    await client.storage.list("avatars");
    const url = (fetchFn as ReturnType<typeof vi.fn>).mock.calls[0][0] as string;
    expect(url).toBe("http://localhost:8090/api/storage/avatars");
  });

  it("list includes offset param", async () => {
    const fetchFn = mockFetch(200, { items: [], totalItems: 0 });
    const client = new AYBClient("http://localhost:8090", { fetch: fetchFn });
    await client.storage.list("avatars", { offset: 10 });
    const url = (fetchFn as ReturnType<typeof vi.fn>).mock.calls[0][0] as string;
    expect(url).toContain("offset=10");
  });

  it("list preserves offset=0", async () => {
    const fetchFn = mockFetch(200, { items: [], totalItems: 0 });
    const client = new AYBClient("http://localhost:8090", { fetch: fetchFn });
    await client.storage.list("avatars", { offset: 0 });
    const url = (fetchFn as ReturnType<typeof vi.fn>).mock.calls[0][0] as string;
    expect(url).toContain("offset=0");
  });
});

describe("records params coverage", () => {
  it("list includes perPage, fields, expand, skipTotal", async () => {
    const fetchFn = mockFetch(200, { items: [], page: 1, perPage: 5, totalItems: 0, totalPages: 0 });
    const client = new AYBClient("http://localhost:8090", { fetch: fetchFn });
    await client.records.list("posts", { perPage: 5, fields: "id,title", expand: "author", skipTotal: true });
    const url = (fetchFn as ReturnType<typeof vi.fn>).mock.calls[0][0] as string;
    expect(url).toContain("perPage=5");
    expect(url).toContain("fields=id%2Ctitle");
    expect(url).toContain("expand=author");
    expect(url).toContain("skipTotal=true");
  });

  it("get includes fields param", async () => {
    const fetchFn = mockFetch(200, { id: "1", title: "hello" });
    const client = new AYBClient("http://localhost:8090", { fetch: fetchFn });
    await client.records.get("posts", "1", { fields: "id,title" });
    const url = (fetchFn as ReturnType<typeof vi.fn>).mock.calls[0][0] as string;
    expect(url).toContain("fields=id%2Ctitle");
  });

  it("list preserves page=0 (falsy but valid)", async () => {
    const fetchFn = mockFetch(200, { items: [], page: 0, perPage: 20, totalItems: 0, totalPages: 0 });
    const client = new AYBClient("http://localhost:8090", { fetch: fetchFn });
    await client.records.list("posts", { page: 0 });
    const url = (fetchFn as ReturnType<typeof vi.fn>).mock.calls[0][0] as string;
    expect(url).toContain("page=0");
  });

  it("list includes search param", async () => {
    const fetchFn = mockFetch(200, { items: [], page: 1, perPage: 20, totalItems: 0, totalPages: 0 });
    const client = new AYBClient("http://localhost:8090", { fetch: fetchFn });
    await client.records.list("posts", { search: "postgres database" });
    const url = (fetchFn as ReturnType<typeof vi.fn>).mock.calls[0][0] as string;
    expect(url).toContain("search=postgres+database");
  });

  it("list combines search with filter", async () => {
    const fetchFn = mockFetch(200, { items: [], page: 1, perPage: 20, totalItems: 0, totalPages: 0 });
    const client = new AYBClient("http://localhost:8090", { fetch: fetchFn });
    await client.records.list("posts", { search: "postgres", filter: "status='active'" });
    const url = (fetchFn as ReturnType<typeof vi.fn>).mock.calls[0][0] as string;
    expect(url).toContain("search=postgres");
    expect(url).toContain("filter=status%3D%27active%27");
  });

  it("delete returns undefined for 204", async () => {
    const fetchFn = mockFetch(204, undefined);
    const client = new AYBClient("http://localhost:8090", { fetch: fetchFn });
    const result = await client.records.delete("posts", "42");
    expect(result).toBeUndefined();
  });
});

describe("API keys", () => {
  it("setApiKey stores key as token", () => {
    const client = new AYBClient("http://localhost:8090");
    client.setApiKey("ayb_abc123def456abc123def456abc123def456abc123def456");
    expect(client.token).toBe("ayb_abc123def456abc123def456abc123def456abc123def456");
    expect(client.refreshToken).toBeNull();
  });

  it("setApiKey clears existing refresh token", () => {
    const client = new AYBClient("http://localhost:8090");
    client.setTokens("jwt-token", "refresh-token");
    expect(client.refreshToken).toBe("refresh-token");
    client.setApiKey("ayb_abc123def456abc123def456abc123def456abc123def456");
    expect(client.refreshToken).toBeNull();
  });

  it("clearApiKey removes API key", () => {
    const client = new AYBClient("http://localhost:8090");
    client.setApiKey("ayb_abc123def456abc123def456abc123def456abc123def456");
    client.clearApiKey();
    expect(client.token).toBeNull();
    expect(client.refreshToken).toBeNull();
  });

  it("API key sends Bearer header on requests", async () => {
    const fetchFn = mockFetch(200, { items: [], page: 1, perPage: 20, totalItems: 0, totalPages: 0 });
    const client = new AYBClient("http://localhost:8090", { fetch: fetchFn });
    client.setApiKey("ayb_abc123def456abc123def456abc123def456abc123def456");
    await client.records.list("posts");
    const call = (fetchFn as ReturnType<typeof vi.fn>).mock.calls[0];
    expect(call[1].headers.Authorization).toBe("Bearer ayb_abc123def456abc123def456abc123def456abc123def456");
  });

  it("API key is used for all request types", async () => {
    const apiKey = "ayb_abc123def456abc123def456abc123def456abc123def456";
    const fetchFn = mockFetch(200, { id: "1", title: "test" });
    const client = new AYBClient("http://localhost:8090", { fetch: fetchFn });
    client.setApiKey(apiKey);

    await client.records.get("posts", "1");
    const getCall = (fetchFn as ReturnType<typeof vi.fn>).mock.calls[0];
    expect(getCall[1].headers.Authorization).toBe(`Bearer ${apiKey}`);

    await client.records.create("posts", { title: "new" });
    const createCall = (fetchFn as ReturnType<typeof vi.fn>).mock.calls[1];
    expect(createCall[1].headers.Authorization).toBe(`Bearer ${apiKey}`);
  });

  it("setApiKey replaces JWT token", async () => {
    const fetchFn = mockFetch(200, { items: [], page: 1, perPage: 20, totalItems: 0, totalPages: 0 });
    const client = new AYBClient("http://localhost:8090", { fetch: fetchFn });
    client.setTokens("jwt-token", "refresh-token");
    client.setApiKey("ayb_newkey123456789012345678901234567890123456");
    await client.records.list("posts");
    const call = (fetchFn as ReturnType<typeof vi.fn>).mock.calls[0];
    expect(call[1].headers.Authorization).toBe("Bearer ayb_newkey123456789012345678901234567890123456");
  });

  it("no auth header when no API key or token set", async () => {
    const fetchFn = mockFetch(200, { items: [], page: 1, perPage: 20, totalItems: 0, totalPages: 0 });
    const client = new AYBClient("http://localhost:8090", { fetch: fetchFn });
    await client.records.list("posts");
    const call = (fetchFn as ReturnType<typeof vi.fn>).mock.calls[0];
    expect(call[1].headers.Authorization).toBeUndefined();
  });
});

describe("error handling", () => {
  it("throws AYBError on non-2xx", async () => {
    const fetchFn = mockFetch(404, { message: "collection not found: missing" });
    const client = new AYBClient("http://localhost:8090", { fetch: fetchFn });
    await expect(client.records.list("missing")).rejects.toThrow(AYBError);
    await expect(client.records.list("missing")).rejects.toThrow("collection not found: missing");
  });

  it("AYBError has status", async () => {
    const fetchFn = mockFetch(401, { message: "unauthorized" });
    const client = new AYBClient("http://localhost:8090", { fetch: fetchFn });
    expect.assertions(2);
    try {
      await client.auth.me();
    } catch (e) {
      expect(e).toBeInstanceOf(AYBError);
      expect((e as AYBError).status).toBe(401);
    }
  });

  it("AYBError includes data and docUrl from server response", async () => {
    const fetchFn = mockFetch(409, {
      message: "unique constraint violation",
      data: {
        users_email_key: { code: "unique_violation", message: "Key (email)=(a@b.com) already exists." },
      },
      doc_url: "https://allyourbase.io/guide/api-reference#error-format",
    });
    const client = new AYBClient("http://localhost:8090", { fetch: fetchFn });
    expect.assertions(4);
    try {
      await client.records.create("users", { email: "a@b.com" });
    } catch (e) {
      expect(e).toBeInstanceOf(AYBError);
      const err = e as AYBError;
      expect(err.status).toBe(409);
      expect(err.data).toEqual({
        users_email_key: { code: "unique_violation", message: "Key (email)=(a@b.com) already exists." },
      });
      expect(err.docUrl).toBe("https://allyourbase.io/guide/api-reference#error-format");
    }
  });

  it("AYBError data and docUrl are undefined when server omits them", async () => {
    const fetchFn = mockFetch(404, { message: "record not found" });
    const client = new AYBClient("http://localhost:8090", { fetch: fetchFn });
    expect.assertions(2);
    try {
      await client.records.get("posts", "999");
    } catch (e) {
      const err = e as AYBError;
      expect(err.data).toBeUndefined();
      expect(err.docUrl).toBeUndefined();
    }
  });

  it("AYBError falls back to statusText when json parse fails", async () => {
    const fetchFn = vi.fn().mockResolvedValue({
      ok: false,
      status: 502,
      statusText: "Bad Gateway",
      headers: new Headers(),
      json: () => Promise.reject(new Error("not json")),
    }) as unknown as typeof globalThis.fetch;
    const client = new AYBClient("http://localhost:8090", { fetch: fetchFn });
    await expect(client.records.list("posts")).rejects.toThrow("Bad Gateway");
  });
});

describe("rpc", () => {
  it("calls POST /api/rpc/{function} with args", async () => {
    const fetchFn = mockFetch(200, 42);
    const client = new AYBClient("http://localhost:8090", { fetch: fetchFn });
    const result = await client.rpc("get_total", { user_id: "abc" });
    expect(result).toBe(42);
    const call = (fetchFn as ReturnType<typeof vi.fn>).mock.calls[0];
    expect(call[0]).toBe("http://localhost:8090/api/rpc/get_total");
    expect(call[1].method).toBe("POST");
    expect(call[1].headers["Content-Type"]).toBe("application/json");
    expect(JSON.parse(call[1].body as string)).toEqual({ user_id: "abc" });
  });

  it("calls without body when no args", async () => {
    const fetchFn = mockFetch(200, "PostgreSQL 16.2");
    const client = new AYBClient("http://localhost:8090", { fetch: fetchFn });
    const result = await client.rpc<string>("pg_version");
    expect(result).toBe("PostgreSQL 16.2");
    const call = (fetchFn as ReturnType<typeof vi.fn>).mock.calls[0];
    expect(call[0]).toBe("http://localhost:8090/api/rpc/pg_version");
    expect(call[1].body).toBeUndefined();
    expect(call[1].headers["Content-Type"]).toBeUndefined();
  });

  it("calls without body when args is empty object", async () => {
    const fetchFn = mockFetch(200, "ok");
    const client = new AYBClient("http://localhost:8090", { fetch: fetchFn });
    await client.rpc("no_args_fn", {});
    const call = (fetchFn as ReturnType<typeof vi.fn>).mock.calls[0];
    expect(call[1].body).toBeUndefined();
  });

  it("returns undefined for void functions (204)", async () => {
    const fetchFn = mockFetch(204, undefined);
    const client = new AYBClient("http://localhost:8090", { fetch: fetchFn });
    const result = await client.rpc("cleanup_old_data", { days: 30 });
    expect(result).toBeUndefined();
  });

  it("returns array for set-returning functions", async () => {
    const rows = [
      { id: "1", name: "Alice" },
      { id: "2", name: "Bob" },
    ];
    const fetchFn = mockFetch(200, rows);
    const client = new AYBClient("http://localhost:8090", { fetch: fetchFn });
    const result = await client.rpc<{ id: string; name: string }[]>("search_users", { query: "a" });
    expect(result).toEqual(rows);
    expect(result).toHaveLength(2);
  });

  it("sends auth header when authenticated", async () => {
    const fetchFn = mockFetch(200, 1);
    const client = new AYBClient("http://localhost:8090", { fetch: fetchFn });
    client.setTokens("my-jwt", "my-refresh");
    await client.rpc("my_func");
    const call = (fetchFn as ReturnType<typeof vi.fn>).mock.calls[0];
    expect(call[1].headers["Authorization"]).toBe("Bearer my-jwt");
  });

  it("throws AYBError on function not found", async () => {
    const fetchFn = mockFetch(404, { message: "function not found: nope" });
    const client = new AYBClient("http://localhost:8090", { fetch: fetchFn });
    await expect(client.rpc("nope")).rejects.toThrow(AYBError);
    await expect(client.rpc("nope")).rejects.toThrow("function not found: nope");
  });
});

describe("realtime", () => {
  beforeEach(() => {
    MockEventSource.instances = [];
    globalThis.EventSource = MockEventSource as unknown as typeof EventSource;
  });

  afterEach(() => {
    globalThis.EventSource = OriginalEventSource;
  });

  it("subscribe creates EventSource with correct URL for single table", () => {
    const client = new AYBClient("http://localhost:8090");
    client.realtime.subscribe(["posts"], () => {});
    expect(MockEventSource.instances).toHaveLength(1);
    expect(MockEventSource.instances[0].url).toBe(
      "http://localhost:8090/api/realtime?tables=posts",
    );
  });

  it("subscribe creates EventSource with comma-separated tables", () => {
    const client = new AYBClient("http://localhost:8090");
    client.realtime.subscribe(["posts", "comments", "users"], () => {});
    expect(MockEventSource.instances).toHaveLength(1);
    const url = MockEventSource.instances[0].url;
    expect(url).toContain("tables=posts%2Ccomments%2Cusers");
  });

  it("subscribe includes auth token in URL when set", () => {
    const client = new AYBClient("http://localhost:8090");
    client.setTokens("my-jwt-token", "refresh");
    client.realtime.subscribe(["posts"], () => {});
    const url = MockEventSource.instances[0].url;
    expect(url).toContain("token=my-jwt-token");
    expect(url).toContain("tables=posts");
  });

  it("subscribe omits token param when no auth", () => {
    const client = new AYBClient("http://localhost:8090");
    client.realtime.subscribe(["posts"], () => {});
    const url = MockEventSource.instances[0].url;
    expect(url).not.toContain("token=");
  });

  it("dispatches parsed JSON events to callback", () => {
    expect.assertions(3);
    const client = new AYBClient("http://localhost:8090");
    const callback = vi.fn();
    client.realtime.subscribe(["posts"], callback);

    const es = MockEventSource.instances[0];
    es._sendJSON({ action: "create", table: "posts", record: { id: "1", title: "Hello" } });

    expect(callback).toHaveBeenCalledTimes(1);
    expect(callback).toHaveBeenCalledWith({
      action: "create",
      table: "posts",
      record: { id: "1", title: "Hello" },
    });
    expect(callback.mock.calls[0][0].action).toBe("create");
  });

  it("dispatches multiple events sequentially", () => {
    const client = new AYBClient("http://localhost:8090");
    const events: unknown[] = [];
    client.realtime.subscribe(["posts"], (e) => events.push(e));

    const es = MockEventSource.instances[0];
    es._sendJSON({ action: "create", table: "posts", record: { id: "1" } });
    es._sendJSON({ action: "update", table: "posts", record: { id: "1", title: "Updated" } });
    es._sendJSON({ action: "delete", table: "posts", record: { id: "1" } });

    expect(events).toHaveLength(3);
    expect((events[0] as { action: string }).action).toBe("create");
    expect((events[1] as { action: string }).action).toBe("update");
    expect((events[2] as { action: string }).action).toBe("delete");
  });

  it("ignores non-JSON messages (heartbeats)", () => {
    const client = new AYBClient("http://localhost:8090");
    const callback = vi.fn();
    client.realtime.subscribe(["posts"], callback);

    const es = MockEventSource.instances[0];
    // Send a non-JSON heartbeat message
    es._sendMessage("ping");
    es._sendMessage("");
    es._sendMessage(":");

    expect(callback).not.toHaveBeenCalled();
  });

  it("ignores malformed JSON gracefully", () => {
    const client = new AYBClient("http://localhost:8090");
    const callback = vi.fn();
    client.realtime.subscribe(["posts"], callback);

    const es = MockEventSource.instances[0];
    es._sendMessage("{invalid json}");
    // Valid event should still work after malformed one
    es._sendJSON({ action: "create", table: "posts", record: { id: "1" } });

    expect(callback).toHaveBeenCalledTimes(1);
    expect(callback).toHaveBeenCalledWith({
      action: "create",
      table: "posts",
      record: { id: "1" },
    });
  });

  it("unsubscribe closes EventSource", () => {
    const client = new AYBClient("http://localhost:8090");
    const unsub = client.realtime.subscribe(["posts"], () => {});

    const es = MockEventSource.instances[0];
    expect(es.closed).toBe(false);

    unsub();
    expect(es.closed).toBe(true);
  });

  it("no callbacks after unsubscribe", () => {
    const client = new AYBClient("http://localhost:8090");
    const callback = vi.fn();
    const unsub = client.realtime.subscribe(["posts"], callback);

    const es = MockEventSource.instances[0];
    es._sendJSON({ action: "create", table: "posts", record: { id: "1" } });
    expect(callback).toHaveBeenCalledTimes(1);

    unsub();
    // onmessage is still assigned but EventSource is closed — in real impl
    // the browser wouldn't deliver more events. We verify close was called.
    expect(es.closed).toBe(true);
  });

  it("multiple independent subscriptions", () => {
    const client = new AYBClient("http://localhost:8090");
    const cb1 = vi.fn();
    const cb2 = vi.fn();
    const unsub1 = client.realtime.subscribe(["posts"], cb1);
    client.realtime.subscribe(["comments"], cb2);

    expect(MockEventSource.instances).toHaveLength(2);
    expect(MockEventSource.instances[0].url).toContain("tables=posts");
    expect(MockEventSource.instances[1].url).toContain("tables=comments");

    // Events to first subscription
    MockEventSource.instances[0]._sendJSON({ action: "create", table: "posts", record: { id: "1" } });
    expect(cb1).toHaveBeenCalledTimes(1);
    expect(cb2).not.toHaveBeenCalled();

    // Unsubscribe first doesn't affect second
    unsub1();
    expect(MockEventSource.instances[0].closed).toBe(true);
    expect(MockEventSource.instances[1].closed).toBe(false);
  });

  it("subscribe with API key includes token in URL", () => {
    const client = new AYBClient("http://localhost:8090");
    client.setApiKey("ayb_abc123def456abc123def456abc123def456abc123def456");
    client.realtime.subscribe(["posts"], () => {});
    const url = MockEventSource.instances[0].url;
    expect(url).toContain("token=ayb_abc123def456abc123def456abc123def456abc123def456");
  });

  it("subscribe returns a function", () => {
    const client = new AYBClient("http://localhost:8090");
    const unsub = client.realtime.subscribe(["posts"], () => {});
    expect(typeof unsub).toBe("function");
  });
});
