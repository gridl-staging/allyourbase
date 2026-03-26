import { describe, expect, it } from "vitest";
import {
  buildOIDCUpdatePayload,
  buildProviderUpdatePayload,
  createProviderForm,
} from "../auth-settings-helpers";

describe("auth-settings-helpers", () => {
  it("buildProviderUpdatePayload includes provider-specific fields", () => {
    const microsoftPayload = buildProviderUpdatePayload("microsoft", {
      enabled: true,
      client_id: "client",
      client_secret: "secret",
      tenant_id: "tenant",
      team_id: "",
      key_id: "",
      private_key: "",
    });
    expect(microsoftPayload).toEqual({
      enabled: true,
      client_id: "client",
      client_secret: "secret",
      tenant_id: "tenant",
    });

    const applePayload = buildProviderUpdatePayload("apple", {
      enabled: true,
      client_id: "apple-client",
      client_secret: "",
      tenant_id: "",
      team_id: "TEAM",
      key_id: "KEY",
      private_key: "PRIVATE",
    });
    expect(applePayload).toEqual({
      enabled: true,
      client_id: "apple-client",
      team_id: "TEAM",
      key_id: "KEY",
      private_key: "PRIVATE",
    });

    const oidcPayload = buildProviderUpdatePayload("custom-oidc", {
      enabled: true,
      client_id: "oidc-client",
      client_secret: "oidc-secret",
      tenant_id: "",
      team_id: "",
      key_id: "",
      private_key: "",
      issuer_url: " https://issuer.example.com/realms/main ",
      display_name: "  My OIDC Provider  ",
      scopes: "openid   profile   email",
    });
    expect(oidcPayload).toEqual({
      enabled: true,
      client_id: "oidc-client",
      client_secret: "oidc-secret",
      issuer_url: "https://issuer.example.com/realms/main",
      display_name: "My OIDC Provider",
      scopes: ["openid", "profile", "email"],
    });
  });

  it("createProviderForm initializes OIDC-capable fields and omits empty OIDC payload values", () => {
    const form = createProviderForm({
      name: "custom-oidc",
      type: "oidc",
      enabled: false,
      client_id_configured: false,
    });

    const formAsRecord = form as Record<string, unknown>;
    expect(formAsRecord.issuer_url).toBe("");
    expect(formAsRecord.display_name).toBe("");
    expect(formAsRecord.scopes).toBe("");

    const payload = buildProviderUpdatePayload("custom-oidc", {
      ...form,
      issuer_url: "   ",
      display_name: "",
      scopes: "   ",
    });

    expect(payload).toEqual({
      enabled: false,
    });
  });

  it("buildOIDCUpdatePayload trims and normalizes scopes", () => {
    const payload = buildOIDCUpdatePayload({
      provider_name: "custom",
      issuer_url: " https://issuer.example ",
      client_id: " client-id ",
      client_secret: " secret ",
      display_name: "  ",
      scopes: "openid   profile email",
    });

    expect(payload).toEqual({
      enabled: true,
      issuer_url: "https://issuer.example",
      client_id: "client-id",
      client_secret: "secret",
      display_name: undefined,
      scopes: ["openid", "profile", "email"],
    });
  });
});
