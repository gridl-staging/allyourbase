import { beforeEach, describe, expect, it, vi } from "vitest";
import {
  createDrain,
  deleteDrain,
  listDrains,
} from "../api_drains";
import {
  createSAMLProvider,
  deleteSAMLProvider,
  listSAMLProviders,
  updateSAMLProvider,
} from "../api_saml";

describe("stage 6 admin API request paths", () => {
  const fetchMock = vi.fn<typeof fetch>();

  beforeEach(() => {
    vi.clearAllMocks();
    localStorage.clear();
    localStorage.setItem("ayb_admin_token", "admin-token");
    vi.stubGlobal("fetch", fetchMock);
  });

  it("uses the backend SAML admin routes", async () => {
    fetchMock
      .mockResolvedValueOnce(
        new Response(JSON.stringify({ providers: [] }), {
          status: 200,
          headers: { "Content-Type": "application/json" },
        }),
      )
      .mockResolvedValueOnce(
        new Response(
          JSON.stringify({
            name: "okta prod",
            entity_id: "https://sp.example.com/okta",
            idp_metadata_xml: "<EntityDescriptor />",
            attribute_mapping: {},
            created_at: "2026-03-15T00:00:00Z",
            updated_at: "2026-03-15T00:00:00Z",
          }),
          {
            status: 200,
            headers: { "Content-Type": "application/json" },
          },
        ),
      )
      .mockResolvedValueOnce(
        new Response(
          JSON.stringify({
            name: "okta prod",
            entity_id: "https://sp.example.com/okta",
            idp_metadata_xml: "<EntityDescriptor />",
            attribute_mapping: {},
            created_at: "2026-03-15T00:00:00Z",
            updated_at: "2026-03-15T00:00:00Z",
          }),
          {
            status: 200,
            headers: { "Content-Type": "application/json" },
          },
        ),
      )
      .mockResolvedValueOnce(new Response(null, { status: 204 }));

    await expect(listSAMLProviders()).resolves.toEqual([]);
    await createSAMLProvider({
      name: "okta prod",
      entity_id: "https://sp.example.com/okta",
      idp_metadata_xml: "<EntityDescriptor />",
      attribute_mapping: {},
    });
    await updateSAMLProvider("okta prod", {
      name: "okta prod",
      entity_id: "https://sp.example.com/okta",
      idp_metadata_xml: "<EntityDescriptor />",
      attribute_mapping: { email: "mail" },
    });
    await deleteSAMLProvider("okta prod");

    expect(fetchMock.mock.calls[0]).toEqual([
      "/api/admin/auth/saml",
      {
        headers: {
          Authorization: "Bearer admin-token",
        },
      },
    ]);
    expect(fetchMock.mock.calls[1]).toEqual([
      "/api/admin/auth/saml",
      {
        method: "POST",
        headers: {
          "Content-Type": "application/json",
          Authorization: "Bearer admin-token",
        },
        body: JSON.stringify({
          name: "okta prod",
          entity_id: "https://sp.example.com/okta",
          idp_metadata_xml: "<EntityDescriptor />",
          attribute_mapping: {},
        }),
      },
    ]);
    expect(fetchMock.mock.calls[2]).toEqual([
      "/api/admin/auth/saml/okta%20prod",
      {
        method: "PUT",
        headers: {
          "Content-Type": "application/json",
          Authorization: "Bearer admin-token",
        },
        body: JSON.stringify({
          name: "okta prod",
          entity_id: "https://sp.example.com/okta",
          idp_metadata_xml: "<EntityDescriptor />",
          attribute_mapping: { email: "mail" },
        }),
      },
    ]);
    expect(fetchMock.mock.calls[3]).toEqual([
      "/api/admin/auth/saml/okta%20prod",
      {
        method: "DELETE",
        headers: {
          Authorization: "Bearer admin-token",
        },
      },
    ]);
  });

  it("uses the backend log drain admin routes", async () => {
    fetchMock
      .mockResolvedValueOnce(
        new Response(JSON.stringify([]), {
          status: 200,
          headers: { "Content-Type": "application/json" },
        }),
      )
      .mockResolvedValueOnce(
        new Response(
          JSON.stringify({
            id: "drain/http primary",
            name: "HTTP Primary",
            stats: { sent: 10, failed: 0, dropped: 0 },
          }),
          {
            status: 200,
            headers: { "Content-Type": "application/json" },
          },
        ),
      )
      .mockResolvedValueOnce(new Response(null, { status: 204 }));

    await listDrains();
    await createDrain({
      type: "http",
      url: "https://logs.example.com/ingest",
      batch_size: 100,
      flush_interval_seconds: 5,
      enabled: true,
    });
    await deleteDrain("drain/http primary");

    expect(fetchMock.mock.calls[0]).toEqual([
      "/api/admin/logging/drains",
      {
        headers: {
          Authorization: "Bearer admin-token",
        },
      },
    ]);
    expect(fetchMock.mock.calls[1]).toEqual([
      "/api/admin/logging/drains",
      {
        method: "POST",
        headers: {
          "Content-Type": "application/json",
          Authorization: "Bearer admin-token",
        },
        body: JSON.stringify({
          type: "http",
          url: "https://logs.example.com/ingest",
          batch_size: 100,
          flush_interval_seconds: 5,
          enabled: true,
        }),
      },
    ]);
    expect(fetchMock.mock.calls[2]).toEqual([
      "/api/admin/logging/drains/drain%2Fhttp%20primary",
      {
        method: "DELETE",
        headers: {
          Authorization: "Bearer admin-token",
        },
      },
    ]);
  });
});
