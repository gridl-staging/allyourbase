import { vi, describe, it, expect, beforeEach } from "vitest";
import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { SMSHealth } from "../SMSHealth";
import { getSMSHealth } from "../../api";
import type { SMSHealthResponse, SMSWindowStats } from "../../types";

vi.mock("../../api", () => ({
  getSMSHealth: vi.fn(),
}));

const mockGetSMSHealth = vi.mocked(getSMSHealth);

function makeWindowStats(
  overrides: Partial<SMSWindowStats> = {},
): SMSWindowStats {
  return {
    sent: 10,
    confirmed: 8,
    failed: 2,
    conversion_rate: 80.0,
    ...overrides,
  };
}

function makeSMSHealthResponse(
  overrides: Partial<SMSHealthResponse> = {},
): SMSHealthResponse {
  return {
    today: makeWindowStats(),
    last_7d: makeWindowStats({ sent: 70, confirmed: 60, failed: 10, conversion_rate: 85.7 }),
    last_30d: makeWindowStats({ sent: 300, confirmed: 270, failed: 30, conversion_rate: 90.0 }),
    ...overrides,
  };
}

describe("SMSHealth", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("renders heading", async () => {
    mockGetSMSHealth.mockResolvedValueOnce(makeSMSHealthResponse());
    render(<SMSHealth />);
    await waitFor(() => {
      expect(screen.getByRole("heading", { name: /SMS Health/i })).toBeVisible();
    });
  });

  it("shows loading state", () => {
    mockGetSMSHealth.mockReturnValue(new Promise(() => {}));
    render(<SMSHealth />);
    expect(screen.getByText("Loading...")).toBeInTheDocument();
  });

  it("displays stats for all three windows", async () => {
    const fixture = makeSMSHealthResponse();
    mockGetSMSHealth.mockResolvedValueOnce(fixture);
    render(<SMSHealth />);

    await waitFor(() => {
      expect(screen.getByText("Today")).toBeInTheDocument();
      expect(screen.getByText("Last 7 Days")).toBeInTheDocument();
      expect(screen.getByText("Last 30 Days")).toBeInTheDocument();
    });

    // Today: sent=10, confirmed=8, failed=2, rate=80.0%
    // Use within().getByText() for exact element text matching.
    // toHaveTextContent() does substring matching on the whole card text,
    // causing false positives (e.g. "8" matches inside "80.0%").
    const todayCard = within(screen.getByTestId("sms-stats-today"));
    expect(todayCard.getByText("10")).toBeInTheDocument();
    expect(todayCard.getByText("8")).toBeInTheDocument();
    expect(todayCard.getByText("2")).toBeInTheDocument();
    expect(todayCard.getByText("80.0%")).toBeInTheDocument();

    // Last 7 Days: sent=70, confirmed=60, failed=10, rate=85.7%
    const weekCard = within(screen.getByTestId("sms-stats-last_7d"));
    expect(weekCard.getByText("70")).toBeInTheDocument();
    expect(weekCard.getByText("60")).toBeInTheDocument();
    expect(weekCard.getByText("10")).toBeInTheDocument();
    expect(weekCard.getByText("85.7%")).toBeInTheDocument();

    // Last 30 Days: sent=300, confirmed=270, failed=30, rate=90.0%
    // getByText("30") matches only <span>30</span>, not "300" or "Last 30 Days"
    const monthCard = within(screen.getByTestId("sms-stats-last_30d"));
    expect(monthCard.getByText("300")).toBeInTheDocument();
    expect(monthCard.getByText("270")).toBeInTheDocument();
    expect(monthCard.getByText("30")).toBeInTheDocument();
    expect(monthCard.getByText("90.0%")).toBeInTheDocument();
  });

  it("shows warning badge when warning key present", async () => {
    const fixture = makeSMSHealthResponse({ warning: "low conversion rate" });
    mockGetSMSHealth.mockResolvedValueOnce(fixture);
    render(<SMSHealth />);

    await waitFor(() => {
      const badge = screen.getByTestId("sms-warning-badge");
      expect(badge).toBeInTheDocument();
      expect(badge).toHaveTextContent("low conversion rate");
    });
  });

  it("hides warning badge when no warning key", async () => {
    const fixture = makeSMSHealthResponse();
    mockGetSMSHealth.mockResolvedValueOnce(fixture);
    render(<SMSHealth />);

    await waitFor(() => {
      expect(screen.getByText("Today")).toBeInTheDocument();
    });

    expect(screen.queryByTestId("sms-warning-badge")).not.toBeInTheDocument();
  });

  it("shows 0% conversion rate when sent is 0", async () => {
    const zeroStats = makeWindowStats({ sent: 0, confirmed: 0, failed: 0, conversion_rate: 0 });
    const fixture = makeSMSHealthResponse({
      today: zeroStats,
      last_7d: zeroStats,
      last_30d: zeroStats,
    });
    mockGetSMSHealth.mockResolvedValueOnce(fixture);
    render(<SMSHealth />);

    await waitFor(() => {
      expect(screen.getByText("Today")).toBeInTheDocument();
    });

    // Should show "0.0%" without NaN.
    // Use within().getByText() for exact element matching (not substring on whole card).
    const todayCard = within(screen.getByTestId("sms-stats-today"));
    expect(todayCard.getByText("0.0%")).toBeInTheDocument();
    expect(screen.getByTestId("sms-stats-today")).not.toHaveTextContent("NaN");
  });

  it("shows error state with retry button", async () => {
    mockGetSMSHealth.mockRejectedValueOnce(new Error("network failure"));
    render(<SMSHealth />);

    await waitFor(() => {
      expect(screen.getByText("network failure")).toBeInTheDocument();
      expect(screen.getByText("Retry")).toBeInTheDocument();
    });
  });

  it("clicking Retry re-fetches", async () => {
    const user = userEvent.setup();
    mockGetSMSHealth.mockRejectedValueOnce(new Error("network failure"));
    render(<SMSHealth />);

    await waitFor(() => {
      expect(screen.getByText("Retry")).toBeInTheDocument();
    });

    mockGetSMSHealth.mockResolvedValueOnce(makeSMSHealthResponse());
    await user.click(screen.getByText("Retry"));

    expect(mockGetSMSHealth).toHaveBeenCalledTimes(2);
  });
});
