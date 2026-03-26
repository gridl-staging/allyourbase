import { vi, describe, it, expect, beforeEach } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MFAChallenge } from "../MFAChallenge";
import {
  getMFAFactors,
  challengeTOTP,
  verifyTOTP,
  challengeEmailMFA,
  verifyEmailMFA,
  challengeSMSMFA,
  verifySMSMFA,
  verifyBackupCode,
} from "../../api";
import type { MFAFactor, AuthTokens } from "../../types";

vi.mock("../../api", () => ({
  getMFAFactors: vi.fn(),
  challengeTOTP: vi.fn(),
  verifyTOTP: vi.fn(),
  challengeEmailMFA: vi.fn(),
  verifyEmailMFA: vi.fn(),
  challengeSMSMFA: vi.fn(),
  verifySMSMFA: vi.fn(),
  verifyBackupCode: vi.fn(),
}));

const mockGetMFAFactors = vi.mocked(getMFAFactors);
const mockChallengeTOTP = vi.mocked(challengeTOTP);
const mockVerifyTOTP = vi.mocked(verifyTOTP);
const mockChallengeEmailMFA = vi.mocked(challengeEmailMFA);
const mockVerifyEmailMFA = vi.mocked(verifyEmailMFA);
const mockChallengeSMSMFA = vi.mocked(challengeSMSMFA);
const mockVerifySMSMFA = vi.mocked(verifySMSMFA);
const mockVerifyBackupCode = vi.mocked(verifyBackupCode);

const TOKENS: AuthTokens = {
  token: "aal2-token",
  refreshToken: "aal2-refresh",
  user: {
    id: "user-1",
    email: "user@test.com",
    createdAt: "2026-01-01T00:00:00Z",
    updatedAt: "2026-01-01T00:00:00Z",
  },
};

describe("MFAChallenge", () => {
  const onVerified = vi.fn();

  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("shows loading state while fetching factors", () => {
    mockGetMFAFactors.mockReturnValue(new Promise(() => {}));
    render(<MFAChallenge onVerified={onVerified} />);
    expect(screen.getByText(/loading/i)).toBeInTheDocument();
  });

  it("shows error when factor fetch fails", async () => {
    mockGetMFAFactors.mockRejectedValue(new Error("unauthorized"));
    render(<MFAChallenge onVerified={onVerified} />);
    await waitFor(() => {
      expect(screen.getByText(/unauthorized/i)).toBeInTheDocument();
    });
  });

  it("shows error when no MFA factors are enrolled", async () => {
    mockGetMFAFactors.mockResolvedValue({ factors: [] });
    render(<MFAChallenge onVerified={onVerified} />);
    await waitFor(() => {
      expect(screen.getByText(/no mfa factors enrolled/i)).toBeInTheDocument();
    });
  });

  describe("factor selection", () => {
    const factors: MFAFactor[] = [
      { id: "f1", method: "totp" },
      { id: "f2", method: "email" },
    ];

    it("shows factor selection when multiple factors exist", async () => {
      mockGetMFAFactors.mockResolvedValue({ factors });
      render(<MFAChallenge onVerified={onVerified} />);
      await waitFor(() => {
        expect(screen.getByRole("heading", { name: /verify your identity/i })).toBeInTheDocument();
      });
      expect(screen.getByRole("button", { name: /authenticator app/i })).toBeInTheDocument();
      expect(screen.getByRole("button", { name: /email/i })).toBeInTheDocument();
    });

    it("selecting a factor from multi-factor screen starts the challenge", async () => {
      mockGetMFAFactors.mockResolvedValue({ factors });
      mockChallengeEmailMFA.mockResolvedValue({ challenge_id: "ch-email-select" });
      const user = userEvent.setup();
      render(<MFAChallenge onVerified={onVerified} />);

      await waitFor(() => {
        expect(screen.getByRole("button", { name: /email/i })).toBeInTheDocument();
      });

      await user.click(screen.getByRole("button", { name: /email/i }));

      await waitFor(() => {
        expect(screen.getByTestId("mfa-code-input")).toBeInTheDocument();
      });
      expect(screen.getByText(/a code was sent to your email/i)).toBeInTheDocument();
    });

    it("auto-selects when only one factor exists", async () => {
      mockGetMFAFactors.mockResolvedValue({ factors: [{ id: "f1", method: "totp" }] });
      mockChallengeTOTP.mockResolvedValue({ challenge_id: "ch-1" });
      render(<MFAChallenge onVerified={onVerified} />);
      await waitFor(() => {
        // Should jump straight to TOTP code entry
        expect(screen.getByTestId("mfa-code-input")).toBeInTheDocument();
      });
    });
  });

  describe("TOTP verification", () => {
    it("creates challenge and verifies TOTP code", async () => {
      mockGetMFAFactors.mockResolvedValue({ factors: [{ id: "f1", method: "totp" }] });
      mockChallengeTOTP.mockResolvedValue({ challenge_id: "ch-1" });
      mockVerifyTOTP.mockResolvedValue(TOKENS);
      const user = userEvent.setup();
      render(<MFAChallenge onVerified={onVerified} />);

      await waitFor(() => expect(screen.getByTestId("mfa-code-input")).toBeInTheDocument());

      await user.type(screen.getByTestId("mfa-code-input"), "123456");
      await user.click(screen.getByRole("button", { name: /verify/i }));

      await waitFor(() => {
        expect(mockVerifyTOTP).toHaveBeenCalledWith("ch-1", "123456");
      });
      await waitFor(() => {
        expect(onVerified).toHaveBeenCalledWith(TOKENS);
      });
    });

    it("shows error on invalid TOTP code", async () => {
      mockGetMFAFactors.mockResolvedValue({ factors: [{ id: "f1", method: "totp" }] });
      mockChallengeTOTP.mockResolvedValue({ challenge_id: "ch-1" });
      mockVerifyTOTP.mockRejectedValue(new Error("invalid TOTP code"));
      const user = userEvent.setup();
      render(<MFAChallenge onVerified={onVerified} />);

      await waitFor(() => expect(screen.getByTestId("mfa-code-input")).toBeInTheDocument());

      await user.type(screen.getByTestId("mfa-code-input"), "000000");
      await user.click(screen.getByRole("button", { name: /verify/i }));

      await waitFor(() => {
        expect(screen.getByText(/invalid.*code/i)).toBeInTheDocument();
      });
      expect(onVerified).not.toHaveBeenCalled();
    });

    it("shows lockout UX on TOTP verify 429", async () => {
      mockGetMFAFactors.mockResolvedValue({ factors: [{ id: "f1", method: "totp" }] });
      mockChallengeTOTP.mockResolvedValue({ challenge_id: "ch-1" });
      mockVerifyTOTP.mockRejectedValue({ status: 429, message: "too many failed attempts, try again later" });
      const user = userEvent.setup();
      render(<MFAChallenge onVerified={onVerified} />);

      await waitFor(() => expect(screen.getByTestId("mfa-code-input")).toBeInTheDocument());

      await user.type(screen.getByTestId("mfa-code-input"), "000000");
      await user.click(screen.getByRole("button", { name: /verify/i }));

      await waitFor(() => {
        expect(screen.getByText(/temporarily locked/i)).toBeInTheDocument();
      });
      expect(screen.getByText(/use backup code/i)).toBeInTheDocument();
      expect(onVerified).not.toHaveBeenCalled();
    });

    it("shows TOTP replay rejection error", async () => {
      mockGetMFAFactors.mockResolvedValue({ factors: [{ id: "f1", method: "totp" }] });
      mockChallengeTOTP.mockResolvedValue({ challenge_id: "ch-1" });
      mockVerifyTOTP.mockRejectedValue(new Error("TOTP code already used"));
      const user = userEvent.setup();
      render(<MFAChallenge onVerified={onVerified} />);

      await waitFor(() => expect(screen.getByTestId("mfa-code-input")).toBeInTheDocument());

      await user.type(screen.getByTestId("mfa-code-input"), "123456");
      await user.click(screen.getByRole("button", { name: /verify/i }));

      await waitFor(() => {
        expect(screen.getByText(/already used/i)).toBeInTheDocument();
      });
      expect(onVerified).not.toHaveBeenCalled();
    });
  });

  describe("Email MFA verification", () => {
    it("challenges and verifies email MFA code", async () => {
      mockGetMFAFactors.mockResolvedValue({ factors: [{ id: "f1", method: "email" }] });
      mockChallengeEmailMFA.mockResolvedValue({ challenge_id: "ch-email-1" });
      mockVerifyEmailMFA.mockResolvedValue(TOKENS);
      const user = userEvent.setup();
      render(<MFAChallenge onVerified={onVerified} />);

      await waitFor(() => expect(screen.getByTestId("mfa-code-input")).toBeInTheDocument());
      expect(screen.getByText(/a code was sent to your email/i)).toBeInTheDocument();

      await user.type(screen.getByTestId("mfa-code-input"), "654321");
      await user.click(screen.getByRole("button", { name: /verify/i }));

      await waitFor(() => {
        expect(mockVerifyEmailMFA).toHaveBeenCalledWith("ch-email-1", "654321");
      });
      await waitFor(() => {
        expect(onVerified).toHaveBeenCalledWith(TOKENS);
      });
    });

    it("shows rate limit error on email challenge", async () => {
      mockGetMFAFactors.mockResolvedValue({ factors: [{ id: "f1", method: "email" }] });
      mockChallengeEmailMFA.mockRejectedValue(new Error("too many email challenges, try again later"));
      render(<MFAChallenge onVerified={onVerified} />);

      await waitFor(() => {
        expect(screen.getByText(/too many.*challenges/i)).toBeInTheDocument();
      });
    });

    it("shows lockout UX copy when email MFA verify is temporarily locked", async () => {
      mockGetMFAFactors.mockResolvedValue({ factors: [{ id: "f1", method: "email" }] });
      mockChallengeEmailMFA.mockResolvedValue({ challenge_id: "ch-email-1" });
      mockVerifyEmailMFA.mockRejectedValue(new Error("too many failed attempts, try again later"));
      const user = userEvent.setup();
      render(<MFAChallenge onVerified={onVerified} />);

      await waitFor(() => expect(screen.getByTestId("mfa-code-input")).toBeInTheDocument());
      await user.type(screen.getByTestId("mfa-code-input"), "111111");
      await user.click(screen.getByRole("button", { name: /verify/i }));

      await waitFor(() => {
        expect(screen.getByText(/temporarily locked/i)).toBeInTheDocument();
      });
      expect(screen.getByText(/use backup code/i)).toBeInTheDocument();
    });

    it("preserves lockout retry detail from non-Error API failures", async () => {
      mockGetMFAFactors.mockResolvedValue({ factors: [{ id: "f1", method: "email" }] });
      mockChallengeEmailMFA.mockResolvedValue({ challenge_id: "ch-email-1" });
      mockVerifyEmailMFA.mockRejectedValue({
        status: 429,
        message: "too many failed attempts, try again in 27 minutes",
      });
      const user = userEvent.setup();
      render(<MFAChallenge onVerified={onVerified} />);

      await waitFor(() => expect(screen.getByTestId("mfa-code-input")).toBeInTheDocument());
      await user.type(screen.getByTestId("mfa-code-input"), "111111");
      await user.click(screen.getByRole("button", { name: /verify/i }));

      await waitFor(() => {
        expect(screen.getByText(/temporarily locked/i)).toBeInTheDocument();
      });
      expect(screen.getByText(/27 minutes/i)).toBeInTheDocument();
    });
  });

  describe("SMS MFA verification", () => {
    it("challenges and verifies SMS MFA code", async () => {
      mockGetMFAFactors.mockResolvedValue({ factors: [{ id: "f1", method: "sms", phone: "***2671" }] });
      mockChallengeSMSMFA.mockResolvedValue({ message: "verification code sent" });
      mockVerifySMSMFA.mockResolvedValue(TOKENS);
      const user = userEvent.setup();
      render(<MFAChallenge onVerified={onVerified} />);

      await waitFor(() => expect(screen.getByTestId("mfa-code-input")).toBeInTheDocument());
      expect(screen.getByText(/a code was sent to your phone/i)).toBeInTheDocument();

      await user.type(screen.getByTestId("mfa-code-input"), "654321");
      await user.click(screen.getByRole("button", { name: /verify/i }));

      await waitFor(() => {
        expect(mockVerifySMSMFA).toHaveBeenCalledWith("654321");
      });
      await waitFor(() => {
        expect(onVerified).toHaveBeenCalledWith(TOKENS);
      });
    });
  });

  describe("resend and countdown", () => {
    it("shows resend button for email challenges after initial code sent", async () => {
      mockGetMFAFactors.mockResolvedValue({ factors: [{ id: "f1", method: "email" }] });
      mockChallengeEmailMFA.mockResolvedValue({ challenge_id: "ch-email-1" });
      render(<MFAChallenge onVerified={onVerified} />);

      await waitFor(() => expect(screen.getByTestId("mfa-code-input")).toBeInTheDocument());
      expect(screen.getByRole("button", { name: /resend code/i })).toBeInTheDocument();
    });

    it("shows resend button for SMS challenges after initial code sent", async () => {
      mockGetMFAFactors.mockResolvedValue({ factors: [{ id: "f1", method: "sms", phone: "***2671" }] });
      mockChallengeSMSMFA.mockResolvedValue({ message: "verification code sent" });
      render(<MFAChallenge onVerified={onVerified} />);

      await waitFor(() => expect(screen.getByTestId("mfa-code-input")).toBeInTheDocument());
      expect(screen.getByRole("button", { name: /resend code/i })).toBeInTheDocument();
    });

    it("does not show resend button for TOTP challenges", async () => {
      mockGetMFAFactors.mockResolvedValue({ factors: [{ id: "f1", method: "totp" }] });
      mockChallengeTOTP.mockResolvedValue({ challenge_id: "ch-1" });
      render(<MFAChallenge onVerified={onVerified} />);

      await waitFor(() => expect(screen.getByTestId("mfa-code-input")).toBeInTheDocument());
      expect(screen.queryByRole("button", { name: /resend code/i })).not.toBeInTheDocument();
    });

    it("resend button triggers a new challenge for email", async () => {
      mockGetMFAFactors.mockResolvedValue({ factors: [{ id: "f1", method: "email" }] });
      mockChallengeEmailMFA.mockResolvedValue({ challenge_id: "ch-email-1" });
      const user = userEvent.setup();
      render(<MFAChallenge onVerified={onVerified} />);

      await waitFor(() => expect(screen.getByTestId("mfa-code-input")).toBeInTheDocument());
      expect(mockChallengeEmailMFA).toHaveBeenCalledTimes(1);

      mockChallengeEmailMFA.mockResolvedValue({ challenge_id: "ch-email-2" });
      await user.click(screen.getByRole("button", { name: /resend code/i }));

      await waitFor(() => {
        expect(mockChallengeEmailMFA).toHaveBeenCalledTimes(2);
      });
    });

    it("shows countdown timer text for email code expiry", async () => {
      mockGetMFAFactors.mockResolvedValue({ factors: [{ id: "f1", method: "email" }] });
      mockChallengeEmailMFA.mockResolvedValue({ challenge_id: "ch-email-1" });
      render(<MFAChallenge onVerified={onVerified} />);

      await waitFor(() => expect(screen.getByTestId("mfa-code-input")).toBeInTheDocument());
      // Should display code expiry info
      expect(screen.getByText(/expires in/i)).toBeInTheDocument();
    });

    it("shows an expired state at countdown end and clears it after resend", async () => {
      mockGetMFAFactors.mockResolvedValue({ factors: [{ id: "f1", method: "email" }] });
      mockChallengeEmailMFA
        .mockResolvedValueOnce({ challenge_id: "ch-email-1" })
        .mockResolvedValueOnce({ challenge_id: "ch-email-2" });

      const user = userEvent.setup();
      render(
        <MFAChallenge
          onVerified={onVerified}
          codeExpirySeconds={{ email: 1, sms: 1 }}
        />,
      );

      await waitFor(() => expect(screen.getByTestId("mfa-code-input")).toBeInTheDocument());
      const codeInput = screen.getByTestId("mfa-code-input");
      const verifyButton = screen.getByRole("button", { name: /verify/i });

      await user.type(codeInput, "123456");
      expect(verifyButton).toBeEnabled();

      await waitFor(() => {
        expect(screen.getByText(/code expired/i)).toBeInTheDocument();
      }, { timeout: 2500 });
      expect(verifyButton).toBeDisabled();

      await user.click(screen.getByRole("button", { name: /resend code/i }));
      await waitFor(() => {
        expect(screen.queryByText(/code expired/i)).not.toBeInTheDocument();
      });
      expect(codeInput).toHaveValue("");
    });
  });

  describe("Backup code verification", () => {
    it("shows backup code option and verifies code", async () => {
      mockGetMFAFactors.mockResolvedValue({ factors: [{ id: "f1", method: "totp" }] });
      mockChallengeTOTP.mockResolvedValue({ challenge_id: "ch-1" });
      mockVerifyBackupCode.mockResolvedValue(TOKENS);
      const user = userEvent.setup();
      render(<MFAChallenge onVerified={onVerified} />);

      await waitFor(() => expect(screen.getByTestId("mfa-code-input")).toBeInTheDocument());

      // Switch to backup code mode
      await user.click(screen.getByRole("button", { name: /use backup code/i }));
      expect(screen.getByTestId("backup-code-input")).toBeInTheDocument();

      await user.type(screen.getByTestId("backup-code-input"), "abc12-def34");
      await user.click(screen.getByRole("button", { name: /verify/i }));

      await waitFor(() => {
        expect(mockVerifyBackupCode).toHaveBeenCalledWith("abc12-def34");
      });
      await waitFor(() => {
        expect(onVerified).toHaveBeenCalledWith(TOKENS);
      });
    });

    it("shows error on invalid backup code", async () => {
      mockGetMFAFactors.mockResolvedValue({ factors: [{ id: "f1", method: "totp" }] });
      mockChallengeTOTP.mockResolvedValue({ challenge_id: "ch-1" });
      mockVerifyBackupCode.mockRejectedValue(new Error("invalid or already used backup code"));
      const user = userEvent.setup();
      render(<MFAChallenge onVerified={onVerified} />);

      await waitFor(() => expect(screen.getByTestId("mfa-code-input")).toBeInTheDocument());

      await user.click(screen.getByRole("button", { name: /use backup code/i }));
      await user.type(screen.getByTestId("backup-code-input"), "bad00-code0");
      await user.click(screen.getByRole("button", { name: /verify/i }));

      await waitFor(() => {
        expect(screen.getByText(/invalid.*backup code/i)).toBeInTheDocument();
      });
    });
  });
});
