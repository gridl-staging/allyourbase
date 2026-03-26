import { describe, expect, it, vi } from "vitest";
import type { APIRequestContext } from "@playwright/test";
import {
  authorizeOAuthRequest,
  exchangeOAuthAuthorizationCode,
  exchangeOAuthClientCredentials,
  exchangeOAuthRefreshToken,
  generateOAuthPKCEPair,
  parseOAuthRedirectURL,
  seedOAuthClient,
  submitOAuthConsent,
} from "../../browser-tests-unmocked/fixtures";

function okResponse(body: unknown, statusCode = 200) {
  return {
    ok: () => statusCode >= 200 && statusCode < 300,
    status: () => statusCode,
    json: async () => body,
    text: async () => JSON.stringify(body),
  };
}

describe("browser-unmocked oauth fixture helpers", () => {
  it("generateOAuthPKCEPair matches S256 base64url challenge contract", () => {
    const pkce = generateOAuthPKCEPair(
      "dBjftJeZ4CVP-mB92K27uhbUJU1p1r_wW1gFWFOEjXk",
    );
    expect(pkce.codeVerifier).toBe("dBjftJeZ4CVP-mB92K27uhbUJU1p1r_wW1gFWFOEjXk");
    expect(pkce.codeChallengeMethod).toBe("S256");
    expect(pkce.codeChallenge).toBe("E9Melhoa2OwvFrEMTJguCHaoeK1t8URWbuGJSstw-cM");
  });

  it("generateOAuthPKCEPair preserves an explicitly provided empty verifier", () => {
    const pkce = generateOAuthPKCEPair("");
    expect(pkce.codeVerifier).toBe("");
  });

  it("parseOAuthRedirectURL keeps code/state/error fields from callback URL", () => {
    const parsed = parseOAuthRedirectURL(
      "https://client.example.com/callback?code=auth-code-1&state=opaque-state&error=access_denied",
    );
    expect(parsed.code).toBe("auth-code-1");
    expect(parsed.state).toBe("opaque-state");
    expect(parsed.error).toBe("access_denied");
  });

  it("authorizeOAuthRequest forwards query fields and decodes consent prompt response", async () => {
    const request = {
      get: vi.fn(async () =>
        okResponse({
          requires_consent: true,
          client_id: "ayb_cid_1",
          client_name: "Test OAuth App",
          redirect_uri: "https://client.example.com/callback",
          scope: "readonly",
          state: "state-1",
          code_challenge: "challenge-1",
          code_challenge_method: "S256",
          allowed_tables: ["users", "profiles"],
        })),
    } as unknown as APIRequestContext;

    const result = await authorizeOAuthRequest(request, "admin-token", {
      responseType: "code",
      clientId: "ayb_cid_1",
      redirectURI: "https://client.example.com/callback",
      scope: "readonly",
      state: "state-1",
      codeChallenge: "challenge-1",
      codeChallengeMethod: "S256",
      allowedTables: ["users,profiles", "events"],
    });

    const callArgs = request.get.mock.calls[0];
    expect(callArgs[1]).toEqual({
      headers: {
        Accept: "application/json",
        Authorization: "Bearer admin-token",
      },
    });
    const requestURL = new URL(`http://local${callArgs[0] as string}`);
    expect(requestURL.pathname).toBe("/api/auth/authorize");
    expect(requestURL.searchParams.getAll("allowed_tables")).toEqual([
      "users,profiles",
      "events",
    ]);
    expect(result.kind).toBe("requires_consent");
    expect(result.requiresConsent).toBe(true);
    expect(result.allowedTables).toEqual(["users", "profiles"]);
  });

  it("authorizeOAuthRequest decodes redirect-ready response", async () => {
    const request = {
      get: vi.fn(async () =>
        okResponse({
          requires_consent: false,
          redirect_to:
            "https://client.example.com/callback?code=json-code-1&state=state-json",
        })),
    } as unknown as APIRequestContext;

    const result = await authorizeOAuthRequest(request, "admin-token", {
      responseType: "code",
      clientId: "ayb_cid_1",
      redirectURI: "https://client.example.com/callback",
      scope: "readonly",
      state: "state-json",
      codeChallenge: "challenge-json",
      codeChallengeMethod: "S256",
    });

    expect(result.kind).toBe("redirect_ready");
    expect(result.redirectTo).toContain("code=json-code-1");
  });

  it("submitOAuthConsent forwards JSON contract and returns redirect", async () => {
    const request = {
      post: vi.fn(async () =>
        okResponse({
          redirect_to:
            "https://client.example.com/callback?error=access_denied&state=state-deny",
        })),
    } as unknown as APIRequestContext;

    const result = await submitOAuthConsent(request, "admin-token", {
      decision: "deny",
      responseType: "code",
      clientId: "ayb_cid_1",
      redirectURI: "https://client.example.com/callback",
      scope: "readonly",
      state: "state-deny",
      codeChallenge: "challenge-1",
      codeChallengeMethod: "S256",
      allowedTables: ["users", "profiles"],
    });

    expect(request.post).toHaveBeenCalledWith("/api/auth/authorize/consent", {
      headers: {
        Accept: "application/json",
        Authorization: "Bearer admin-token",
        "Content-Type": "application/json",
      },
      data: {
        decision: "deny",
        response_type: "code",
        client_id: "ayb_cid_1",
        redirect_uri: "https://client.example.com/callback",
        scope: "readonly",
        state: "state-deny",
        code_challenge: "challenge-1",
        code_challenge_method: "S256",
        allowed_tables: ["users", "profiles"],
      },
    });
    expect(result.redirectTo).toContain("error=access_denied");
  });

  it("submitOAuthConsent omits allowed_tables when the caller omits it", async () => {
    const request = {
      post: vi.fn(async () =>
        okResponse({
          redirect_to: "https://client.example.com/callback?code=auth-code-2&state=state-2",
        })),
    } as unknown as APIRequestContext;

    await submitOAuthConsent(request, "admin-token", {
      decision: "approve",
      responseType: "code",
      clientId: "ayb_cid_1",
      redirectURI: "https://client.example.com/callback",
      scope: "readonly",
      state: "state-2",
      codeChallenge: "challenge-2",
      codeChallengeMethod: "S256",
    });

    const [, init] = request.post.mock.calls[0];
    expect(init.data).not.toHaveProperty("allowed_tables");
  });

  it("exchangeOAuthAuthorizationCode enforces code_verifier before request", async () => {
    const request = {
      post: vi.fn(async () => okResponse({})),
    } as unknown as APIRequestContext;

    await expect(
      exchangeOAuthAuthorizationCode(request, {
        code: "code-1",
        redirectURI: "https://client.example.com/callback",
        codeVerifier: "",
        clientAuth: {
          method: "body",
          clientId: "ayb_cid_1",
          clientSecret: "secret-1",
        },
      }),
    ).rejects.toThrow("codeVerifier is required");
    expect(request.post).not.toHaveBeenCalled();
  });

  it("exchangeOAuthAuthorizationCode forwards a whitespace-only verifier unchanged", async () => {
    const request = {
      post: vi.fn(async () =>
        okResponse({
          access_token: "ayb_at_access_ws",
          token_type: "Bearer",
          expires_in: 3600,
          refresh_token: "ayb_rt_refresh_ws",
          scope: "readonly",
        })),
    } as unknown as APIRequestContext;

    await exchangeOAuthAuthorizationCode(request, {
      code: "code-ws",
      redirectURI: "https://client.example.com/callback",
      codeVerifier: "   ",
      clientAuth: {
        method: "body",
        clientId: "ayb_cid_ws",
        clientSecret: "secret-ws",
      },
    });

    const [, init] = request.post.mock.calls[0];
    const form = new URLSearchParams(init.data);
    expect(form.get("code_verifier")).toBe("   ");
  });

  it("exchangeOAuthAuthorizationCode uses one explicit body-auth path", async () => {
    const request = {
      post: vi.fn(async () =>
        okResponse({
          access_token: "ayb_at_access_1",
          token_type: "Bearer",
          expires_in: 3600,
          refresh_token: "ayb_rt_refresh_1",
          scope: "readonly",
        })),
    } as unknown as APIRequestContext;

    await exchangeOAuthAuthorizationCode(request, {
      code: "code-1",
      redirectURI: "https://client.example.com/callback",
      codeVerifier: "verifier-1",
      clientAuth: {
        method: "body",
        clientId: "ayb_cid_1",
        clientSecret: "secret-1",
      },
    });

    const [, init] = request.post.mock.calls[0];
    const form = new URLSearchParams(init.data);
    expect(form.get("grant_type")).toBe("authorization_code");
    expect(form.get("code_verifier")).toBe("verifier-1");
    expect(form.get("client_id")).toBe("ayb_cid_1");
    expect(form.get("client_secret")).toBe("secret-1");
    expect(init.headers.Authorization).toBeUndefined();
    expect(init.headers["Content-Type"]).toBe("application/x-www-form-urlencoded");
  });

  it("exchangeOAuthClientCredentials keeps allowed_tables values unchanged", async () => {
    const request = {
      post: vi.fn(async () =>
        okResponse({
          access_token: "ayb_at_access_2",
          token_type: "Bearer",
          expires_in: 3600,
          scope: "readonly",
        })),
    } as unknown as APIRequestContext;

    await exchangeOAuthClientCredentials(request, {
      scope: "readonly",
      allowedTables: ["users,profiles", "events"],
      clientAuth: {
        method: "body",
        clientId: "ayb_cid_2",
        clientSecret: "secret-2",
      },
    });

    const [, init] = request.post.mock.calls[0];
    const form = new URLSearchParams(init.data);
    expect(form.get("grant_type")).toBe("client_credentials");
    expect(form.getAll("allowed_tables")).toEqual(["users,profiles", "events"]);
  });

  it("exchangeOAuthClientCredentials forwards a whitespace-only body client_id unchanged", async () => {
    const request = {
      post: vi.fn(async () =>
        okResponse({
          access_token: "ayb_at_access_body_ws",
          token_type: "Bearer",
          expires_in: 3600,
          scope: "readonly",
        })),
    } as unknown as APIRequestContext;

    await exchangeOAuthClientCredentials(request, {
      scope: "readonly",
      clientAuth: {
        method: "body",
        clientId: "   ",
        clientSecret: "secret-2",
      },
    });

    const [, init] = request.post.mock.calls[0];
    const form = new URLSearchParams(init.data);
    expect(form.get("client_id")).toBe("   ");
  });

  it("exchangeOAuthRefreshToken supports client basic auth without body fallback", async () => {
    const request = {
      post: vi.fn(async () =>
        okResponse({
          access_token: "ayb_at_access_3",
          token_type: "Bearer",
          expires_in: 3600,
          refresh_token: "ayb_rt_refresh_3",
          scope: "readonly",
        })),
    } as unknown as APIRequestContext;

    await exchangeOAuthRefreshToken(request, {
      refreshToken: "ayb_rt_old",
      clientAuth: {
        method: "basic",
        clientId: "ayb_cid_3",
        clientSecret: "secret-basic",
      },
    });

    const [, init] = request.post.mock.calls[0];
    const form = new URLSearchParams(init.data);
    expect(form.get("grant_type")).toBe("refresh_token");
    expect(form.get("client_id")).toBeNull();
    expect(form.get("client_secret")).toBeNull();
    expect(init.headers.Authorization).toMatch(/^Basic /);
  });

  it("exchangeOAuthRefreshToken forwards an empty basic-auth client secret unchanged", async () => {
    const request = {
      post: vi.fn(async () =>
        okResponse({
          access_token: "ayb_at_access_4",
          token_type: "Bearer",
          expires_in: 3600,
          refresh_token: "ayb_rt_refresh_4",
          scope: "readonly",
        })),
    } as unknown as APIRequestContext;

    await exchangeOAuthRefreshToken(request, {
      refreshToken: "ayb_rt_old_2",
      clientAuth: {
        method: "basic",
        clientId: "ayb_cid_4",
        clientSecret: "",
      },
    });

    const [, init] = request.post.mock.calls[0];
    const encodedCredentials = init.headers.Authorization.replace(/^Basic /, "");
    expect(Buffer.from(encodedCredentials, "base64").toString("utf8")).toBe("ayb_cid_4:");
  });

  it("seedOAuthClient returns one-time confidential secret and omits it for public", async () => {
    const request = {
      post: vi
        .fn()
        .mockImplementationOnce(async () =>
          okResponse({
            clientSecret: "secret-one-time",
            client: { id: "id-1", clientId: "cid-1", name: "confidential-client" },
          }, 201))
        .mockImplementationOnce(async () =>
          okResponse({
            client: { id: "id-2", clientId: "cid-2", name: "public-client" },
          }, 201)),
    } as unknown as APIRequestContext;

    const confidential = await seedOAuthClient(request, "admin-token", {
      appId: "app-1",
      name: "confidential-client",
      clientType: "confidential",
      redirectUris: ["https://client.example.com/callback"],
      scopes: ["readonly"],
    });
    const publicClient = await seedOAuthClient(request, "admin-token", {
      appId: "app-1",
      name: "public-client",
      clientType: "public",
      redirectUris: ["https://client.example.com/callback"],
      scopes: ["readonly"],
    });

    expect(confidential.clientSecret).toBe("secret-one-time");
    expect(publicClient.clientSecret).toBeUndefined();
  });
});
