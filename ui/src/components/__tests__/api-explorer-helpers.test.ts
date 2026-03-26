import { describe, expect, it, beforeEach } from "vitest";
import {
  formatJson,
  generateCurl,
  generateJsSdk,
  loadHistory,
  saveHistory,
} from "../api-explorer-helpers";

describe("api-explorer-helpers", () => {
  beforeEach(() => {
    localStorage.clear();
  });

  it("formats JSON and preserves invalid JSON", () => {
    expect(formatJson('{"a":1}')).toContain('\n  "a": 1\n');
    expect(formatJson("not json")).toBe("not json");
  });

  it("generates cURL and JS SDK snippets", () => {
    expect(generateCurl("POST", "https://example.test/api/collections/posts", '{"title":"A"}')).toContain("-d '{\"title\":\"A\"}'");
    expect(generateJsSdk("GET", "/api/collections/posts?perPage=20")).toContain("ayb.records.list");
    expect(generateJsSdk("POST", "/api/rpc/get_total", "{\"user_id\":\"u1\"}")).toContain("ayb.rpc");
  });

  it("persists and loads history entries", () => {
    saveHistory([
      {
        method: "GET",
        path: "/api/collections/posts",
        status: 200,
        durationMs: 10,
        timestamp: "2026-03-13T00:00:00Z",
      },
    ]);

    const history = loadHistory();
    expect(history).toHaveLength(1);
    expect(history[0].path).toBe("/api/collections/posts");
  });
});
