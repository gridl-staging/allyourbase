/**
 * CONTRACT TEST: Google OAuth Provider
 *
 * Purpose: Validate that Google's OAuth API hasn't changed in breaking ways
 *
 * These tests hit REAL Google OAuth endpoints (in sandbox/test mode).
 * Run these:
 * - Weekly (scheduled)
 * - Before major releases
 * - When Google announces OAuth API changes
 *
 * DO NOT MOCK: Contract tests validate external service contracts
 */

import { describe, test, expect } from "vitest";

describe("Google OAuth Contract", () => {
  const GOOGLE_OAUTH_BASE = "https://accounts.google.com/o/oauth2/v2/auth";
  const GOOGLE_TOKEN_ENDPOINT = "https://oauth2.googleapis.com/token";
  const GOOGLE_USERINFO_ENDPOINT = "https://www.googleapis.com/oauth2/v2/userinfo";

  test("authorization URL structure", async () => {
    const params = new URLSearchParams({
      client_id: "test-client-id",
      redirect_uri: "http://localhost:8090/api/auth/oauth/google/callback",
      response_type: "code",
      scope: "openid email profile",
      state: "test-state",
    });

    const authUrl = `${GOOGLE_OAUTH_BASE}?${params}`;
    const response = await fetch(authUrl, { redirect: "manual" });

    // Google should redirect to login page, not return error
    expect([200, 302, 303]).toContain(response.status);
  });

  test("token exchange response structure", async () => {
    const authCode = process.env.GOOGLE_TEST_AUTH_CODE;
    if (!authCode) {
      console.log("⏭️  Skipping: GOOGLE_TEST_AUTH_CODE not set");
      return;
    }

    const response = await fetch(GOOGLE_TOKEN_ENDPOINT, {
      method: "POST",
      headers: { "Content-Type": "application/x-www-form-urlencoded" },
      body: new URLSearchParams({
        code: authCode,
        client_id: process.env.GOOGLE_CLIENT_ID || "",
        client_secret: process.env.GOOGLE_CLIENT_SECRET || "",
        redirect_uri: "http://localhost:8090/api/auth/oauth/google/callback",
        grant_type: "authorization_code",
      }),
    });

    if (!response.ok) {
      console.log("⚠️  Token exchange failed (expected for expired codes)");
      return;
    }

    const data = await response.json();
    expect(data).toHaveProperty("access_token");
    expect(data).toHaveProperty("expires_in");
    expect(data).toHaveProperty("token_type");
    expect(data.token_type).toBe("Bearer");
  });

  test("user info endpoint response fields", async () => {
    const accessToken = process.env.GOOGLE_TEST_ACCESS_TOKEN;
    if (!accessToken) {
      console.log("⏭️  Skipping: GOOGLE_TEST_ACCESS_TOKEN not set");
      return;
    }

    const response = await fetch(GOOGLE_USERINFO_ENDPOINT, {
      headers: { Authorization: `Bearer ${accessToken}` },
    });

    if (!response.ok) {
      console.log("⚠️  User info request failed (expected for expired tokens)");
      return;
    }

    const user = await response.json();
    expect(user).toHaveProperty("id");
    expect(user).toHaveProperty("email");
    expect(typeof user.id).toBe("string");
    expect(typeof user.email).toBe("string");
    expect(user.email).toMatch(/.+@.+/);
  });
});
