import { describe, it, expect } from "vitest";
import { AYBClient } from "./client";
import { AYBError } from "./errors";
import { mockFetchSequence } from "./test_utils/mockFetchSequence";

describe("SDK contract fixtures", () => {
  it("auth response fixture normalizes user aliases", async () => {
    const fetchFn = mockFetchSequence([
      {
        status: 200,
        body: {
          token: "jwt_stage3",
          refreshToken: "refresh_stage3",
          user: {
            id: "usr_1",
            email: "dev@allyourbase.io",
            email_verified: true,
            created_at: "2026-01-01T00:00:00Z",
            updated_at: null,
          },
        },
      },
    ]);

    const client = new AYBClient("https://api.example.com", { fetch: fetchFn });
    const auth = await client.auth.login("dev@allyourbase.io", "secret");

    expect(auth.token).toBe("jwt_stage3");
    expect(auth.refreshToken).toBe("refresh_stage3");
    expect(auth.user.emailVerified).toBe(true);
    expect(auth.user.createdAt).toBe("2026-01-01T00:00:00Z");
    expect(auth.user.updatedAt).toBeUndefined();
  });

  it("list response fixture preserves metadata and order", async () => {
    const fetchFn = mockFetchSequence([
      {
        status: 200,
        body: {
          items: [
            { id: "rec_1", title: "First" },
            { id: "rec_2", title: "Second" },
          ],
          page: 1,
          perPage: 2,
          totalItems: 2,
          totalPages: 1,
        },
      },
    ]);

    const client = new AYBClient("https://api.example.com", { fetch: fetchFn });
    const list = await client.records.list("posts");

    expect(list.totalItems).toBe(2);
    expect(list.items[0].title).toBe("First");
    expect(list.items[1].title).toBe("Second");
  });

  it("error fixtures normalize numeric and string code variants", async () => {
    const fetchFn = mockFetchSequence([
      {
        status: 403,
        body: {
          code: 403,
          message: "forbidden",
          data: { resource: "posts" },
          doc_url: "https://allyourbase.io/docs/errors#forbidden",
        },
      },
      {
        status: 400,
        body: {
          code: "auth/missing-refresh-token",
          message: "Missing refresh token",
          data: { detail: "refresh token not available" },
        },
      },
    ]);

    const client = new AYBClient("https://api.example.com", { fetch: fetchFn });

    await expect(client.records.list("posts")).rejects.toMatchObject<Partial<AYBError>>({
      status: 403,
      message: "forbidden",
      code: "403",
      data: { resource: "posts" },
      docUrl: "https://allyourbase.io/docs/errors#forbidden",
    });

    await expect(client.auth.refresh()).rejects.toMatchObject<Partial<AYBError>>({
      status: 400,
      message: "Missing refresh token",
      code: "auth/missing-refresh-token",
      data: { detail: "refresh token not available" },
    });
  });

  it("storage object fixture and list fixture decode nullable fields", async () => {
    const fetchFn = mockFetchSequence([
      {
        status: 200,
        body: {
          id: "file_abc123",
          bucket: "uploads",
          name: "document.pdf",
          size: 1024,
          contentType: "application/pdf",
          userId: "usr_1",
          createdAt: "2026-01-01T00:00:00Z",
          updatedAt: "2026-01-02T12:30:00Z",
        },
      },
      {
        status: 200,
        body: {
          items: [
            {
              id: "file_1",
              bucket: "uploads",
              name: "doc1.pdf",
              size: 1024,
              contentType: "application/pdf",
              userId: "usr_1",
              createdAt: "2026-01-01T00:00:00Z",
              updatedAt: null,
            },
            {
              id: "file_2",
              bucket: "uploads",
              name: "image.png",
              size: 2048,
              contentType: "image/png",
              userId: null,
              createdAt: "2026-01-02T00:00:00Z",
              updatedAt: null,
            },
          ],
          totalItems: 2,
        },
      },
    ]);

    const client = new AYBClient("https://api.example.com", { fetch: fetchFn });

    const uploaded = await client.storage.upload("uploads", new Blob(["hello"]), "document.pdf");
    expect(uploaded.id).toBe("file_abc123");
    expect(uploaded.userId).toBe("usr_1");

    const listed = await client.storage.list("uploads");
    expect(listed.totalItems).toBe(2);
    expect(listed.items[0].userId).toBe("usr_1");
    expect(listed.items[0].updatedAt).toBeUndefined();
    expect(listed.items[1].userId).toBeUndefined();
    expect(listed.items[1].updatedAt).toBeUndefined();
  });
});
