/**
 * Shared test utilities for UI component tests.
 *
 * Mock class for ApiError — used in vi.mock() factory functions
 * so tests don't need to duplicate the class definition.
 */
export class MockApiError extends Error {
  status: number;
  constructor(status: number, message: string) {
    super(message);
    this.status = status;
  }
}
