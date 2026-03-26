import { describe, it, expect, vi, beforeEach } from "vitest";
import { screen, fireEvent, waitFor } from "@testing-library/react";
import { renderWithProviders } from "../../test-utils";
import { AIAssistant } from "../AIAssistant";

vi.mock("../../api_ai", () => ({
  listAILogs: vi.fn(),
  getAIUsage: vi.fn(),
  getDailyUsage: vi.fn(),
  listPrompts: vi.fn(),
  createPrompt: vi.fn(),
  deletePrompt: vi.fn(),
  sendAssistantQuery: vi.fn(),
  streamAssistant: vi.fn(),
  listAssistantHistory: vi.fn(),
  getPromptVersions: vi.fn(),
  renderPrompt: vi.fn(),
  getPrompt: vi.fn(),
  updatePrompt: vi.fn(),
}));

import * as api from "../../api_ai";

const mockLogs = {
  logs: [
    {
      id: "log-1",
      provider: "openai",
      model: "gpt-4",
      input_tokens: 500,
      output_tokens: 200,
      cost_usd: 0.035,
      duration_ms: 1200,
      status: "success",
      created_at: "2026-03-12T12:00:00Z",
    },
    {
      id: "log-2",
      provider: "anthropic",
      model: "claude-3",
      input_tokens: 300,
      output_tokens: 100,
      cost_usd: 0.012,
      duration_ms: 800,
      status: "error",
      error_message: "Rate limited",
      created_at: "2026-03-12T12:05:00Z",
    },
  ],
  total: 2,
};

const mockUsage = {
  total_calls: 100,
  total_input_tokens: 50000,
  total_output_tokens: 20000,
  total_tokens: 70000,
  total_cost_usd: 3.5,
  error_count: 5,
  by_provider: {
    openai: {
      calls: 60,
      input_tokens: 30000,
      output_tokens: 12000,
      total_tokens: 42000,
      total_cost_usd: 2.1,
      error_count: 2,
    },
    anthropic: {
      calls: 40,
      input_tokens: 20000,
      output_tokens: 8000,
      total_tokens: 28000,
      total_cost_usd: 1.4,
      error_count: 3,
    },
  },
};

const mockPrompts = {
  prompts: [
    {
      id: "p-1",
      name: "SQL Generator",
      version: 3,
      template: "Generate SQL for: {{query}}",
      variables: [{ name: "query", type: "string", required: true }],
      model: "gpt-4",
      provider: "openai",
      created_at: "2026-03-01T00:00:00Z",
      updated_at: "2026-03-10T00:00:00Z",
    },
  ],
  total: 1,
};

const mockDailyUsage = [
  {
    day: "2026-03-12T00:00:00Z",
    provider: "openai",
    model: "gpt-4",
    calls: 25,
    input_tokens: 12000,
    output_tokens: 5000,
    total_tokens: 17000,
    total_cost_usd: 0.9,
  },
];

function expectedDateBoundary(value: string, boundary: "start" | "end"): string {
  const [year, month, day] = value.split("-").map((part) => Number.parseInt(part, 10));
  const localDate = boundary === "start"
    ? new Date(year, month - 1, day, 0, 0, 0, 0)
    : new Date(year, month - 1, day, 23, 59, 59, 999);
  return localDate.toISOString();
}

beforeEach(() => {
  vi.clearAllMocks();
  (api.listAILogs as ReturnType<typeof vi.fn>).mockResolvedValue(mockLogs);
  (api.getAIUsage as ReturnType<typeof vi.fn>).mockResolvedValue(mockUsage);
  (api.getDailyUsage as ReturnType<typeof vi.fn>).mockResolvedValue(mockDailyUsage);
  (api.listPrompts as ReturnType<typeof vi.fn>).mockResolvedValue(mockPrompts);
  (api.createPrompt as ReturnType<typeof vi.fn>).mockResolvedValue(mockPrompts.prompts[0]);
  (api.updatePrompt as ReturnType<typeof vi.fn>).mockResolvedValue(mockPrompts.prompts[0]);
  (api.getPrompt as ReturnType<typeof vi.fn>).mockResolvedValue(mockPrompts.prompts[0]);
  (api.getPromptVersions as ReturnType<typeof vi.fn>).mockResolvedValue([
    {
      id: "pv-1",
      prompt_id: "p-1",
      version: 1,
      template: "Generate SQL for: {{query}}",
      variables: [{ name: "query", type: "string", required: true }],
      created_at: "2026-03-01T00:00:00Z",
    },
  ]);
  (api.renderPrompt as ReturnType<typeof vi.fn>).mockResolvedValue({
    rendered: "Generate SQL for: list active users",
    prompt: mockPrompts.prompts[0],
  });
  (api.deletePrompt as ReturnType<typeof vi.fn>).mockResolvedValue(undefined);
  (api.streamAssistant as ReturnType<typeof vi.fn>).mockResolvedValue({
    sql: "SELECT 1;",
    explanation: "Query checks database reachability.",
  });
});

describe("AIAssistant", () => {
  it("renders AI logs with provider/model/tokens/cost/status", async () => {
    renderWithProviders(<AIAssistant />);
    await waitFor(() => {
      expect(screen.getByText("openai")).toBeInTheDocument();
    });
    expect(screen.getByText("gpt-4")).toBeInTheDocument();
    expect(screen.getByText("success")).toBeInTheDocument();
    expect(screen.getByText("error")).toBeInTheDocument();
  });

  it("switches to usage tab and shows summary", async () => {
    renderWithProviders(<AIAssistant />);
    await waitFor(() => {
      expect(screen.getByText("openai")).toBeInTheDocument();
    });
    fireEvent.click(screen.getByRole("button", { name: /usage/i }));
    await waitFor(() => {
      expect(screen.getByText("100")).toBeInTheDocument();
    });
    expect(screen.getByText("$3.50")).toBeInTheDocument();
    expect(screen.getByText("gpt-4")).toBeInTheDocument();
  });

  it("switches to prompts tab and shows prompt list", async () => {
    renderWithProviders(<AIAssistant />);
    await waitFor(() => {
      expect(screen.getByText("openai")).toBeInTheDocument();
    });
    fireEvent.click(screen.getByRole("button", { name: /prompts/i }));
    await waitFor(() => {
      expect(screen.getByText("SQL Generator")).toBeInTheDocument();
    });
  });

  it("shows confirm dialog when deleting a prompt", async () => {
    renderWithProviders(<AIAssistant />);
    await waitFor(() => {
      expect(screen.getByText("openai")).toBeInTheDocument();
    });
    fireEvent.click(screen.getByRole("button", { name: /prompts/i }));
    await waitFor(() => {
      expect(screen.getByText("SQL Generator")).toBeInTheDocument();
    });
    fireEvent.click(screen.getByLabelText(/delete prompt sql generator/i));
    await waitFor(() => {
      expect(screen.getByRole("heading", { name: /delete prompt/i })).toBeInTheDocument();
    });
  });

  it("shows error state on fetch failure", async () => {
    (api.listAILogs as ReturnType<typeof vi.fn>).mockRejectedValue(
      new Error("AI service unavailable"),
    );
    renderWithProviders(<AIAssistant />);
    await waitFor(() => {
      expect(screen.getByText("AI service unavailable")).toBeInTheDocument();
    });
  });

  it("does not reload AI logs until Apply is clicked", async () => {
    renderWithProviders(<AIAssistant />);
    await waitFor(() => {
      expect(api.listAILogs).toHaveBeenCalledTimes(1);
    });

    fireEvent.change(screen.getByLabelText("Provider"), {
      target: { value: "openai" },
    });
    expect(api.listAILogs).toHaveBeenCalledTimes(1);

    fireEvent.click(screen.getByRole("button", { name: /apply filters/i }));
    await waitFor(() => {
      expect(api.listAILogs).toHaveBeenCalledTimes(2);
    });
    expect(api.listAILogs).toHaveBeenLastCalledWith({
      provider: "openai",
      status: undefined,
      from: undefined,
      to: undefined,
    });
  });

  it("converts AI log date filters to local-day RFC3339 boundaries", async () => {
    renderWithProviders(<AIAssistant />);
    await waitFor(() => {
      expect(api.listAILogs).toHaveBeenCalledTimes(1);
    });

    fireEvent.change(screen.getByLabelText("From"), {
      target: { value: "2026-03-01" },
    });
    fireEvent.change(screen.getByLabelText("To"), {
      target: { value: "2026-03-12" },
    });
    fireEvent.click(screen.getByRole("button", { name: /apply filters/i }));

    await waitFor(() => {
      expect(api.listAILogs).toHaveBeenLastCalledWith({
        provider: undefined,
        status: undefined,
        from: expectedDateBoundary("2026-03-01", "start"),
        to: expectedDateBoundary("2026-03-12", "end"),
      });
    });
  });

  it("streams assistant response from Assistant tab", async () => {
    renderWithProviders(<AIAssistant />);
    await waitFor(() => {
      expect(screen.getByText("openai")).toBeInTheDocument();
    });

    fireEvent.click(screen.getByRole("button", { name: /assistant/i }));
    fireEvent.change(screen.getByLabelText("Query"), {
      target: { value: "show a heartbeat query" },
    });
    fireEvent.click(screen.getByRole("button", { name: /send query/i }));

    await waitFor(() => {
      expect(api.streamAssistant).toHaveBeenCalled();
      expect(screen.getByText("SELECT 1;")).toBeInTheDocument();
    });
  });

  it("retries the active usage tab request instead of logs", async () => {
    (api.getAIUsage as ReturnType<typeof vi.fn>)
      .mockRejectedValueOnce(new Error("usage failed"))
      .mockResolvedValue(mockUsage);

    renderWithProviders(<AIAssistant />);
    await waitFor(() => {
      expect(screen.getByText("openai")).toBeInTheDocument();
    });

    fireEvent.click(screen.getByRole("button", { name: /^Usage$/i }));
    await waitFor(() => {
      expect(screen.getByText("usage failed")).toBeInTheDocument();
    });

    fireEvent.click(screen.getByRole("button", { name: /retry/i }));
    await waitFor(() => {
      expect(api.getAIUsage).toHaveBeenCalledTimes(2);
    });
    expect(api.listAILogs).toHaveBeenCalledTimes(1);
  });

  it("supports prompt create, update, versions, and template rendering", async () => {
    renderWithProviders(<AIAssistant />);
    await waitFor(() => {
      expect(screen.getByText("openai")).toBeInTheDocument();
    });

    fireEvent.click(screen.getByRole("button", { name: /prompts/i }));
    await waitFor(() => {
      expect(screen.getByText("SQL Generator")).toBeInTheDocument();
    });

    fireEvent.click(screen.getByRole("button", { name: /create prompt/i }));
    fireEvent.change(screen.getByLabelText("Prompt Name"), {
      target: { value: "Schema Helper" },
    });
    fireEvent.change(screen.getByLabelText("Template"), {
      target: { value: "Explain: {{question}}" },
    });
    fireEvent.click(screen.getByRole("button", { name: /save prompt/i }));
    await waitFor(() => {
      expect(api.createPrompt).toHaveBeenCalled();
      expect(api.listPrompts).toHaveBeenCalledTimes(2);
    });

    fireEvent.click(screen.getByLabelText(/edit prompt sql generator/i));
    fireEvent.change(screen.getByLabelText("Template"), {
      target: { value: "Generate SQL for: {{query}} LIMIT 10" },
    });
    fireEvent.click(screen.getByRole("button", { name: /update prompt/i }));
    await waitFor(() => {
      expect(api.updatePrompt).toHaveBeenCalledWith(
        "p-1",
        expect.objectContaining({ template: "Generate SQL for: {{query}} LIMIT 10" }),
      );
      expect(api.listPrompts).toHaveBeenCalledTimes(3);
    });

    fireEvent.click(screen.getByLabelText(/view versions sql generator/i));
    await waitFor(() => {
      expect(api.getPromptVersions).toHaveBeenCalledWith("p-1");
    });

    fireEvent.change(screen.getByLabelText("Render Variables"), {
      target: { value: "{\"query\":\"list active users\"}" },
    });
    fireEvent.click(screen.getByRole("button", { name: /render template/i }));
    await waitFor(() => {
      expect(api.renderPrompt).toHaveBeenCalledWith("p-1", { query: "list active users" });
      expect(screen.getByText("Generate SQL for: list active users")).toBeInTheDocument();
    });
  });
});
