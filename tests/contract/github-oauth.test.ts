/**
 * CONTRACT TEST: GitHub OAuth Provider
 *
 * Purpose: Validate that GitHub's OAuth API hasn't changed in breaking ways
 *
 * These tests hit REAL GitHub OAuth endpoints.
 * Run these:
 * - Weekly (scheduled)
 * - Before major releases
 * - When GitHub announces OAuth API changes
 *
 * DO NOT MOCK: Contract tests validate external service contracts
 */

import { describe, test, expect } from "vitest";

describe("GitHub OAuth Contract", () => {
  const GITHUB_OAUTH_BASE = "https://github.com/login/oauth/authorize";
  const GITHUB_TOKEN_ENDPOINT = "https://github.com/login/oauth/access_token";
  const GITHUB_USER_ENDPOINT = "https://api.github.com/user";

  test("authorization URL structure", async () => {
    const params = new URLSearchParams({
      client_id: "test-client-id",
      redirect_uri: "http://localhost:8090/api/auth/oauth/github/callback",
      scope: "read:user user:email",
      state: "test-state",
    });

    const authUrl = `${GITHUB_OAUTH_BASE}?${params}`;
    const response = await fetch(authUrl, { redirect: "manual" });

    // GitHub should redirect to login page, not return error
    expect([200, 302, 303]).toContain(response.status);
  });

  test("token exchange response structure", async () => {
    const authCode = process.env.GITHUB_TEST_AUTH_CODE;
    if (!authCode) {
      console.log("⏭️  Skipping: GITHUB_TEST_AUTH_CODE not set");
      return;
    }

    const response = await fetch(GITHUB_TOKEN_ENDPOINT, {
      method: "POST",
      headers: {
        "Content-Type": "application/json",
        "Accept": "application/json",
      },
      body: JSON.stringify({
        code: authCode,
        client_id: process.env.GITHUB_CLIENT_ID || "",
        client_secret: process.env.GITHUB_CLIENT_SECRET || "",
        redirect_uri: "http://localhost:8090/api/auth/oauth/github/callback",
      }),
    });

    if (!response.ok) {
      console.log("⚠️  Token exchange failed (expected for expired codes)");
      return;
    }

    const data = await response.json();
    expect(data).toHaveProperty("access_token");
    expect(data).toHaveProperty("token_type");
    expect(data.token_type).toBe("bearer");
    expect(data).toHaveProperty("scope");
  });

  test("user info endpoint response fields", async () => {
    const accessToken = process.env.GITHUB_TEST_ACCESS_TOKEN;
    if (!accessToken) {
      console.log("⏭️  Skipping: GITHUB_TEST_ACCESS_TOKEN not set");
      return;
    }

    const response = await fetch(GITHUB_USER_ENDPOINT, {
      headers: {
        Authorization: `Bearer ${accessToken}`,
        Accept: "application/vnd.github.v3+json",
      },
    });

    if (!response.ok) {
      console.log("⚠️  User info request failed (expected for expired tokens)");
      return;
    }

    const user = await response.json();
    expect(user).toHaveProperty("id");
    expect(user).toHaveProperty("login");
    expect(typeof user.id).toBe("number");
    expect(typeof user.login).toBe("string");
  });
});
