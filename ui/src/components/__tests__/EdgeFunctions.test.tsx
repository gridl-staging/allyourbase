import { vi, describe, it, expect, beforeEach } from "vitest";
import { screen, waitFor } from "@testing-library/react";
import { renderWithProviders } from "../../test-utils";
import userEvent from "@testing-library/user-event";
import { EdgeFunctions } from "../EdgeFunctions";
import {
  listEdgeFunctions,
  getEdgeFunction,
  deployEdgeFunction,
  updateEdgeFunction,
  deleteEdgeFunction,
  listEdgeFunctionLogs,
  invokeEdgeFunction,
} from "../../api";
import type {
  EdgeFunctionResponse,
  EdgeFunctionLogEntry,
  EdgeFunctionInvokeResponse,
} from "../../types";

vi.mock("../../api", () => ({
  listEdgeFunctions: vi.fn(),
  getEdgeFunction: vi.fn(),
  deployEdgeFunction: vi.fn(),
  updateEdgeFunction: vi.fn(),
  deleteEdgeFunction: vi.fn(),
  listEdgeFunctionLogs: vi.fn(),
  invokeEdgeFunction: vi.fn(),
  ApiError: class extends Error {
    status: number;
    constructor(status: number, message: string) {
      super(message);
      this.status = status;
    }
  },
}));

const mockAddToast = vi.fn();

vi.mock("../Toast", () => ({
  ToastContainer: () => null,
  useToast: () => ({
    toasts: [],
    addToast: mockAddToast,
    removeToast: vi.fn(),
  }),
}));

// Mock CodeMirror since jsdom doesn't support it
vi.mock("@uiw/react-codemirror", () => ({
  __esModule: true,
  default: ({ value, onChange, "data-testid": testId }: {
    value: string;
    onChange?: (val: string) => void;
    "data-testid"?: string;
  }) => (
    <textarea
      data-testid={testId || "codemirror-editor"}
      value={value}
      onChange={(e) => onChange?.(e.target.value)}
    />
  ),
}));

vi.mock("@codemirror/lang-javascript", () => ({
  javascript: () => [],
}));

const mockListEdgeFunctions = vi.mocked(listEdgeFunctions);
const mockGetEdgeFunction = vi.mocked(getEdgeFunction);
const mockDeployEdgeFunction = vi.mocked(deployEdgeFunction);
const mockUpdateEdgeFunction = vi.mocked(updateEdgeFunction);
const mockDeleteEdgeFunction = vi.mocked(deleteEdgeFunction);
const mockListEdgeFunctionLogs = vi.mocked(listEdgeFunctionLogs);
const mockInvokeEdgeFunction = vi.mocked(invokeEdgeFunction);

function makeFn(overrides: Partial<EdgeFunctionResponse> = {}): EdgeFunctionResponse {
  return {
    id: "ef_1",
    name: "hello-world",
    entryPoint: "handler",
    timeout: 5000000000, // 5s in nanoseconds
    public: true,
    source: 'export default function handler(req) { return { statusCode: 200, body: "Hello" }; }',
    compiledJs: "",
    lastInvokedAt: null,
    createdAt: "2026-02-01T00:00:00Z",
    updatedAt: "2026-02-01T00:00:00Z",
    ...overrides,
  };
}

function makeLog(overrides: Partial<EdgeFunctionLogEntry> = {}): EdgeFunctionLogEntry {
  return {
    id: "log_1",
    functionId: "ef_1",
    invocationId: "inv_1",
    status: "success",
    durationMs: 42,
    requestMethod: "GET",
    requestPath: "/hello-world",
    createdAt: "2026-02-01T12:00:00Z",
    ...overrides,
  };
}

function mockLogsForDetailThenLogsTab(logs: EdgeFunctionLogEntry[]) {
  mockListEdgeFunctionLogs
    .mockResolvedValueOnce(logs)
    .mockResolvedValueOnce(logs);
}

describe("EdgeFunctions", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  // --- List view ---

  it("shows loading state", () => {
    mockListEdgeFunctions.mockReturnValue(new Promise(() => {}));
    renderWithProviders(<EdgeFunctions />);
    expect(screen.getByText("Loading edge functions...")).toBeInTheDocument();
  });

  it("displays empty state when no functions", async () => {
    mockListEdgeFunctions.mockResolvedValueOnce([]);
    renderWithProviders(<EdgeFunctions />);
    await waitFor(() => {
      expect(screen.getByText("No edge functions deployed yet")).toBeInTheDocument();
    });
  });

  it("shows error state on API failure", async () => {
    mockListEdgeFunctions.mockRejectedValueOnce(new Error("Network error"));
    renderWithProviders(<EdgeFunctions />);
    await waitFor(() => {
      expect(screen.getByText("Network error")).toBeInTheDocument();
    });
  });

  it("renders function list with correct details", async () => {
    const fn1 = makeFn({ id: "ef_1", name: "hello-world", public: true });
    const fn2 = makeFn({ id: "ef_2", name: "auth-check", public: false });
    mockListEdgeFunctions.mockResolvedValueOnce([fn1, fn2]);
    renderWithProviders(<EdgeFunctions />);

    await waitFor(() => {
      expect(screen.getByText("hello-world")).toBeInTheDocument();
      expect(screen.getByText("auth-check")).toBeInTheDocument();
    });

    // Verify public/private badges
    expect(screen.getByTestId("fn-public-ef_1")).toHaveTextContent("Public");
    expect(screen.getByTestId("fn-public-ef_2")).toHaveTextContent("Private");
  });

  it("shows last invoked timestamps with never fallback", async () => {
    const invokedAt = "2026-02-03T12:34:56Z";
    const fn1 = makeFn({ id: "ef_1", name: "hello-world", lastInvokedAt: invokedAt });
    const fn2 = makeFn({ id: "ef_2", name: "auth-check", lastInvokedAt: null });
    mockListEdgeFunctions.mockResolvedValueOnce([fn1, fn2]);

    renderWithProviders(<EdgeFunctions />);

    await waitFor(() => {
      expect(screen.getByRole("columnheader", { name: "Last Invoked" })).toBeInTheDocument();
    });

    expect(screen.getByText(new Date(invokedAt).toLocaleString())).toBeInTheDocument();
    expect(screen.getByText("Never")).toBeInTheDocument();
  });

  it("shows page heading", async () => {
    mockListEdgeFunctions.mockResolvedValueOnce([]);
    renderWithProviders(<EdgeFunctions />);
    await waitFor(() => {
      expect(screen.getByRole("heading", { name: "Edge Functions" })).toBeInTheDocument();
    });
  });

  // --- Create function ---

  it("opens create dialog and deploys a new function", async () => {
    const user = userEvent.setup();
    mockListEdgeFunctions.mockResolvedValueOnce([]);
    const deployed = makeFn({ id: "ef_new", name: "new-fn" });
    mockDeployEdgeFunction.mockResolvedValueOnce(deployed);
    // After deploy, refresh list
    mockListEdgeFunctions.mockResolvedValueOnce([deployed]);

    renderWithProviders(<EdgeFunctions />);
    await waitFor(() => {
      expect(screen.getByText("No edge functions deployed yet")).toBeInTheDocument();
    });

    // Click create button
    await user.click(screen.getByRole("button", { name: /new function/i }));

    // Fill form
    await user.clear(screen.getByLabelText("Name"));
    await user.type(screen.getByLabelText("Name"), "new-fn");
    const editor = screen.getByTestId("codemirror-editor");
    await user.clear(editor);
    // Use fireEvent for text containing braces (userEvent parses {} as key descriptors)
    const { fireEvent } = await import("@testing-library/react");
    fireEvent.change(editor, { target: { value: "function handler() {}" } });

    // Submit
    await user.click(screen.getByRole("button", { name: /deploy/i }));

    await waitFor(() => {
      expect(mockDeployEdgeFunction).toHaveBeenCalledWith(
        expect.objectContaining({ name: "new-fn" }),
      );
    });
  });

  // --- Detail view ---

  it("navigates to function detail when clicking a function", async () => {
    const user = userEvent.setup();
    const fn1 = makeFn();
    mockListEdgeFunctions.mockResolvedValueOnce([fn1]);
    mockGetEdgeFunction.mockResolvedValueOnce(fn1);
    mockListEdgeFunctionLogs.mockResolvedValueOnce([]);

    renderWithProviders(<EdgeFunctions />);
    await waitFor(() => {
      expect(screen.getByText("hello-world")).toBeInTheDocument();
    });

    await user.click(screen.getByText("hello-world"));

    await waitFor(() => {
      expect(mockGetEdgeFunction).toHaveBeenCalledWith("ef_1");
    });
  });

  it("shows function source in detail view", async () => {
    const user = userEvent.setup();
    const fn1 = makeFn();
    mockListEdgeFunctions.mockResolvedValueOnce([fn1]);
    mockGetEdgeFunction.mockResolvedValueOnce(fn1);
    mockListEdgeFunctionLogs.mockResolvedValueOnce([]);

    renderWithProviders(<EdgeFunctions />);
    await waitFor(() => {
      expect(screen.getByText("hello-world")).toBeInTheDocument();
    });

    await user.click(screen.getByText("hello-world"));

    await waitFor(() => {
      expect(screen.getByTestId("codemirror-editor")).toHaveValue(fn1.source);
    });
  });

  // --- Update function ---

  it("updates function on save", async () => {
    const user = userEvent.setup();
    const fn1 = makeFn();
    mockListEdgeFunctions.mockResolvedValueOnce([fn1]);
    mockGetEdgeFunction.mockResolvedValueOnce(fn1);
    mockListEdgeFunctionLogs.mockResolvedValueOnce([]);
    const updated = makeFn({ source: "updated code" });
    mockUpdateEdgeFunction.mockResolvedValueOnce(updated);

    renderWithProviders(<EdgeFunctions />);
    await waitFor(() => {
      expect(screen.getByText("hello-world")).toBeInTheDocument();
    });

    await user.click(screen.getByText("hello-world"));

    await waitFor(() => {
      expect(screen.getByTestId("codemirror-editor")).toBeInTheDocument();
    });

    // Modify source in editor
    const editor = screen.getByTestId("codemirror-editor");
    await user.clear(editor);
    await user.type(editor, "updated code");

    // Save
    await user.click(screen.getByRole("button", { name: /save/i }));

    await waitFor(() => {
      expect(mockUpdateEdgeFunction).toHaveBeenCalledWith(
        "ef_1",
        expect.objectContaining({ source: "updated code" }),
      );
    });
  });

  // --- Delete function ---

  it("deletes function with confirmation", async () => {
    const user = userEvent.setup();
    const fn1 = makeFn();
    mockListEdgeFunctions.mockResolvedValueOnce([fn1]);
    mockGetEdgeFunction.mockResolvedValueOnce(fn1);
    mockListEdgeFunctionLogs.mockResolvedValueOnce([]);
    mockDeleteEdgeFunction.mockResolvedValueOnce(undefined);
    mockListEdgeFunctions.mockResolvedValueOnce([]);

    renderWithProviders(<EdgeFunctions />);
    await waitFor(() => {
      expect(screen.getByText("hello-world")).toBeInTheDocument();
    });

    await user.click(screen.getByText("hello-world"));

    await waitFor(() => {
      expect(screen.getByRole("button", { name: /delete/i })).toBeInTheDocument();
    });

    await user.click(screen.getByRole("button", { name: /delete/i }));

    // Confirm dialog
    await waitFor(() => {
      expect(screen.getByText(/are you sure/i)).toBeInTheDocument();
    });
    await user.click(screen.getByRole("button", { name: /confirm/i }));

    await waitFor(() => {
      expect(mockDeleteEdgeFunction).toHaveBeenCalledWith("ef_1");
    });
  });

  // --- Logs ---

  it("displays execution logs in detail view", async () => {
    const user = userEvent.setup();
    const fn1 = makeFn();
    const log1 = makeLog({ durationMs: 42, status: "success" });
    const log2 = makeLog({ id: "log_2", invocationId: "inv_2", status: "error", error: "timeout", durationMs: 5000 });
    mockListEdgeFunctions.mockResolvedValueOnce([fn1]);
    mockGetEdgeFunction.mockResolvedValueOnce(fn1);
    mockLogsForDetailThenLogsTab([log1, log2]);

    renderWithProviders(<EdgeFunctions />);
    await waitFor(() => {
      expect(screen.getByText("hello-world")).toBeInTheDocument();
    });

    await user.click(screen.getByText("hello-world"));

    // Switch to logs tab
    await waitFor(() => {
      expect(screen.getByRole("button", { name: /logs/i })).toBeInTheDocument();
    });
    await user.click(screen.getByRole("button", { name: /logs/i }));

    // Check log content
    await waitFor(() => {
      expect(screen.getByText("42ms")).toBeInTheDocument();
      expect(screen.getByText("5000ms")).toBeInTheDocument();
    });
  });

  // --- Invoke tester ---

  it("invokes function and shows response", async () => {
    const user = userEvent.setup();
    const fn1 = makeFn();
    mockListEdgeFunctions.mockResolvedValueOnce([fn1]);
    mockGetEdgeFunction.mockResolvedValueOnce(fn1);
    mockListEdgeFunctionLogs.mockResolvedValueOnce([]);

    const invokeResp: EdgeFunctionInvokeResponse = {
      statusCode: 200,
      body: '{"message":"hello"}',
    };
    mockInvokeEdgeFunction.mockResolvedValueOnce(invokeResp);
    // Mock the logs refresh after invocation
    mockListEdgeFunctionLogs.mockResolvedValueOnce([]);

    renderWithProviders(<EdgeFunctions />);
    await waitFor(() => {
      expect(screen.getByText("hello-world")).toBeInTheDocument();
    });

    await user.click(screen.getByText("hello-world"));

    // Switch to invoke tab
    await waitFor(() => {
      expect(screen.getByRole("button", { name: /invoke/i })).toBeInTheDocument();
    });
    await user.click(screen.getByRole("button", { name: /invoke/i }));

    await waitFor(() => {
      expect(screen.getByRole("button", { name: /send/i })).toBeInTheDocument();
    });

    // Click send to invoke
    await user.click(screen.getByRole("button", { name: /send/i }));

    await waitFor(() => {
      expect(mockInvokeEdgeFunction).toHaveBeenCalledWith(
        "ef_1",
        expect.objectContaining({ method: "GET" }),
      );
    });

    // Response should be visible
    await waitFor(() => {
      expect(screen.getByText("Response")).toBeInTheDocument();
      expect(screen.getByText('{"message":"hello"}')).toBeInTheDocument();
    });
  });

  // --- Invoke tester headers ---

  it("sends custom headers from invoke tester", async () => {
    const user = userEvent.setup();
    const fn1 = makeFn();
    mockListEdgeFunctions.mockResolvedValueOnce([fn1]);
    mockGetEdgeFunction.mockResolvedValueOnce(fn1);
    mockListEdgeFunctionLogs.mockResolvedValueOnce([]);

    const invokeResp: EdgeFunctionInvokeResponse = {
      statusCode: 200,
      body: '{"ok":true}',
    };
    mockInvokeEdgeFunction.mockResolvedValueOnce(invokeResp);
    mockListEdgeFunctionLogs.mockResolvedValueOnce([]);

    renderWithProviders(<EdgeFunctions />);
    await waitFor(() => {
      expect(screen.getByText("hello-world")).toBeInTheDocument();
    });

    await user.click(screen.getByText("hello-world"));

    // Switch to invoke tab
    await waitFor(() => {
      expect(screen.getByRole("button", { name: /invoke/i })).toBeInTheDocument();
    });
    await user.click(screen.getByRole("button", { name: /invoke/i }));

    // Add a custom header
    await waitFor(() => {
      expect(screen.getByRole("button", { name: /add header/i })).toBeInTheDocument();
    });
    await user.click(screen.getByRole("button", { name: /add header/i }));

    // Fill in header key/value
    const headerKeyInput = screen.getByPlaceholderText("Header-Name");
    const headerValueInput = screen.getByPlaceholderText("value");
    await user.type(headerKeyInput, "X-Custom-Auth");
    await user.type(headerValueInput, "Bearer token123");

    // Send the request
    await user.click(screen.getByRole("button", { name: /send/i }));

    await waitFor(() => {
      expect(mockInvokeEdgeFunction).toHaveBeenCalledWith(
        "ef_1",
        expect.objectContaining({
          method: "GET",
          headers: { "X-Custom-Auth": ["Bearer token123"] },
        }),
      );
    });

    // Verify response panel rendered (not just that mock was called)
    await waitFor(() => {
      expect(screen.getByText("Response")).toBeInTheDocument();
      expect(screen.getByText('{"ok":true}')).toBeInTheDocument();
    });
  });

  // --- Env vars editor ---

  it("shows env vars in detail view", async () => {
    const user = userEvent.setup();
    const fn1 = makeFn({ envVars: { API_KEY: "secret123", DB_URL: "postgres://..." } });
    mockListEdgeFunctions.mockResolvedValueOnce([fn1]);
    mockGetEdgeFunction.mockResolvedValueOnce(fn1);
    mockListEdgeFunctionLogs.mockResolvedValueOnce([]);

    renderWithProviders(<EdgeFunctions />);
    await waitFor(() => {
      expect(screen.getByText("hello-world")).toBeInTheDocument();
    });

    await user.click(screen.getByText("hello-world"));

    await waitFor(() => {
      expect(screen.getByDisplayValue("API_KEY")).toBeInTheDocument();
      expect(screen.getByDisplayValue("secret123")).toBeInTheDocument();
    });
  });

  // --- Back navigation ---

  it("navigates back to list from detail view", async () => {
    const user = userEvent.setup();
    const fn1 = makeFn();
    mockListEdgeFunctions.mockResolvedValueOnce([fn1]);
    mockGetEdgeFunction.mockResolvedValueOnce(fn1);
    mockListEdgeFunctionLogs.mockResolvedValueOnce([]);
    // Second list call when navigating back
    mockListEdgeFunctions.mockResolvedValueOnce([fn1]);

    renderWithProviders(<EdgeFunctions />);
    await waitFor(() => {
      expect(screen.getByText("hello-world")).toBeInTheDocument();
    });

    await user.click(screen.getByText("hello-world"));

    await waitFor(() => {
      expect(screen.getByRole("button", { name: /back/i })).toBeInTheDocument();
    });

    await user.click(screen.getByRole("button", { name: /back/i }));

    await waitFor(() => {
      expect(screen.getByRole("heading", { name: "Edge Functions" })).toBeInTheDocument();
    });
  });

  // --- Log expandable rows ---

  it("expands log row to show stdout and error on click", async () => {
    const user = userEvent.setup();
    const fn1 = makeFn();
    const logWithOutput = makeLog({
      id: "log_expand",
      stdout: "hello from stdout",
      error: "something went wrong",
      status: "error",
    });
    mockListEdgeFunctions.mockResolvedValueOnce([fn1]);
    mockGetEdgeFunction.mockResolvedValueOnce(fn1);
    mockLogsForDetailThenLogsTab([logWithOutput]);

    renderWithProviders(<EdgeFunctions />);
    await waitFor(() => expect(screen.getByText("hello-world")).toBeInTheDocument());
    await user.click(screen.getByText("hello-world"));

    // Switch to logs tab
    await waitFor(() => expect(screen.getByRole("button", { name: /logs/i })).toBeInTheDocument());
    await user.click(screen.getByRole("button", { name: /logs/i }));

    // Verify log row is visible
    await waitFor(() => expect(screen.getByText("42ms")).toBeInTheDocument());

    // stdout/error should NOT be visible before expanding
    expect(screen.queryByText("hello from stdout")).not.toBeInTheDocument();
    expect(screen.queryByText("something went wrong")).not.toBeInTheDocument();

    // Click to expand
    await user.click(screen.getByText("42ms"));

    // Now stdout and error should be visible
    await waitFor(() => {
      expect(screen.getByText("hello from stdout")).toBeInTheDocument();
      expect(screen.getByText("something went wrong")).toBeInTheDocument();
    });

    // Verify labels
    expect(screen.getByText("stdout:")).toBeInTheDocument();
    expect(screen.getByText("error:")).toBeInTheDocument();
  });

  // --- Empty logs state ---

  it("shows empty state when function has no logs", async () => {
    const user = userEvent.setup();
    const fn1 = makeFn();
    mockListEdgeFunctions.mockResolvedValueOnce([fn1]);
    mockGetEdgeFunction.mockResolvedValueOnce(fn1);
    mockLogsForDetailThenLogsTab([]);

    renderWithProviders(<EdgeFunctions />);
    await waitFor(() => expect(screen.getByText("hello-world")).toBeInTheDocument());
    await user.click(screen.getByText("hello-world"));

    await waitFor(() => expect(screen.getByRole("button", { name: /logs/i })).toBeInTheDocument());
    await user.click(screen.getByRole("button", { name: /logs/i }));

    await waitFor(() => {
      expect(screen.getByText("No execution logs yet.")).toBeInTheDocument();
    });
  });

  // --- Invoke with POST method and body ---

  it("sends POST request with body from invoke tester", async () => {
    const user = userEvent.setup();
    const fn1 = makeFn();
    mockListEdgeFunctions.mockResolvedValueOnce([fn1]);
    mockGetEdgeFunction.mockResolvedValueOnce(fn1);
    mockListEdgeFunctionLogs.mockResolvedValueOnce([]);

    const invokeResp: EdgeFunctionInvokeResponse = {
      statusCode: 201,
      body: '{"created":true}',
    };
    mockInvokeEdgeFunction.mockResolvedValueOnce(invokeResp);
    mockListEdgeFunctionLogs.mockResolvedValueOnce([]);

    renderWithProviders(<EdgeFunctions />);
    await waitFor(() => expect(screen.getByText("hello-world")).toBeInTheDocument());
    await user.click(screen.getByText("hello-world"));

    await waitFor(() => expect(screen.getByRole("button", { name: /invoke/i })).toBeInTheDocument());
    await user.click(screen.getByRole("button", { name: /invoke/i }));

    // Change method to POST
    await waitFor(() => expect(screen.getByLabelText("HTTP Method")).toBeInTheDocument());
    await user.selectOptions(screen.getByLabelText("HTTP Method"), "POST");

    // Body textarea should appear for POST
    await waitFor(() => expect(screen.getByPlaceholderText('{"key": "value"}')).toBeInTheDocument());
    const { fireEvent: fire } = await import("@testing-library/react");
    fire.change(screen.getByPlaceholderText('{"key": "value"}'), { target: { value: '{"name":"test"}' } });

    await user.click(screen.getByRole("button", { name: /send/i }));

    await waitFor(() => {
      expect(mockInvokeEdgeFunction).toHaveBeenCalledWith(
        "ef_1",
        expect.objectContaining({
          method: "POST",
          body: '{"name":"test"}',
        }),
      );
    });
  });

  // --- Invoke response status code display ---

  it("displays response status code badge after invoke", async () => {
    const user = userEvent.setup();
    const fn1 = makeFn();
    mockListEdgeFunctions.mockResolvedValueOnce([fn1]);
    mockGetEdgeFunction.mockResolvedValueOnce(fn1);
    mockListEdgeFunctionLogs.mockResolvedValueOnce([]);

    const invokeResp: EdgeFunctionInvokeResponse = {
      statusCode: 404,
      body: "not found",
    };
    mockInvokeEdgeFunction.mockResolvedValueOnce(invokeResp);
    mockListEdgeFunctionLogs.mockResolvedValueOnce([]); // post-invoke refresh

    renderWithProviders(<EdgeFunctions />);
    await waitFor(() => expect(screen.getByText("hello-world")).toBeInTheDocument());
    await user.click(screen.getByText("hello-world"));

    await waitFor(() => expect(screen.getByRole("button", { name: /invoke/i })).toBeInTheDocument());
    await user.click(screen.getByRole("button", { name: /invoke/i }));
    await waitFor(() => expect(screen.getByRole("button", { name: /send/i })).toBeInTheDocument());
    await user.click(screen.getByRole("button", { name: /send/i }));

    await waitFor(() => {
      expect(screen.getByText("404")).toBeInTheDocument();
      expect(screen.getByText("not found")).toBeInTheDocument();
    });
  });

  // --- Env var add and remove ---

  it("adds and removes environment variables", async () => {
    const user = userEvent.setup();
    const fn1 = makeFn({ envVars: {} });
    mockListEdgeFunctions.mockResolvedValueOnce([fn1]);
    mockGetEdgeFunction.mockResolvedValueOnce(fn1);
    mockListEdgeFunctionLogs.mockResolvedValueOnce([]);

    renderWithProviders(<EdgeFunctions />);
    await waitFor(() => expect(screen.getByText("hello-world")).toBeInTheDocument());
    await user.click(screen.getByText("hello-world"));

    // Should show empty env var message
    await waitFor(() => {
      expect(screen.getByText("No environment variables configured.")).toBeInTheDocument();
    });

    // Add a variable
    await user.click(screen.getByText("+ Add Variable"));

    // Empty message should be gone, inputs should appear
    expect(screen.queryByText("No environment variables configured.")).not.toBeInTheDocument();
    const keyInput = screen.getByPlaceholderText("KEY");
    const valueInput = screen.getByPlaceholderText("value");
    expect(keyInput).toBeInTheDocument();
    expect(valueInput).toBeInTheDocument();

    await user.type(keyInput, "MY_VAR");
    await user.type(valueInput, "my_value");

    // Verify inputs have correct values
    expect(keyInput).toHaveValue("MY_VAR");
    expect(valueInput).toHaveValue("my_value");

    // Now remove the variable
    await user.click(screen.getByTestId("env-remove-0"));

    // Empty message should reappear, inputs should be gone
    await waitFor(() => {
      expect(screen.getByText("No environment variables configured.")).toBeInTheDocument();
    });
    expect(screen.queryByPlaceholderText("KEY")).not.toBeInTheDocument();
  });

  // --- Deploy error handling ---

  it("shows error toast when deploy fails in create view", async () => {
    const user = userEvent.setup();
    mockListEdgeFunctions.mockResolvedValueOnce([]);
    mockDeployEdgeFunction.mockRejectedValueOnce(new Error("compile error: syntax error at line 3"));

    renderWithProviders(<EdgeFunctions />);
    await waitFor(() => expect(screen.getByText("No edge functions deployed yet")).toBeInTheDocument());

    await user.click(screen.getByRole("button", { name: /new function/i }));

    await user.clear(screen.getByLabelText("Name"));
    await user.type(screen.getByLabelText("Name"), "bad-fn");
    const editor = screen.getByTestId("codemirror-editor");
    const { fireEvent } = await import("@testing-library/react");
    fireEvent.change(editor, { target: { value: "invalid code {{{" } });

    await user.click(screen.getByRole("button", { name: /deploy/i }));

    // Error should be displayed inline and via toast
    await waitFor(() => {
      expect(screen.getByTestId("deploy-error")).toBeInTheDocument();
      expect(screen.getAllByText(/compile error: syntax error at line 3/)).toHaveLength(2);
    });

    // Should stay on create page (not navigate away)
    expect(screen.getByRole("heading", { name: /deploy new function/i })).toBeInTheDocument();
  });
});
