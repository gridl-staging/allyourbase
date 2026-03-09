import { vi, describe, it, expect, beforeEach } from "vitest";
import { render, screen, waitFor, act } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { ApiExplorer } from "../ApiExplorer";
import { executeApiExplorer } from "../../api";
import type { SchemaCache, ApiExplorerResponse } from "../../types";

vi.mock("../../api", () => ({
  executeApiExplorer: vi.fn(),
  ApiError: class extends Error {
    status: number;
    constructor(status: number, message: string) {
      super(message);
      this.status = status;
    }
  },
}));

const mockExecute = vi.mocked(executeApiExplorer);

function makeSchema(tableNames: string[] = ["posts", "users"]): SchemaCache {
  const tables: SchemaCache["tables"] = {};
  for (const name of tableNames) {
    tables[`public.${name}`] = {
      schema: "public",
      name,
      kind: "table",
      columns: [
        {
          name: "id",
          position: 1,
          type: "uuid",
          nullable: false,
          isPrimaryKey: true,
          jsonType: "string",
        },
        {
          name: "title",
          position: 2,
          type: "text",
          nullable: true,
          isPrimaryKey: false,
          jsonType: "string",
        },
      ],
      primaryKey: ["id"],
      foreignKeys: [],
    };
  }
  return { tables, schemas: ["public"], builtAt: "2026-02-10T12:00:00Z" };
}

function makeResponse(overrides: Partial<ApiExplorerResponse> = {}): ApiExplorerResponse {
  return {
    status: 200,
    statusText: "OK",
    headers: { "content-type": "application/json" },
    body: JSON.stringify({ items: [], page: 1, perPage: 20, totalItems: 0, totalPages: 0 }),
    durationMs: 42,
    ...overrides,
  };
}

describe("ApiExplorer", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    localStorage.clear();
  });

  it("renders the API Explorer with title", () => {
    render(<ApiExplorer schema={makeSchema()} />);
    expect(screen.getByText("API Explorer")).toBeInTheDocument();
  });

  it("renders method selector with GET selected by default", () => {
    render(<ApiExplorer schema={makeSchema()} />);
    const select = screen.getByLabelText("HTTP method") as HTMLSelectElement;
    expect(select.value).toBe("GET");
  });

  it("renders path input with collections prefix", () => {
    render(<ApiExplorer schema={makeSchema()} />);
    const input = screen.getByLabelText("Request path") as HTMLInputElement;
    expect(input.value).toBe("/api/collections/");
  });

  it("renders collection quick-select buttons from schema", () => {
    render(<ApiExplorer schema={makeSchema(["posts", "comments"])} />);
    expect(screen.getByText("posts")).toBeInTheDocument();
    expect(screen.getByText("comments")).toBeInTheDocument();
  });

  it("clicking collection button sets the path", async () => {
    render(<ApiExplorer schema={makeSchema(["posts"])} />);
    const user = userEvent.setup();
    await user.click(screen.getByText("posts"));

    const input = screen.getByLabelText("Request path") as HTMLInputElement;
    expect(input.value).toBe("/api/collections/posts");
  });

  it("shows Send button and empty state", () => {
    render(<ApiExplorer schema={makeSchema()} />);
    expect(screen.getByText("Send")).toBeInTheDocument();
    expect(screen.getByText("Send a request to see the response")).toBeInTheDocument();
  });

  it("executes request and displays response", async () => {
    mockExecute.mockResolvedValueOnce(
      makeResponse({
        status: 200,
        statusText: "OK",
        body: JSON.stringify({ items: [{ id: "1", title: "Hello" }] }),
        durationMs: 55,
      }),
    );
    render(<ApiExplorer schema={makeSchema()} />);

    const user = userEvent.setup();
    // Set a path
    const input = screen.getByLabelText("Request path") as HTMLInputElement;
    await user.clear(input);
    await user.type(input, "/api/collections/posts");

    await user.click(screen.getByText("Send"));

    await waitFor(() => {
      expect(screen.getByText("200 OK")).toBeInTheDocument();
      expect(screen.getByText("55ms")).toBeInTheDocument();
    });

    expect(mockExecute).toHaveBeenCalledWith("GET", "/api/collections/posts", undefined);
  });

  it("shows error response status", async () => {
    mockExecute.mockResolvedValueOnce(
      makeResponse({
        status: 404,
        statusText: "Not Found",
        body: JSON.stringify({ code: 404, message: "collection not found" }),
        durationMs: 12,
      }),
    );
    render(<ApiExplorer schema={makeSchema()} />);

    const user = userEvent.setup();
    const input = screen.getByLabelText("Request path") as HTMLInputElement;
    await user.clear(input);
    await user.type(input, "/api/collections/nonexistent");
    await user.click(screen.getByText("Send"));

    await waitFor(() => {
      expect(screen.getByText("404 Not Found")).toBeInTheDocument();
    });
  });

  it("shows network error in error display", async () => {
    mockExecute.mockRejectedValueOnce(new Error("Failed to fetch"));
    render(<ApiExplorer schema={makeSchema()} />);

    const user = userEvent.setup();
    const input = screen.getByLabelText("Request path") as HTMLInputElement;
    await user.clear(input);
    await user.type(input, "/api/collections/posts");
    await user.click(screen.getByText("Send"));

    await waitFor(() => {
      expect(screen.getByText("Failed to fetch")).toBeInTheDocument();
    });
  });

  it("shows body editor for POST method", async () => {
    render(<ApiExplorer schema={makeSchema()} />);

    const user = userEvent.setup();
    await user.selectOptions(screen.getByLabelText("HTTP method"), "POST");

    expect(screen.getByLabelText("Request body")).toBeInTheDocument();
  });

  it("shows body editor for PATCH method", async () => {
    render(<ApiExplorer schema={makeSchema()} />);

    const user = userEvent.setup();
    await user.selectOptions(screen.getByLabelText("HTTP method"), "PATCH");

    expect(screen.getByLabelText("Request body")).toBeInTheDocument();
  });

  it("hides body editor for DELETE method", async () => {
    render(<ApiExplorer schema={makeSchema()} />);

    const user = userEvent.setup();
    await user.selectOptions(screen.getByLabelText("HTTP method"), "DELETE");

    expect(screen.queryByLabelText("Request body")).not.toBeInTheDocument();
  });

  it("sends body with POST request", async () => {
    mockExecute.mockResolvedValueOnce(
      makeResponse({ status: 201, statusText: "Created" }),
    );
    render(<ApiExplorer schema={makeSchema()} />);

    const user = userEvent.setup();
    await user.selectOptions(screen.getByLabelText("HTTP method"), "POST");

    const input = screen.getByLabelText("Request path") as HTMLInputElement;
    await user.clear(input);
    await user.type(input, "/api/collections/posts");

    const bodyInput = screen.getByLabelText("Request body") as HTMLTextAreaElement;
    // Use fireEvent to set value directly (userEvent.type has special char issues with braces)
    await user.clear(bodyInput);
    // Set value directly via the DOM to avoid userEvent brace escaping
    await act(async () => {
      Object.getOwnPropertyDescriptor(
        HTMLTextAreaElement.prototype,
        "value",
      )!.set!.call(bodyInput, '{"title":"New Post"}');
      bodyInput.dispatchEvent(new Event("input", { bubbles: true }));
    });

    await user.click(screen.getByText("Send"));

    await waitFor(() => {
      expect(mockExecute).toHaveBeenCalledWith(
        "POST",
        "/api/collections/posts",
        '{"title":"New Post"}',
      );
    });
  });

  it("toggles query parameters panel", async () => {
    render(<ApiExplorer schema={makeSchema()} />);

    // Query params panel should be hidden by default
    expect(screen.queryByLabelText("filter")).not.toBeInTheDocument();

    const user = userEvent.setup();
    await user.click(screen.getByText("Query Parameters"));

    // Now visible
    expect(screen.getByLabelText("filter")).toBeInTheDocument();
    expect(screen.getByLabelText("sort")).toBeInTheDocument();
    expect(screen.getByLabelText("page")).toBeInTheDocument();
    expect(screen.getByLabelText("perPage")).toBeInTheDocument();
    expect(screen.getByLabelText("fields")).toBeInTheDocument();
    expect(screen.getByLabelText("expand")).toBeInTheDocument();
    expect(screen.getByLabelText("search")).toBeInTheDocument();
  });

  it("includes query params in request path", async () => {
    mockExecute.mockResolvedValueOnce(makeResponse());
    render(<ApiExplorer schema={makeSchema()} />);

    const user = userEvent.setup();

    // Set path
    const input = screen.getByLabelText("Request path") as HTMLInputElement;
    await user.clear(input);
    await user.type(input, "/api/collections/posts");

    // Open params panel
    await user.click(screen.getByText("Query Parameters"));

    // Set filter
    await user.type(screen.getByLabelText("filter"), "status='active'");

    // Set sort
    await user.type(screen.getByLabelText("sort"), "-created_at");

    await user.click(screen.getByText("Send"));

    await waitFor(() => {
      expect(mockExecute).toHaveBeenCalledWith(
        "GET",
        expect.stringContaining("filter="),
        undefined,
      );
      const calledPath = mockExecute.mock.calls[0][1];
      expect(calledPath).toContain("filter=status%3D%27active%27");
      expect(calledPath).toContain("sort=-created_at");
    });
  });

  it("generates curl snippet after response", async () => {
    mockExecute.mockResolvedValueOnce(makeResponse());
    render(<ApiExplorer schema={makeSchema()} />);

    const user = userEvent.setup();
    const input = screen.getByLabelText("Request path") as HTMLInputElement;
    await user.clear(input);
    await user.type(input, "/api/collections/posts");
    await user.click(screen.getByText("Send"));

    await waitFor(() => {
      expect(screen.getByText("cURL")).toBeInTheDocument();
      expect(screen.getByText("JS SDK")).toBeInTheDocument();
    });

    // cURL tab should be active by default and show curl command
    const snippetPre = screen.getByText(/curl -X GET/);
    expect(snippetPre).toBeInTheDocument();
    // Verify the URL is included in the snippet
    expect(snippetPre.textContent).toContain("/api/collections/posts");
  });

  it("switches to JS SDK snippet tab", async () => {
    mockExecute.mockResolvedValueOnce(makeResponse());
    render(<ApiExplorer schema={makeSchema()} />);

    const user = userEvent.setup();
    const input = screen.getByLabelText("Request path") as HTMLInputElement;
    await user.clear(input);
    await user.type(input, "/api/collections/posts");
    await user.click(screen.getByText("Send"));

    await waitFor(() => {
      expect(screen.getByText("JS SDK")).toBeInTheDocument();
    });

    await user.click(screen.getByText("JS SDK"));

    await waitFor(() => {
      expect(screen.getByText(/ayb\.records\.list/)).toBeInTheDocument();
    });
  });

  it("saves request to history on success", async () => {
    mockExecute.mockResolvedValueOnce(makeResponse({ durationMs: 42 }));
    render(<ApiExplorer schema={makeSchema()} />);

    const user = userEvent.setup();
    const input = screen.getByLabelText("Request path") as HTMLInputElement;
    await user.clear(input);
    await user.type(input, "/api/collections/posts");
    await user.click(screen.getByText("Send"));

    await waitFor(() => {
      expect(screen.getByText("200 OK")).toBeInTheDocument();
    });

    // History should show count
    expect(screen.getByText("History (1)")).toBeInTheDocument();
  });

  it("shows history panel when clicked", async () => {
    mockExecute.mockResolvedValueOnce(makeResponse());
    render(<ApiExplorer schema={makeSchema()} />);

    const user = userEvent.setup();
    const input = screen.getByLabelText("Request path") as HTMLInputElement;
    await user.clear(input);
    await user.type(input, "/api/collections/posts");
    await user.click(screen.getByText("Send"));

    await waitFor(() => {
      expect(screen.getByText("200 OK")).toBeInTheDocument();
    });

    await user.click(screen.getByText("History (1)"));

    expect(screen.getByText("Recent Requests")).toBeInTheDocument();
    expect(screen.getByText("Clear")).toBeInTheDocument();
  });

  it("clears history when clear button clicked", async () => {
    mockExecute.mockResolvedValueOnce(makeResponse());
    render(<ApiExplorer schema={makeSchema()} />);

    const user = userEvent.setup();
    const input = screen.getByLabelText("Request path") as HTMLInputElement;
    await user.clear(input);
    await user.type(input, "/api/collections/posts");
    await user.click(screen.getByText("Send"));

    await waitFor(() => {
      expect(screen.getByText("History (1)")).toBeInTheDocument();
    });

    await user.click(screen.getByText("History (1)"));
    await user.click(screen.getByText("Clear"));

    expect(screen.getByText("History (0)")).toBeInTheDocument();
  });

  it("disables Send button when path is empty", async () => {
    render(<ApiExplorer schema={makeSchema()} />);

    const user = userEvent.setup();
    const input = screen.getByLabelText("Request path") as HTMLInputElement;
    await user.clear(input);

    const sendBtn = screen.getByText("Send");
    expect(sendBtn).toBeDisabled();
  });

  it("shows 'Sending...' while loading", async () => {
    mockExecute.mockReturnValue(new Promise(() => {})); // never resolves
    render(<ApiExplorer schema={makeSchema()} />);

    const user = userEvent.setup();
    const input = screen.getByLabelText("Request path") as HTMLInputElement;
    await user.clear(input);
    await user.type(input, "/api/collections/posts");
    await user.click(screen.getByText("Send"));

    expect(screen.getByText("Sending...")).toBeInTheDocument();
  });

  it("formats JSON response body", async () => {
    mockExecute.mockResolvedValueOnce(
      makeResponse({
        body: '{"id":"1","title":"Hello World"}',
      }),
    );
    render(<ApiExplorer schema={makeSchema()} />);

    const user = userEvent.setup();
    const input = screen.getByLabelText("Request path") as HTMLInputElement;
    await user.clear(input);
    await user.type(input, "/api/collections/posts");
    await user.click(screen.getByText("Send"));

    await waitFor(() => {
      // Should be formatted with indentation
      expect(screen.getByText(/Response Body/)).toBeInTheDocument();
      expect(screen.getByText(/"Hello World"/)).toBeInTheDocument();
    });
  });

  it("shows response size in bytes", async () => {
    const body = JSON.stringify({ items: [] });
    mockExecute.mockResolvedValueOnce(makeResponse({ body }));
    render(<ApiExplorer schema={makeSchema()} />);

    const user = userEvent.setup();
    const input = screen.getByLabelText("Request path") as HTMLInputElement;
    await user.clear(input);
    await user.type(input, "/api/collections/posts");
    await user.click(screen.getByText("Send"));

    await waitFor(() => {
      const expectedBytes = new TextEncoder().encode(body).length;
      expect(screen.getByText(`${expectedBytes} bytes`)).toBeInTheDocument();
    });
  });

  it("shows schema-prefixed collection for non-public schema", () => {
    const schema = makeSchema([]);
    schema.tables["myschema.tasks"] = {
      schema: "myschema",
      name: "tasks",
      kind: "table",
      columns: [],
      primaryKey: ["id"],
    };
    render(<ApiExplorer schema={schema} />);

    expect(screen.getByText("myschema.tasks")).toBeInTheDocument();
  });

  it("keyboard shortcut text shows on the page", () => {
    render(<ApiExplorer schema={makeSchema()} />);
    // The shortcut hint should be visible
    expect(screen.getByText(/Enter to/)).toBeInTheDocument();
  });

  it("hides body editor for GET method", () => {
    render(<ApiExplorer schema={makeSchema()} />);
    // GET is default - no body editor
    expect(screen.queryByLabelText("Request body")).not.toBeInTheDocument();
  });

  it("switching method toggles body editor visibility", async () => {
    render(<ApiExplorer schema={makeSchema()} />);

    const user = userEvent.setup();

    // GET: no body
    expect(screen.queryByLabelText("Request body")).not.toBeInTheDocument();

    // POST: body
    await user.selectOptions(screen.getByLabelText("HTTP method"), "POST");
    expect(screen.getByLabelText("Request body")).toBeInTheDocument();

    // Back to GET: no body
    await user.selectOptions(screen.getByLabelText("HTTP method"), "GET");
    expect(screen.queryByLabelText("Request body")).not.toBeInTheDocument();
  });

  it("cURL snippet includes correct method", async () => {
    mockExecute.mockResolvedValueOnce(
      makeResponse({ status: 201, statusText: "Created" }),
    );
    render(<ApiExplorer schema={makeSchema()} />);

    const user = userEvent.setup();
    await user.selectOptions(screen.getByLabelText("HTTP method"), "POST");
    const input = screen.getByLabelText("Request path") as HTMLInputElement;
    await user.clear(input);
    await user.type(input, "/api/collections/posts");
    await user.click(screen.getByText("Send"));

    await waitFor(() => {
      expect(screen.getByText(/curl -X POST/)).toBeInTheDocument();
    });
  });

  it("JS SDK snippet shows delete method for DELETE", async () => {
    mockExecute.mockResolvedValueOnce(
      makeResponse({ status: 204, statusText: "No Content" }),
    );
    render(<ApiExplorer schema={makeSchema()} />);

    const user = userEvent.setup();
    await user.selectOptions(screen.getByLabelText("HTTP method"), "DELETE");
    const input = screen.getByLabelText("Request path") as HTMLInputElement;
    await user.clear(input);
    await user.type(input, "/api/collections/posts/123");
    await user.click(screen.getByText("Send"));

    await waitFor(() => {
      expect(screen.getByText("JS SDK")).toBeInTheDocument();
    });

    await user.click(screen.getByText("JS SDK"));

    await waitFor(() => {
      expect(screen.getByText(/ayb\.records\.delete/)).toBeInTheDocument();
    });
  });

  it("Send button is disabled while loading", async () => {
    mockExecute.mockReturnValue(new Promise(() => {}));
    render(<ApiExplorer schema={makeSchema()} />);

    const user = userEvent.setup();
    const input = screen.getByLabelText("Request path") as HTMLInputElement;
    await user.clear(input);
    await user.type(input, "/api/collections/posts");
    await user.click(screen.getByText("Send"));

    // Send button should become "Sending..." and be disabled
    expect(screen.getByText("Sending...")).toBeInTheDocument();
    expect(screen.getByText("Sending...")).toBeDisabled();
  });

  it("history item count updates after multiple requests", async () => {
    mockExecute.mockResolvedValue(makeResponse());
    render(<ApiExplorer schema={makeSchema()} />);

    const user = userEvent.setup();
    const input = screen.getByLabelText("Request path") as HTMLInputElement;

    // First request
    await user.clear(input);
    await user.type(input, "/api/collections/posts");
    await user.click(screen.getByText("Send"));

    await waitFor(() => {
      expect(screen.getByText("History (1)")).toBeInTheDocument();
    });

    // Second request
    await user.clear(input);
    await user.type(input, "/api/collections/users");
    await user.click(screen.getByText("Send"));

    await waitFor(() => {
      expect(screen.getByText("History (2)")).toBeInTheDocument();
    });
  });

  it("shows all query parameter fields when panel is open", async () => {
    render(<ApiExplorer schema={makeSchema()} />);

    const user = userEvent.setup();
    await user.click(screen.getByText("Query Parameters"));

    // All 7 query parameter fields
    const fields = ["filter", "sort", "page", "perPage", "fields", "expand", "search"];
    for (const field of fields) {
      expect(screen.getByLabelText(field)).toBeInTheDocument();
    }
  });

  it("empty query params are excluded from request", async () => {
    mockExecute.mockResolvedValueOnce(makeResponse());
    render(<ApiExplorer schema={makeSchema()} />);

    const user = userEvent.setup();
    const input = screen.getByLabelText("Request path") as HTMLInputElement;
    await user.clear(input);
    await user.type(input, "/api/collections/posts");

    // Open params but don't fill anything
    await user.click(screen.getByText("Query Parameters"));
    await user.click(screen.getByText("Send"));

    await waitFor(() => {
      expect(mockExecute).toHaveBeenCalledWith(
        "GET",
        "/api/collections/posts",
        undefined,
      );
    });

    // Path should not contain ? if no params
    const calledPath = mockExecute.mock.calls[0][1];
    expect(calledPath).not.toContain("?");
  });
});
