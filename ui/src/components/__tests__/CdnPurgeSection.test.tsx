import { vi, describe, it, expect, beforeEach } from "vitest";
import { screen, waitFor } from "@testing-library/react";
import { renderWithProviders, MockApiError } from "../../test-utils";
import userEvent from "@testing-library/user-event";
import { CdnPurgeSection } from "../CdnPurgeSection";
import { purgeStorageCDN } from "../../api";

vi.mock("../../api", () => ({
  purgeStorageCDN: vi.fn(),
  ApiError: class extends Error {
    status: number;
    constructor(status: number, message: string) {
      super(message);
      this.status = status;
    }
  },
}));

const mockPurge = vi.mocked(purgeStorageCDN);

describe("CdnPurgeSection", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("renders URL textarea and submit button", () => {
    renderWithProviders(<CdnPurgeSection />);
    expect(screen.getByPlaceholderText(/one URL per line/i)).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /purge urls/i })).toBeInTheDocument();
  });

  it("submits targeted URL purge and shows success toast", async () => {
    const user = userEvent.setup();
    mockPurge.mockResolvedValueOnce({
      operation: "purge_urls",
      submitted: 2,
      provider: "cloudflare",
    });

    renderWithProviders(<CdnPurgeSection />);

    const textarea = screen.getByPlaceholderText(/one URL per line/i);
    await user.type(textarea, "https://cdn.example.com/a.js\nhttps://cdn.example.com/b.css");
    await user.click(screen.getByRole("button", { name: /purge urls/i }));

    await waitFor(() => {
      expect(mockPurge).toHaveBeenCalledWith({
        urls: ["https://cdn.example.com/a.js", "https://cdn.example.com/b.css"],
      });
    });

    await waitFor(() => {
      expect(screen.getByText(/purged 2 urls/i)).toBeInTheDocument();
    });
  });

  it("requires confirmation before purge-all", async () => {
    const user = userEvent.setup();
    mockPurge.mockResolvedValueOnce({
      operation: "purge_all",
      submitted: 0,
      provider: "cloudflare",
    });

    renderWithProviders(<CdnPurgeSection />);

    await user.click(screen.getByRole("button", { name: /purge all/i }));

    // Confirmation step should appear
    expect(screen.getByText(/are you sure/i)).toBeInTheDocument();

    // API should not have been called yet
    expect(mockPurge).not.toHaveBeenCalled();

    // Confirm
    await user.click(screen.getByRole("button", { name: /^confirm$/i }));

    await waitFor(() => {
      expect(mockPurge).toHaveBeenCalledWith({ purgeAll: true });
    });

    await waitFor(() => {
      expect(screen.getByText(/full cache purge submitted/i)).toBeInTheDocument();
    });
  });

  it("cancels purge-all confirmation", async () => {
    const user = userEvent.setup();

    renderWithProviders(<CdnPurgeSection />);

    await user.click(screen.getByRole("button", { name: /purge all/i }));
    expect(screen.getByText(/are you sure/i)).toBeInTheDocument();

    await user.click(screen.getByRole("button", { name: /cancel/i }));
    expect(screen.queryByText(/are you sure/i)).not.toBeInTheDocument();
    expect(mockPurge).not.toHaveBeenCalled();
  });

  it("displays backend validation error (400)", async () => {
    const user = userEvent.setup();
    mockPurge.mockRejectedValueOnce(
      new MockApiError(400, "urls must contain absolute public URLs"),
    );

    renderWithProviders(<CdnPurgeSection />);

    const textarea = screen.getByPlaceholderText(/one URL per line/i);
    await user.type(textarea, "not-a-url");
    await user.click(screen.getByRole("button", { name: /purge urls/i }));

    await waitFor(() => {
      expect(screen.getByText(/urls must contain absolute public urls/i)).toBeInTheDocument();
    });
  });

  it("displays rate-limit error (429)", async () => {
    const user = userEvent.setup();
    mockPurge.mockRejectedValueOnce(
      new MockApiError(429, "cdn purge_all rate limit exceeded"),
    );

    renderWithProviders(<CdnPurgeSection />);

    // Trigger purge-all flow
    await user.click(screen.getByRole("button", { name: /purge all/i }));
    await user.click(screen.getByRole("button", { name: /^confirm$/i }));

    await waitFor(() => {
      expect(screen.getByText(/cdn purge_all rate limit exceeded/i)).toBeInTheDocument();
    });
  });

  it("disables submit button while purge is in flight", async () => {
    const user = userEvent.setup();
    let resolve: (v: { operation: string; submitted: number; provider: string }) => void;
    mockPurge.mockReturnValueOnce(
      new Promise((r) => { resolve = r; }),
    );

    renderWithProviders(<CdnPurgeSection />);

    const textarea = screen.getByPlaceholderText(/one URL per line/i);
    await user.type(textarea, "https://cdn.example.com/a.js");
    await user.click(screen.getByRole("button", { name: /purge urls/i }));

    // Button should be disabled while in flight
    expect(screen.getByRole("button", { name: /purging/i })).toBeDisabled();

    // Resolve the promise
    resolve!({ operation: "purge_urls", submitted: 1, provider: "cloudflare" });

    // After resolution the button text returns to "Purge URLs" (no longer "Purging…")
    // but it's disabled because the textarea was cleared on success
    await waitFor(() => {
      expect(screen.getByRole("button", { name: /purge urls/i })).toBeInTheDocument();
    });
    expect(screen.queryByRole("button", { name: /purging/i })).not.toBeInTheDocument();
  });

  it("does not submit when textarea is empty", async () => {
    renderWithProviders(<CdnPurgeSection />);

    const submitBtn = screen.getByRole("button", { name: /purge urls/i });
    expect(submitBtn).toBeDisabled();

    expect(mockPurge).not.toHaveBeenCalled();
  });

  it("clears textarea after successful targeted purge", async () => {
    const user = userEvent.setup();
    mockPurge.mockResolvedValueOnce({
      operation: "purge_urls",
      submitted: 1,
      provider: "cloudflare",
    });

    renderWithProviders(<CdnPurgeSection />);

    const textarea = screen.getByPlaceholderText(/one URL per line/i) as HTMLTextAreaElement;
    await user.type(textarea, "https://cdn.example.com/a.js");
    await user.click(screen.getByRole("button", { name: /purge urls/i }));

    await waitFor(() => {
      expect(textarea.value).toBe("");
    });
  });
});
