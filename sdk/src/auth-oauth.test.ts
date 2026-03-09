// @vitest-environment jsdom
import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { AYBClient } from "./client";
import { AYBError } from "./errors";

// --- EventSource mock ---

type ESListener = (e: MessageEvent) => void;

class MockEventSource {
  static instances: MockEventSource[] = [];

  url: string;
  listeners: Record<string, ESListener[]> = {};
  onerror: ((e: Event) => void) | null = null;
  closed = false;

  constructor(url: string) {
    this.url = url;
    MockEventSource.instances.push(this);
  }

  addEventListener(event: string, fn: ESListener) {
    if (!this.listeners[event]) this.listeners[event] = [];
    this.listeners[event].push(fn);
  }

  close() {
    this.closed = true;
  }

  // Test helper: emit an event.
  emit(event: string, data: unknown) {
    const msg = { data: JSON.stringify(data) } as MessageEvent;
    for (const fn of this.listeners[event] ?? []) {
      fn(msg);
    }
  }

  // Test helper: trigger error.
  triggerError() {
    if (this.onerror) this.onerror(new Event("error"));
  }
}

function mockFetch(
  status: number,
  body: unknown,
): typeof globalThis.fetch {
  return vi.fn().mockResolvedValue({
    ok: status >= 200 && status < 300,
    status,
    statusText: "OK",
    headers: new Headers(),
    json: () => Promise.resolve(body),
  }) as unknown as typeof globalThis.fetch;
}

describe("signInWithOAuth", () => {
  let originalEventSource: typeof EventSource;
  let originalWindowOpen: typeof window.open;

  beforeEach(() => {
    MockEventSource.instances = [];
    originalEventSource = globalThis.EventSource;
    (globalThis as unknown as Record<string, unknown>).EventSource =
      MockEventSource;
    originalWindowOpen = window.open;
  });

  afterEach(() => {
    (globalThis as unknown as Record<string, unknown>).EventSource =
      originalEventSource;
    window.open = originalWindowOpen;
  });

  it("opens popup when no urlCallback is provided", async () => {
    const openSpy = vi.fn().mockReturnValue({
      closed: false,
      close: vi.fn(),
      location: { href: "" },
    });
    window.open = openSpy as unknown as typeof window.open;

    const fetchFn = mockFetch(200, {});
    const client = new AYBClient("http://localhost:8090", { fetch: fetchFn });

    const promise = client.auth.signInWithOAuth("google");

    // Popup should have been opened immediately.
    expect(openSpy).toHaveBeenCalledWith(
      "about:blank",
      "ayb-oauth",
      expect.stringContaining("width="),
    );

    // Simulate SSE connected event.
    const es = MockEventSource.instances[0];
    expect(es).toBeDefined();
    expect(es.url).toBe("http://localhost:8090/api/realtime?oauth=true");
    es.emit("connected", { clientId: "c1" });

    // Popup location should be set to the OAuth URL.
    await vi.waitFor(() => {
      const popup = openSpy.mock.results[0].value;
      expect(popup.location.href).toContain(
        "/api/auth/oauth/google?state=c1",
      );
    });

    // Simulate OAuth result via SSE.
    es.emit("oauth", {
      token: "access-tok",
      refreshToken: "refresh-tok",
      user: { id: "u1", email: "user@example.com" },
    });

    const result = await promise;
    expect(result.token).toBe("access-tok");
    expect(result.refreshToken).toBe("refresh-tok");
    expect(result.user.email).toBe("user@example.com");

    // Tokens should be stored on the client.
    expect(client.token).toBe("access-tok");
    expect(client.refreshToken).toBe("refresh-tok");

    // EventSource should be closed.
    expect(es.closed).toBe(true);
  });

  it("calls urlCallback instead of opening popup when provided", async () => {
    const urlCallback = vi.fn();

    const fetchFn = mockFetch(200, {});
    const client = new AYBClient("http://localhost:8090", { fetch: fetchFn });

    const openSpy = vi.fn();
    window.open = openSpy as unknown as typeof window.open;

    const promise = client.auth.signInWithOAuth("google", { urlCallback });

    // Popup should NOT have been opened.
    expect(openSpy).not.toHaveBeenCalled();

    // SSE connected.
    const es = MockEventSource.instances[0];
    es.emit("connected", { clientId: "c2" });

    // urlCallback should be called with the OAuth URL.
    await vi.waitFor(() => {
      expect(urlCallback).toHaveBeenCalledWith(
        "http://localhost:8090/api/auth/oauth/google?state=c2",
      );
    });

    // Complete the flow.
    es.emit("oauth", {
      token: "tok",
      refreshToken: "ref",
      user: { id: "1", email: "a@b.com" },
    });

    const result = await promise;
    expect(result.token).toBe("tok");
  });

  it("SSE connection receives clientId", async () => {
    window.open = vi.fn().mockReturnValue({
      closed: false,
      close: vi.fn(),
      location: { href: "" },
    }) as unknown as typeof window.open;

    const client = new AYBClient("http://localhost:8090", {
      fetch: mockFetch(200, {}),
    });

    client.auth.signInWithOAuth("google");

    const es = MockEventSource.instances[0];
    expect(es).toBeDefined();
    expect(es.url).toContain("oauth=true");

    // Emit connected and verify clientId is used as state.
    es.emit("connected", { clientId: "test-id-123" });

    await vi.waitFor(() => {
      const popup = (window.open as ReturnType<typeof vi.fn>).mock
        .results[0].value;
      expect(popup.location.href).toContain("state=test-id-123");
    });

    // Cleanup.
    es.emit("oauth", {
      token: "t",
      refreshToken: "r",
      user: { id: "1", email: "a@b.com" },
    });
  });

  it("SSE error event rejects promise", async () => {
    window.open = vi.fn().mockReturnValue({
      closed: false,
      close: vi.fn(),
      location: { href: "" },
    }) as unknown as typeof window.open;

    const client = new AYBClient("http://localhost:8090", {
      fetch: mockFetch(200, {}),
    });

    const promise = client.auth.signInWithOAuth("google");

    const es = MockEventSource.instances[0];
    es.emit("connected", { clientId: "c1" });

    // Yield to let signInWithOAuth resolve connectOAuthSSE and attach the oauth listener.
    await new Promise((r) => setTimeout(r, 0));

    // Simulate an OAuth error from the server.
    es.emit("oauth", { error: "access denied by provider" });

    await expect(promise).rejects.toThrow("access denied by provider");
    await expect(promise).rejects.toBeInstanceOf(AYBError);
  });

  it("SSE oauth event with missing tokens rejects promise", async () => {
    window.open = vi.fn().mockReturnValue({
      closed: false,
      close: vi.fn(),
      location: { href: "" },
    }) as unknown as typeof window.open;

    const client = new AYBClient("http://localhost:8090", {
      fetch: mockFetch(200, {}),
    });

    const promise = client.auth.signInWithOAuth("google");

    const es = MockEventSource.instances[0];
    es.emit("connected", { clientId: "c1" });

    // Yield to let signInWithOAuth resolve connectOAuthSSE and attach the oauth listener.
    await new Promise((r) => setTimeout(r, 0));

    es.emit("oauth", { token: "tok" }); // Missing refreshToken.

    await expect(promise).rejects.toThrow("missing tokens");
  });

  it("SSE connection failure rejects promise", async () => {
    window.open = vi.fn().mockReturnValue({
      closed: false,
      close: vi.fn(),
      location: { href: "" },
    }) as unknown as typeof window.open;

    const client = new AYBClient("http://localhost:8090", {
      fetch: mockFetch(200, {}),
    });

    const promise = client.auth.signInWithOAuth("google");

    const es = MockEventSource.instances[0];
    es.triggerError();

    await expect(promise).rejects.toThrow(
      "Failed to connect to OAuth SSE channel",
    );
  });

  it("popup closed by user rejects promise", async () => {
    const mockPopup = {
      closed: false,
      close: vi.fn(),
      location: { href: "" },
    };
    window.open = vi.fn().mockReturnValue(mockPopup) as unknown as typeof window.open;

    const client = new AYBClient("http://localhost:8090", {
      fetch: mockFetch(200, {}),
    });

    const promise = client.auth.signInWithOAuth("google");

    const es = MockEventSource.instances[0];
    es.emit("connected", { clientId: "c1" });

    // Simulate user closing the popup.
    mockPopup.closed = true;

    await expect(promise).rejects.toThrow("popup was closed");
  });

  it("closes popup on error", async () => {
    expect.assertions(2);

    const mockPopup = {
      closed: false,
      close: vi.fn(),
      location: { href: "" },
    };
    window.open = vi.fn().mockReturnValue(mockPopup) as unknown as typeof window.open;

    const client = new AYBClient("http://localhost:8090", {
      fetch: mockFetch(200, {}),
    });

    const promise = client.auth.signInWithOAuth("google");

    const es = MockEventSource.instances[0];
    es.triggerError();

    await expect(promise).rejects.toThrow(
      "Failed to connect to OAuth SSE channel",
    );
    expect(mockPopup.close).toHaveBeenCalled();
  });

  it("works with github provider", async () => {
    window.open = vi.fn().mockReturnValue({
      closed: false,
      close: vi.fn(),
      location: { href: "" },
    }) as unknown as typeof window.open;

    const client = new AYBClient("http://localhost:8090", {
      fetch: mockFetch(200, {}),
    });

    const promise = client.auth.signInWithOAuth("github");

    const es = MockEventSource.instances[0];
    es.emit("connected", { clientId: "c1" });

    await vi.waitFor(() => {
      const popup = (window.open as ReturnType<typeof vi.fn>).mock
        .results[0].value;
      expect(popup.location.href).toContain("/api/auth/oauth/github?state=c1");
    });

    es.emit("oauth", {
      token: "t",
      refreshToken: "r",
      user: { id: "1", email: "a@b.com" },
    });

    const result = await promise;
    expect(result.token).toBe("t");
    expect(result.user.email).toBe("a@b.com");
    expect(client.token).toBe("t");
  });
});

describe("handleOAuthRedirect", () => {
  it("parses tokens from hash fragment", () => {
    // Set up window.location with hash.
    Object.defineProperty(window, "location", {
      value: {
        hash: "#token=my-token&refreshToken=my-refresh",
        pathname: "/callback",
        search: "",
      },
      writable: true,
      configurable: true,
    });

    const replaceState = vi.fn();
    Object.defineProperty(window, "history", {
      value: { replaceState },
      writable: true,
      configurable: true,
    });

    const client = new AYBClient("http://localhost:8090", {
      fetch: mockFetch(200, {}),
    });

    const result = client.auth.handleOAuthRedirect();

    expect(result).not.toBeNull();
    expect(result!.token).toBe("my-token");
    expect(result!.refreshToken).toBe("my-refresh");

    // Tokens should be stored.
    expect(client.token).toBe("my-token");
    expect(client.refreshToken).toBe("my-refresh");

    // URL hash should be cleaned up.
    expect(replaceState).toHaveBeenCalledWith(null, "", "/callback");
  });

  it("returns null when no hash", () => {
    Object.defineProperty(window, "location", {
      value: { hash: "", pathname: "/callback", search: "" },
      writable: true,
      configurable: true,
    });

    const client = new AYBClient("http://localhost:8090", {
      fetch: mockFetch(200, {}),
    });

    const result = client.auth.handleOAuthRedirect();
    expect(result).toBeNull();
  });

  it("returns null when hash has no tokens", () => {
    Object.defineProperty(window, "location", {
      value: {
        hash: "#foo=bar",
        pathname: "/callback",
        search: "",
      },
      writable: true,
      configurable: true,
    });

    const client = new AYBClient("http://localhost:8090", {
      fetch: mockFetch(200, {}),
    });

    const result = client.auth.handleOAuthRedirect();
    expect(result).toBeNull();
  });

  it("returns null when only token present (no refreshToken)", () => {
    Object.defineProperty(window, "location", {
      value: {
        hash: "#token=my-token",
        pathname: "/callback",
        search: "",
      },
      writable: true,
      configurable: true,
    });

    const client = new AYBClient("http://localhost:8090", {
      fetch: mockFetch(200, {}),
    });

    const result = client.auth.handleOAuthRedirect();
    expect(result).toBeNull();
  });

  it("preserves search params when cleaning hash", () => {
    Object.defineProperty(window, "location", {
      value: {
        hash: "#token=t&refreshToken=r",
        pathname: "/callback",
        search: "?page=1",
      },
      writable: true,
      configurable: true,
    });

    const replaceState = vi.fn();
    Object.defineProperty(window, "history", {
      value: { replaceState },
      writable: true,
      configurable: true,
    });

    const client = new AYBClient("http://localhost:8090", {
      fetch: mockFetch(200, {}),
    });

    client.auth.handleOAuthRedirect();

    expect(replaceState).toHaveBeenCalledWith(null, "", "/callback?page=1");
  });

  it("fires SIGNED_IN auth state event", () => {
    Object.defineProperty(window, "location", {
      value: {
        hash: "#token=t&refreshToken=r",
        pathname: "/callback",
        search: "",
      },
      writable: true,
      configurable: true,
    });
    Object.defineProperty(window, "history", {
      value: { replaceState: vi.fn() },
      writable: true,
      configurable: true,
    });

    const client = new AYBClient("http://localhost:8090", {
      fetch: mockFetch(200, {}),
    });

    const listener = vi.fn();
    client.onAuthStateChange(listener);
    client.auth.handleOAuthRedirect();

    expect(listener).toHaveBeenCalledWith("SIGNED_IN", {
      token: "t",
      refreshToken: "r",
    });
  });
});

describe("signInWithOAuth error codes", () => {
  let originalEventSource: typeof EventSource;
  let originalWindowOpen: typeof window.open;

  beforeEach(() => {
    MockEventSource.instances = [];
    originalEventSource = globalThis.EventSource;
    (globalThis as unknown as Record<string, unknown>).EventSource =
      MockEventSource;
    originalWindowOpen = window.open;
  });

  afterEach(() => {
    (globalThis as unknown as Record<string, unknown>).EventSource =
      originalEventSource;
    window.open = originalWindowOpen;
  });

  it("popup-blocked has error code oauth/popup-blocked", async () => {
    expect.assertions(3);
    window.open = vi.fn().mockReturnValue(null) as unknown as typeof window.open;

    const client = new AYBClient("http://localhost:8090", {
      fetch: mockFetch(200, {}),
    });

    try {
      await client.auth.signInWithOAuth("google");
      expect.unreachable("should have thrown");
    } catch (err) {
      expect(err).toBeInstanceOf(AYBError);
      expect((err as AYBError).code).toBe("oauth/popup-blocked");
      expect((err as AYBError).status).toBe(403);
    }
  });

  it("SSE failure has error code oauth/sse-failed", async () => {
    expect.assertions(3);
    window.open = vi.fn().mockReturnValue({
      closed: false,
      close: vi.fn(),
      location: { href: "" },
    }) as unknown as typeof window.open;

    const client = new AYBClient("http://localhost:8090", {
      fetch: mockFetch(200, {}),
    });

    const promise = client.auth.signInWithOAuth("google");
    const es = MockEventSource.instances[0];
    es.triggerError();

    try {
      await promise;
      expect.unreachable("should have thrown");
    } catch (err) {
      expect(err).toBeInstanceOf(AYBError);
      expect((err as AYBError).code).toBe("oauth/sse-failed");
      expect((err as AYBError).status).toBe(503);
    }
  });

  it("provider error has error code oauth/provider-error", async () => {
    expect.assertions(3);
    window.open = vi.fn().mockReturnValue({
      closed: false,
      close: vi.fn(),
      location: { href: "" },
    }) as unknown as typeof window.open;

    const client = new AYBClient("http://localhost:8090", {
      fetch: mockFetch(200, {}),
    });

    const promise = client.auth.signInWithOAuth("google");
    const es = MockEventSource.instances[0];
    es.emit("connected", { clientId: "c1" });
    await new Promise((r) => setTimeout(r, 0));
    es.emit("oauth", { error: "access denied by provider" });

    try {
      await promise;
      expect.unreachable("should have thrown");
    } catch (err) {
      expect(err).toBeInstanceOf(AYBError);
      expect((err as AYBError).code).toBe("oauth/provider-error");
      expect((err as AYBError).status).toBe(401);
    }
  });

  it("missing tokens has error code oauth/missing-tokens", async () => {
    expect.assertions(3);
    window.open = vi.fn().mockReturnValue({
      closed: false,
      close: vi.fn(),
      location: { href: "" },
    }) as unknown as typeof window.open;

    const client = new AYBClient("http://localhost:8090", {
      fetch: mockFetch(200, {}),
    });

    const promise = client.auth.signInWithOAuth("google");
    const es = MockEventSource.instances[0];
    es.emit("connected", { clientId: "c1" });
    await new Promise((r) => setTimeout(r, 0));
    es.emit("oauth", { token: "tok" }); // Missing refreshToken.

    try {
      await promise;
      expect.unreachable("should have thrown");
    } catch (err) {
      expect(err).toBeInstanceOf(AYBError);
      expect((err as AYBError).code).toBe("oauth/missing-tokens");
      expect((err as AYBError).status).toBe(500);
    }
  });

  it("popup closed has error code oauth/popup-closed", async () => {
    expect.assertions(3);
    const mockPopup = {
      closed: false,
      close: vi.fn(),
      location: { href: "" },
    };
    window.open = vi.fn().mockReturnValue(mockPopup) as unknown as typeof window.open;

    const client = new AYBClient("http://localhost:8090", {
      fetch: mockFetch(200, {}),
    });

    const promise = client.auth.signInWithOAuth("google");
    const es = MockEventSource.instances[0];
    es.emit("connected", { clientId: "c1" });
    mockPopup.closed = true;

    try {
      await promise;
      expect.unreachable("should have thrown");
    } catch (err) {
      expect(err).toBeInstanceOf(AYBError);
      expect((err as AYBError).code).toBe("oauth/popup-closed");
      expect((err as AYBError).status).toBe(499);
    }
  });
});

describe("signInWithOAuth scopes", () => {
  let originalEventSource: typeof EventSource;
  let originalWindowOpen: typeof window.open;

  beforeEach(() => {
    MockEventSource.instances = [];
    originalEventSource = globalThis.EventSource;
    (globalThis as unknown as Record<string, unknown>).EventSource =
      MockEventSource;
    originalWindowOpen = window.open;
  });

  afterEach(() => {
    (globalThis as unknown as Record<string, unknown>).EventSource =
      originalEventSource;
    window.open = originalWindowOpen;
  });

  it("passes scopes to OAuth URL", async () => {
    const openSpy = vi.fn().mockReturnValue({
      closed: false,
      close: vi.fn(),
      location: { href: "" },
    });
    window.open = openSpy as unknown as typeof window.open;

    const client = new AYBClient("http://localhost:8090", {
      fetch: mockFetch(200, {}),
    });

    const promise = client.auth.signInWithOAuth("google", {
      scopes: ["calendar.read", "drive.read"],
    });

    const es = MockEventSource.instances[0];
    es.emit("connected", { clientId: "c1" });

    await vi.waitFor(() => {
      const popup = openSpy.mock.results[0].value;
      expect(popup.location.href).toContain("scopes=");
      expect(popup.location.href).toContain("calendar.read");
      expect(popup.location.href).toContain("drive.read");
    });

    es.emit("oauth", {
      token: "t",
      refreshToken: "r",
      user: { id: "1", email: "a@b.com" },
    });
    await promise;
  });

  it("omits scopes param when no scopes provided", async () => {
    const openSpy = vi.fn().mockReturnValue({
      closed: false,
      close: vi.fn(),
      location: { href: "" },
    });
    window.open = openSpy as unknown as typeof window.open;

    const client = new AYBClient("http://localhost:8090", {
      fetch: mockFetch(200, {}),
    });

    const promise = client.auth.signInWithOAuth("google");

    const es = MockEventSource.instances[0];
    es.emit("connected", { clientId: "c1" });

    await vi.waitFor(() => {
      const popup = openSpy.mock.results[0].value;
      expect(popup.location.href).not.toContain("scopes=");
    });

    es.emit("oauth", {
      token: "t",
      refreshToken: "r",
      user: { id: "1", email: "a@b.com" },
    });
    await promise;
  });
});

describe("signInWithOAuth popup auto-close", () => {
  let originalEventSource: typeof EventSource;
  let originalWindowOpen: typeof window.open;

  beforeEach(() => {
    MockEventSource.instances = [];
    originalEventSource = globalThis.EventSource;
    (globalThis as unknown as Record<string, unknown>).EventSource =
      MockEventSource;
    originalWindowOpen = window.open;
  });

  afterEach(() => {
    (globalThis as unknown as Record<string, unknown>).EventSource =
      originalEventSource;
    window.open = originalWindowOpen;
  });

  it("closes popup on successful OAuth", async () => {
    const mockPopup = {
      closed: false,
      close: vi.fn(),
      location: { href: "" },
    };
    window.open = vi.fn().mockReturnValue(mockPopup) as unknown as typeof window.open;

    const client = new AYBClient("http://localhost:8090", {
      fetch: mockFetch(200, {}),
    });

    const promise = client.auth.signInWithOAuth("google");
    const es = MockEventSource.instances[0];
    es.emit("connected", { clientId: "c1" });

    await new Promise((r) => setTimeout(r, 0));

    es.emit("oauth", {
      token: "access-tok",
      refreshToken: "refresh-tok",
      user: { id: "u1", email: "user@example.com" },
    });

    await promise;
    expect(mockPopup.close).toHaveBeenCalled();
  });
});

describe("onAuthStateChange", () => {
  let originalEventSource: typeof EventSource;
  let originalWindowOpen: typeof window.open;

  beforeEach(() => {
    MockEventSource.instances = [];
    originalEventSource = globalThis.EventSource;
    (globalThis as unknown as Record<string, unknown>).EventSource =
      MockEventSource;
    originalWindowOpen = window.open;
  });

  afterEach(() => {
    (globalThis as unknown as Record<string, unknown>).EventSource =
      originalEventSource;
    window.open = originalWindowOpen;
  });

  it("fires SIGNED_IN on OAuth success", async () => {
    expect.assertions(1);

    window.open = vi.fn().mockReturnValue({
      closed: false,
      close: vi.fn(),
      location: { href: "" },
    }) as unknown as typeof window.open;

    const client = new AYBClient("http://localhost:8090", {
      fetch: mockFetch(200, {}),
    });

    const listener = vi.fn();
    client.onAuthStateChange(listener);

    const promise = client.auth.signInWithOAuth("google");
    const es = MockEventSource.instances[0];
    es.emit("connected", { clientId: "c1" });
    await new Promise((r) => setTimeout(r, 0));
    es.emit("oauth", {
      token: "tok",
      refreshToken: "ref",
      user: { id: "1", email: "a@b.com" },
    });
    await promise;

    expect(listener).toHaveBeenCalledWith("SIGNED_IN", {
      token: "tok",
      refreshToken: "ref",
    });
  });

  it("fires SIGNED_IN on login", async () => {
    expect.assertions(1);

    const client = new AYBClient("http://localhost:8090", {
      fetch: mockFetch(200, {
        token: "t",
        refreshToken: "r",
        user: { id: "1", email: "a@b.com" },
      }),
    });

    const listener = vi.fn();
    client.onAuthStateChange(listener);

    await client.auth.login("a@b.com", "pass");

    expect(listener).toHaveBeenCalledWith("SIGNED_IN", {
      token: "t",
      refreshToken: "r",
    });
  });

  it("fires SIGNED_OUT on logout", async () => {
    expect.assertions(1);

    const client = new AYBClient("http://localhost:8090", {
      fetch: mockFetch(204, undefined),
    });
    client.setTokens("tok", "ref");

    const listener = vi.fn();
    client.onAuthStateChange(listener);

    await client.auth.logout();

    expect(listener).toHaveBeenCalledWith("SIGNED_OUT", null);
  });

  it("fires TOKEN_REFRESHED on refresh", async () => {
    expect.assertions(1);

    const client = new AYBClient("http://localhost:8090", {
      fetch: mockFetch(200, {
        token: "new-t",
        refreshToken: "new-r",
        user: { id: "1", email: "a@b.com" },
      }),
    });
    client.setTokens("old-t", "old-r");

    const listener = vi.fn();
    client.onAuthStateChange(listener);

    await client.auth.refresh();

    expect(listener).toHaveBeenCalledWith("TOKEN_REFRESHED", {
      token: "new-t",
      refreshToken: "new-r",
    });
  });

  it("unsubscribe stops events", async () => {
    const client = new AYBClient("http://localhost:8090", {
      fetch: mockFetch(200, {
        token: "t",
        refreshToken: "r",
        user: { id: "1", email: "a@b.com" },
      }),
    });

    const listener = vi.fn();
    const unsub = client.onAuthStateChange(listener);
    unsub();

    await client.auth.login("a@b.com", "pass");

    expect(listener).not.toHaveBeenCalled();
  });

  it("multiple listeners all receive events", async () => {
    expect.assertions(2);

    const client = new AYBClient("http://localhost:8090", {
      fetch: mockFetch(200, {
        token: "t",
        refreshToken: "r",
        user: { id: "1", email: "a@b.com" },
      }),
    });

    const listener1 = vi.fn();
    const listener2 = vi.fn();
    client.onAuthStateChange(listener1);
    client.onAuthStateChange(listener2);

    await client.auth.login("a@b.com", "pass");

    expect(listener1).toHaveBeenCalledWith("SIGNED_IN", {
      token: "t",
      refreshToken: "r",
    });
    expect(listener2).toHaveBeenCalledWith("SIGNED_IN", {
      token: "t",
      refreshToken: "r",
    });
  });
});
