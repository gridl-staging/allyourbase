import { describe, expect, it } from "vitest";
import {
  applyNextSetCookies,
  applySvelteKitSetCookies,
  nextCookieHeader,
  remixCookieHeader,
  remixSetCookiesHeaders,
  svelteKitCookieHeader,
} from "../src";

describe("framework adapters", () => {
  it("handles Next.js cookie extract/apply", () => {
    const req = { headers: new Headers({ cookie: "a=1; ayb_token=t1" }) };
    expect(nextCookieHeader(req)).toContain("ayb_token=t1");

    const res = { headers: new Headers() };
    applyNextSetCookies(res, ["a=1; Path=/", "b=2; Path=/"]);
    expect(res.headers.get("set-cookie")).toContain("a=1");
  });

  it("handles SvelteKit and Remix adapters", () => {
    const event = { request: new Request("http://localhost", { headers: { cookie: "a=1" } }) };
    expect(svelteKitCookieHeader(event)).toBe("a=1");

    const svelteHeaders = new Headers();
    applySvelteKitSetCookies(svelteHeaders, ["c=3; Path=/"]);
    expect(svelteHeaders.get("set-cookie")).toContain("c=3");

    const remixReq = new Request("http://localhost", { headers: { cookie: "x=9" } });
    expect(remixCookieHeader(remixReq)).toBe("x=9");

    const remixHeaders = remixSetCookiesHeaders(["z=1; Path=/"]);
    expect(remixHeaders.get("set-cookie")).toContain("z=1");
  });
});
