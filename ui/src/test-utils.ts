/**
 * Shared test utilities for UI component tests.
 *
 * Mock class for ApiError — used in vi.mock() factory functions
 * so tests don't need to duplicate the class definition.
 */
import { createElement, type ReactElement } from "react";
import { render } from "@testing-library/react";
import { expect } from "vitest";
import { ThemeProvider } from "./components/ThemeProvider";
import { ToastProvider } from "./components/ToastProvider";

export class MockApiError extends Error {
  status: number;
  retryAfterSeconds?: number;
  constructor(status: number, message: string, retryAfterSeconds?: number) {
    super(message);
    this.status = status;
    this.retryAfterSeconds = retryAfterSeconds;
  }
}

export function renderWithProviders(ui: ReactElement) {
  return render(
    createElement(
      ThemeProvider,
      null,
      createElement(ToastProvider, null, ui),
    ),
  );
}

// Shared WCAG token assertion so contrast-focused tests stay aligned on the
// approved muted text token and reject the older lower-contrast variant.
export function expectWcagContrastToken(className: string) {
  expect(className).toContain("text-gray-500");
  expect(className).not.toMatch(/\b(?:dark:)?text-gray-400\b/);
}
