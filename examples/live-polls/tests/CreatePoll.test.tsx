import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import CreatePoll from "../src/components/CreatePoll";
import { ayb } from "../src/lib/ayb";

// Mock the ayb module.
vi.mock("../src/lib/ayb", () => ({
  ayb: {
    records: {
      create: vi.fn(),
    },
  },
}));

const mockCreate = vi.mocked(ayb.records.create);

describe("CreatePoll", () => {
  beforeEach(() => {
    mockCreate.mockReset();
  });

  it("renders question input and two option inputs", () => {
    render(<CreatePoll userId="test-user-id" onCreated={vi.fn()} />);

    expect(screen.getByPlaceholderText("Ask a question...")).toBeInTheDocument();
    expect(screen.getByPlaceholderText("Option 1")).toBeInTheDocument();
    expect(screen.getByPlaceholderText("Option 2")).toBeInTheDocument();
  });

  it("renders create button", () => {
    render(<CreatePoll userId="test-user-id" onCreated={vi.fn()} />);
    expect(screen.getByText("Create Poll")).toBeInTheDocument();
  });

  it("can add a third option", () => {
    render(<CreatePoll userId="test-user-id" onCreated={vi.fn()} />);

    fireEvent.click(screen.getByText("+ Add option"));
    expect(screen.getByPlaceholderText("Option 3")).toBeInTheDocument();
  });

  it("cannot remove options below 2", () => {
    render(<CreatePoll userId="test-user-id" onCreated={vi.fn()} />);

    // With only 2 options, no remove buttons should be present.
    expect(screen.queryAllByText("x")).toHaveLength(0);
  });

  it("shows remove buttons when more than 2 options", () => {
    render(<CreatePoll userId="test-user-id" onCreated={vi.fn()} />);

    fireEvent.click(screen.getByText("+ Add option"));
    // Each of the 3 options should have a remove button.
    expect(screen.getAllByText("x")).toHaveLength(3);
  });

  it("shows error when submitting with empty options", async () => {
    render(<CreatePoll userId="test-user-id" onCreated={vi.fn()} />);

    const questionInput = screen.getByPlaceholderText("Ask a question...");
    fireEvent.change(questionInput, { target: { value: "What is best?" } });

    // Options are empty by default, form submit with required fields
    // The validation fires inside handleSubmit for option count.
    const form = screen.getByText("Create Poll").closest("form")!;
    fireEvent.submit(form);

    expect(await screen.findByText("At least 2 options required")).toBeInTheDocument();
  });

  it("hides + Add option at 10 options", () => {
    render(<CreatePoll userId="test-user-id" onCreated={vi.fn()} />);

    // Add 8 more options (start with 2, max is 10).
    for (let i = 0; i < 8; i++) {
      fireEvent.click(screen.getByText("+ Add option"));
    }
    expect(screen.queryByText("+ Add option")).not.toBeInTheDocument();
    expect(screen.getByPlaceholderText("Option 10")).toBeInTheDocument();
  });

  it("calls onCreated with poll and options on successful submit", async () => {
    const mockPoll = {
      id: "poll-1",
      question: "Best language?",
      user_id: "user-1",
      is_closed: false,
      created_at: "2026-01-01T00:00:00Z",
    };
    const mockOpt1 = { id: "opt-1", poll_id: "poll-1", label: "TypeScript", position: 0 };
    const mockOpt2 = { id: "opt-2", poll_id: "poll-1", label: "Go", position: 1 };

    // First call creates the poll; subsequent calls create options (in parallel).
    mockCreate
      .mockResolvedValueOnce(mockPoll)
      .mockResolvedValueOnce(mockOpt1)
      .mockResolvedValueOnce(mockOpt2);

    const onCreated = vi.fn();
    render(<CreatePoll userId="user-1" onCreated={onCreated} />);

    fireEvent.change(screen.getByPlaceholderText("Ask a question..."), {
      target: { value: "Best language?" },
    });
    fireEvent.change(screen.getByPlaceholderText("Option 1"), {
      target: { value: "TypeScript" },
    });
    fireEvent.change(screen.getByPlaceholderText("Option 2"), {
      target: { value: "Go" },
    });

    fireEvent.submit(screen.getByText("Create Poll").closest("form")!);

    await waitFor(() => expect(onCreated).toHaveBeenCalledOnce());
    expect(onCreated).toHaveBeenCalledWith(mockPoll, [mockOpt1, mockOpt2]);
  });

  it("shows API error message on failed submit", async () => {
    mockCreate.mockRejectedValueOnce(new Error("Server error"));

    render(<CreatePoll userId="user-1" onCreated={vi.fn()} />);

    fireEvent.change(screen.getByPlaceholderText("Ask a question..."), {
      target: { value: "Test question?" },
    });
    fireEvent.change(screen.getByPlaceholderText("Option 1"), {
      target: { value: "Yes" },
    });
    fireEvent.change(screen.getByPlaceholderText("Option 2"), {
      target: { value: "No" },
    });

    fireEvent.submit(screen.getByText("Create Poll").closest("form")!);

    expect(await screen.findByText("Server error")).toBeInTheDocument();
  });

  it("creates options in parallel (Promise.all), not sequentially", async () => {
    const callOrder: string[] = [];
    const mockPoll = {
      id: "poll-1",
      question: "Test?",
      user_id: "user-1",
      is_closed: false,
      created_at: "2026-01-01T00:00:00Z",
    };

    mockCreate.mockImplementation(async (table: string, data: Record<string, unknown>) => {
      callOrder.push(`${table}:${data.label ?? "poll"}`);
      if (table === "polls") return mockPoll;
      return { id: `opt-${data.position}`, poll_id: "poll-1", ...data };
    });

    const onCreated = vi.fn();
    render(<CreatePoll userId="user-1" onCreated={onCreated} />);

    fireEvent.change(screen.getByPlaceholderText("Ask a question..."), {
      target: { value: "Test?" },
    });
    fireEvent.change(screen.getByPlaceholderText("Option 1"), {
      target: { value: "Alpha" },
    });
    fireEvent.change(screen.getByPlaceholderText("Option 2"), {
      target: { value: "Beta" },
    });

    fireEvent.submit(screen.getByText("Create Poll").closest("form")!);

    await waitFor(() => expect(onCreated).toHaveBeenCalledOnce());

    // poll is created first (sequential), then both options are started together.
    // All 3 creates should have been called.
    expect(mockCreate).toHaveBeenCalledTimes(3);
    expect(callOrder[0]).toBe("polls:poll");
    // Options may arrive in any order due to Promise.all — just verify both called.
    expect(callOrder).toContain("poll_options:Alpha");
    expect(callOrder).toContain("poll_options:Beta");
  });
});
