import { beforeEach, describe, expect, it, vi } from "vitest";
import { fetchAdmin } from "../api_client";
import {
  deleteStorageFile,
  getSignedURL,
  listStorageFiles,
  purgeStorageCDN,
  storageDownloadURL,
  uploadStorageFile,
} from "../api_storage";

function jsonResponse(body: unknown, init?: ResponseInit): Response {
  return new Response(JSON.stringify(body), {
    status: 200,
    headers: { "Content-Type": "application/json" },
    ...init,
  });
}

describe("purgeStorageCDN request paths", () => {
  const fetchMock = vi.fn<typeof fetch>();

  beforeEach(() => {
    vi.clearAllMocks();
    localStorage.clear();
    localStorage.setItem("ayb_admin_token", "admin-token");
    vi.stubGlobal("fetch", fetchMock);
  });

  it("encodes bucket and object path segments before issuing storage requests", async () => {
    fetchMock
      .mockResolvedValueOnce(jsonResponse({ items: [] }))
      .mockResolvedValueOnce(jsonResponse({ key: "uploaded" }))
      .mockResolvedValueOnce(new Response(null, { status: 204 }))
      .mockResolvedValueOnce(jsonResponse({ url: "https://signed.example.com/object" }));

    await listStorageFiles("../admin", { prefix: "../drafts?state=active#frag" });
    await uploadStorageFile("../bucket", new File(["data"], "payload.txt", { type: "text/plain" }));
    await deleteStorageFile("uploads", "../../api/admin/logs?tail=1#frag");
    const signed = await getSignedURL("uploads", "../logs/latest.txt", 300);

    expect(fetchMock).toHaveBeenNthCalledWith(1, "/api/storage/..%2Fadmin?prefix=..%2Fdrafts%3Fstate%3Dactive%23frag", {
      headers: { Authorization: "Bearer admin-token" },
    });
    expect(fetchMock).toHaveBeenNthCalledWith(2, "/api/storage/..%2Fbucket", {
      method: "POST",
      body: expect.any(FormData),
      headers: { Authorization: "Bearer admin-token" },
    });
    expect(fetchMock).toHaveBeenNthCalledWith(3, "/api/storage/uploads/%2E%2E/%2E%2E/api/admin/logs%3Ftail%3D1%23frag", {
      method: "DELETE",
      headers: { Authorization: "Bearer admin-token" },
    });
    expect(fetchMock).toHaveBeenNthCalledWith(4, "/api/storage/uploads/%2E%2E/logs/latest.txt/sign", {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
        Authorization: "Bearer admin-token",
      },
      body: JSON.stringify({ expiresIn: 300 }),
    });
    expect(storageDownloadURL("../bucket", "../reports/index.html?download=1#frag"))
      .toBe("/api/storage/..%2Fbucket/%2E%2E/reports/index.html%3Fdownload%3D1%23frag");
    expect(signed).toEqual({ url: "https://signed.example.com/object" });
  });

  it("sends POST with urls body for targeted purge", async () => {
    fetchMock.mockResolvedValueOnce(
      jsonResponse({ operation: "purge_urls", submitted: 2, provider: "cloudflare" }, { status: 202 }),
    );

    const result = await purgeStorageCDN({ urls: ["https://cdn.example.com/a.js", "https://cdn.example.com/b.css"] });

    expect(fetchMock).toHaveBeenCalledWith("/api/admin/storage/cdn/purge", {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
        Authorization: "Bearer admin-token",
      },
      body: JSON.stringify({ urls: ["https://cdn.example.com/a.js", "https://cdn.example.com/b.css"] }),
    });
    expect(result).toEqual({ operation: "purge_urls", submitted: 2, provider: "cloudflare" });
  });

  it("sends POST with purge_all body for full cache invalidation", async () => {
    fetchMock.mockResolvedValueOnce(
      jsonResponse({ operation: "purge_all", submitted: 0, provider: "cloudflare" }, { status: 202 }),
    );

    const result = await purgeStorageCDN({ purgeAll: true });

    expect(fetchMock).toHaveBeenCalledWith("/api/admin/storage/cdn/purge", {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
        Authorization: "Bearer admin-token",
      },
      body: JSON.stringify({ purge_all: true }),
    });
    expect(result).toEqual({ operation: "purge_all", submitted: 0, provider: "cloudflare" });
  });

  it("throws ApiError with backend message on 400 validation error", async () => {
    fetchMock.mockResolvedValueOnce(
      jsonResponse({ code: 400, message: "choose exactly one mode" }, { status: 400 }),
    );

    await expect(purgeStorageCDN({ urls: [] })).rejects.toThrow("choose exactly one mode");
  });

  it("throws ApiError with backend message on 429 rate limit", async () => {
    fetchMock.mockResolvedValueOnce(
      jsonResponse({ code: 429, message: "cdn purge_all rate limit exceeded" }, {
        status: 429,
        headers: { "Content-Type": "application/json", "Retry-After": "60" },
      }),
    );

    await expect(purgeStorageCDN({ purgeAll: true })).rejects.toThrow("cdn purge_all rate limit exceeded");
  });

  it("rejects cross-origin admin requests before any bearer token is sent", async () => {
    await expect(fetchAdmin("https://attacker.example/collect")).rejects.toThrow(
      "Cross-origin API requests are not allowed",
    );

    expect(fetchMock).not.toHaveBeenCalled();
  });

  it("captures Retry-After metadata on 429 rate limit errors", async () => {
    fetchMock.mockResolvedValueOnce(
      jsonResponse({ code: 429, message: "cdn purge_all rate limit exceeded" }, {
        status: 429,
        headers: { "Content-Type": "application/json", "Retry-After": "60" },
      }),
    );

    let error: unknown;
    try {
      await purgeStorageCDN({ purgeAll: true });
    } catch (caught) {
      error = caught;
    }

    expect(error).toMatchObject({
      status: 429,
      retryAfterSeconds: 60,
    });
  });
});
