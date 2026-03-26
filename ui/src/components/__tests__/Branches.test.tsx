import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, it, expect, vi, beforeEach } from "vitest";
import { Branches } from "../Branches";
import { ToastProvider } from "../ToastProvider";

function renderBranches() {
  return render(
    <ToastProvider>
      <Branches />
    </ToastProvider>,
  );
}

const BRANCH_DEV = {
  id: "b1",
  name: "dev",
  source_database: "postgres://localhost/main",
  branch_database: "postgres://localhost/branch_dev",
  status: "ready",
  created_at: "2026-02-27T10:00:00Z",
  updated_at: "2026-02-27T10:00:00Z",
};

const BRANCH_STAGING = {
  id: "b2",
  name: "staging",
  source_database: "postgres://localhost/main",
  branch_database: "postgres://localhost/branch_staging",
  status: "creating",
  created_at: "2026-02-27T11:00:00Z",
  updated_at: "2026-02-27T11:00:00Z",
};

function mockFetchOk(body: unknown, status = 200) {
  return vi.fn<typeof fetch>().mockResolvedValue(
    new Response(JSON.stringify(body), {
      status,
      headers: { "Content-Type": "application/json" },
    }),
  );
}

function mockFetchSequence(responses: Array<{ body: unknown; status?: number }>) {
  const fn = vi.fn<typeof fetch>();
  for (const r of responses) {
    fn.mockResolvedValueOnce(
      new Response(JSON.stringify(r.body), {
        status: r.status ?? 200,
        headers: { "Content-Type": "application/json" },
      }),
    );
  }
  return fn;
}

beforeEach(() => {
  localStorage.clear();
  localStorage.setItem("ayb_admin_token", "test-admin-token");
  vi.restoreAllMocks();
});

describe("Branches", () => {
  it("renders branch list after successful fetch", async () => {
    vi.stubGlobal("fetch", mockFetchOk({ branches: [BRANCH_DEV, BRANCH_STAGING] }));

    renderBranches();

    await waitFor(() => {
      expect(screen.getByText("dev")).toBeInTheDocument();
    });
    expect(screen.getByText("staging")).toBeInTheDocument();
    expect(screen.getByText("Ready")).toBeInTheDocument();
    expect(screen.getByText("Creating")).toBeInTheDocument();
  });

  it("shows empty state when no branches exist", async () => {
    vi.stubGlobal("fetch", mockFetchOk({ branches: [] }));

    renderBranches();

    await waitFor(() => {
      expect(screen.getByText(/no branches/i)).toBeInTheDocument();
    });
  });

  it("create modal opens and submits", async () => {
    const user = userEvent.setup();
    const fetchMock = mockFetchSequence([
      { body: { branches: [] } },
      {
        body: {
          id: "b3",
          name: "feature-x",
          source_database: "postgres://localhost/main",
          branch_database: "postgres://localhost/branch_feature_x",
          status: "creating",
          created_at: "2026-02-27T12:00:00Z",
          updated_at: "2026-02-27T12:00:00Z",
        },
        status: 201,
      },
      { body: { branches: [] } },
    ]);
    vi.stubGlobal("fetch", fetchMock);

    renderBranches();

    await waitFor(() => {
      expect(screen.getByText(/no branches/i)).toBeInTheDocument();
    });

    await user.click(screen.getByRole("button", { name: /add branch/i }));
    const nameInput = screen.getByPlaceholderText(/branch name/i);
    await user.type(nameInput, "feature-x");
    await user.click(screen.getByRole("button", { name: /^create$/i }));

    await waitFor(() => {
      const postCall = fetchMock.mock.calls.find(
        (c) => (c[1] as RequestInit)?.method === "POST",
      );
      expect(postCall).toBeDefined();
      expect(JSON.parse((postCall![1] as RequestInit).body as string)).toEqual({
        name: "feature-x",
        from: undefined,
      });
    });
  });

  it("shows conflict error on duplicate name", async () => {
    const user = userEvent.setup();
    const fetchMock = mockFetchSequence([
      { body: { branches: [] } },
      { body: { message: "branch already exists" }, status: 409 },
    ]);
    vi.stubGlobal("fetch", fetchMock);

    renderBranches();

    await waitFor(() => {
      expect(screen.getByText(/no branches/i)).toBeInTheDocument();
    });

    await user.click(screen.getByRole("button", { name: /add branch/i }));
    await user.type(screen.getByPlaceholderText(/branch name/i), "dev");
    await user.click(screen.getByRole("button", { name: /^create$/i }));

    await waitFor(() => {
      expect(screen.getByText(/already exists/i)).toBeInTheDocument();
    });
  });

  it("delete confirmation flow works", async () => {
    const user = userEvent.setup();
    const fetchMock = mockFetchSequence([
      { body: { branches: [BRANCH_DEV] } },
      { body: { status: "deleted" } },
      { body: { branches: [] } },
    ]);
    vi.stubGlobal("fetch", fetchMock);

    renderBranches();

    await waitFor(() => {
      expect(screen.getByText("dev")).toBeInTheDocument();
    });

    await user.click(screen.getByRole("button", { name: /delete/i }));
    await user.click(screen.getByRole("button", { name: /confirm/i }));

    await waitFor(() => {
      const deleteCall = fetchMock.mock.calls.find(
        (c) => (c[1] as RequestInit)?.method === "DELETE",
      );
      expect(deleteCall).toBeDefined();
      expect(deleteCall![0]).toContain("/api/admin/branches/dev");
    });
  });
});
