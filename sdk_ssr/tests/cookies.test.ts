import { describe, expect, it } from "vitest";
import { clearSessionCookies, getSessionTokens, parseCookieHeader, serializeCookie } from "../src";

describe("cookie helpers", () => {
  it("parses cookie header and extracts session tokens", () => {
    const parsed = parseCookieHeader("a=1; ayb_token=t1; ayb_refresh_token=r1");
    expect(parsed.ayb_token).toBe("t1");
    expect(parsed.ayb_refresh_token).toBe("r1");

    const tokens = getSessionTokens("a=1; ayb_token=t1; ayb_refresh_token=r1");
    expect(tokens).toEqual({ token: "t1", refreshToken: "r1" });
  });

  it("serializes secure cookie defaults and clears cookies", () => {
    const cookie = serializeCookie("ayb_token", "abc", {
      secure: true,
      httpOnly: true,
      sameSite: "lax",
      path: "/",
      maxAge: 60,
    });

    expect(cookie).toContain("ayb_token=abc");
    expect(cookie).toContain("HttpOnly");
    expect(cookie).toContain("Secure");
    expect(cookie).toContain("SameSite=Lax");

    const clear = clearSessionCookies();
    expect(clear.length).toBe(2);
    expect(clear[0]).toContain("Max-Age=0");
  });
});
