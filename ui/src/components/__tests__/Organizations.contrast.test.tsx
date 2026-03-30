import { describe, expect, it } from "vitest";
import { screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { mockFetchOrgList } from "./orgs-test-helpers";
import { renderWithProviders } from "../../test-utils";
import { Organizations } from "../Organizations";

describe("Organizations contrast regressions", () => {
  it("loading indicator uses WCAG AA compliant contrast token", () => {
    mockFetchOrgList.mockReturnValue(new Promise(() => {}));
    renderWithProviders(<Organizations />);

    const className = screen.getByText(/loading organizations/i).className;
    expect(className).toContain("text-gray-500");
    expect(className).not.toMatch(/\b(?:dark:)?text-gray-400\b/);
  });

  it("org list slug uses WCAG AA compliant contrast token", async () => {
    renderWithProviders(<Organizations />);
    await expect(screen.findByText("acme-inc")).resolves.toBeInTheDocument();

    const className = screen.getByText("acme-inc").className;
    expect(className).toContain("text-gray-500");
    expect(className).not.toMatch(/\b(?:dark:)?text-gray-400\b/);
  });

  it("member joined timestamp uses WCAG AA compliant contrast token", async () => {
    renderWithProviders(<Organizations />);
    const user = userEvent.setup();
    await screen.findByText("Acme Inc");
    await user.click(screen.getByText("Acme Inc"));
    await user.click(screen.getByText("Members"));
    const section = await screen.findByTestId("org-members-section");

    const className = within(section).getAllByText("2026-01-01T00:00:00Z")[0].className;
    expect(className).toContain("text-gray-500");
    expect(className).not.toMatch(/\b(?:dark:)?text-gray-400\b/);
  });

  it("team slug uses WCAG AA compliant contrast token", async () => {
    renderWithProviders(<Organizations />);
    const user = userEvent.setup();
    await screen.findByText("Acme Inc");
    await user.click(screen.getByText("Acme Inc"));
    await user.click(screen.getByText("Teams"));

    const className = (await screen.findByText("engineering")).className;
    expect(className).toContain("text-gray-500");
    expect(className).not.toMatch(/\b(?:dark:)?text-gray-400\b/);
  });
});
