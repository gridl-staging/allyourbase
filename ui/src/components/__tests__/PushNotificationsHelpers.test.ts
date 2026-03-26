import { describe, expect, it } from "vitest";
import {
  parsePushDataJSON,
  previewDeviceToken,
  providerBadgeClass,
  statusBadgeClass,
} from "../push-notifications/helpers";

describe("push notification helpers", () => {
  it("parses empty data payload as an empty object", () => {
    expect(parsePushDataJSON("   ")).toEqual({ data: {}, error: null });
  });

  it("rejects invalid JSON payloads", () => {
    expect(parsePushDataJSON("{oops")).toEqual({
      data: null,
      error: "Data must be valid JSON.",
    });
  });

  it("rejects non-object payloads", () => {
    expect(parsePushDataJSON("[]")).toEqual({
      data: null,
      error: "Data must be a JSON object.",
    });
  });

  it("rejects non-string values", () => {
    expect(parsePushDataJSON('{"count":1}')).toEqual({
      data: null,
      error: "Data value for count must be a string.",
    });
  });

  it("keeps short tokens unchanged", () => {
    expect(previewDeviceToken("short-token")).toBe("short-token");
  });

  it("shortens long tokens with stable prefix/suffix", () => {
    expect(previewDeviceToken("1234567890abcdefghij")).toBe("1234567890...efghij");
  });

  it("maps provider and status badges", () => {
    expect(providerBadgeClass("fcm")).toContain("bg-orange-100");
    expect(providerBadgeClass("apns")).toContain("bg-blue-100");
    expect(statusBadgeClass("pending")).toContain("bg-yellow-100");
    expect(statusBadgeClass("sent")).toContain("bg-green-100");
    expect(statusBadgeClass("failed")).toContain("bg-red-100");
  });
});
