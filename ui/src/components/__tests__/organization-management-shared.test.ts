import { describe, expect, it } from "vitest";
import {
  normalizeOrgRole,
  normalizeTeamRole,
  teamMemberDraftKey,
  toErrorMessage,
} from "../organization-management-shared";

describe("organization management shared utilities", () => {
  describe("normalizeOrgRole", () => {
    it.each(["owner", "admin", "member", "viewer"] as const)(
      "accepts valid org role %s",
      (role) => {
        expect(normalizeOrgRole(role)).toBe(role);
      },
    );

    it("falls back to member for unknown role strings", () => {
      expect(normalizeOrgRole("superadmin")).toBe("member");
      expect(normalizeOrgRole("")).toBe("member");
    });
  });

  describe("normalizeTeamRole", () => {
    it.each(["lead", "member"] as const)(
      "accepts valid team role %s",
      (role) => {
        expect(normalizeTeamRole(role)).toBe(role);
      },
    );

    it("falls back to member for unknown role strings", () => {
      expect(normalizeTeamRole("owner")).toBe("member");
      expect(normalizeTeamRole("")).toBe("member");
    });
  });

  describe("teamMemberDraftKey", () => {
    it("concatenates teamId and userId with colon separator", () => {
      expect(teamMemberDraftKey("team-abc", "user-123")).toBe("team-abc:user-123");
    });
  });

  describe("toErrorMessage", () => {
    it("extracts message from Error objects", () => {
      expect(toErrorMessage(new Error("something broke"))).toBe("something broke");
    });

    it("stringifies non-Error values", () => {
      expect(toErrorMessage("raw string")).toBe("raw string");
      expect(toErrorMessage(42)).toBe("42");
    });

    it("handles null and undefined", () => {
      expect(toErrorMessage(null)).toBe("null");
      expect(toErrorMessage(undefined)).toBe("undefined");
    });
  });
});
