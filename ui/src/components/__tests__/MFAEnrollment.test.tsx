import { vi, describe, it, expect, beforeEach } from "vitest";
import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MFAEnrollment } from "../MFAEnrollment";
import {
  enrollTOTP,
  confirmTOTPEnroll,
  enrollEmailMFA,
  confirmEmailMFAEnroll,
  generateBackupCodes,
  regenerateBackupCodes,
  getBackupCodeCount,
  getMFAFactors,
} from "../../api";
import type { TOTPEnrollment, MFAFactor } from "../../types";

vi.mock("../../api", () => ({
  enrollTOTP: vi.fn(),
  confirmTOTPEnroll: vi.fn(),
  enrollEmailMFA: vi.fn(),
  confirmEmailMFAEnroll: vi.fn(),
  generateBackupCodes: vi.fn(),
  regenerateBackupCodes: vi.fn(),
  getBackupCodeCount: vi.fn(),
  getMFAFactors: vi.fn(),
}));

const mockEnrollTOTP = vi.mocked(enrollTOTP);
const mockConfirmTOTPEnroll = vi.mocked(confirmTOTPEnroll);
const mockEnrollEmailMFA = vi.mocked(enrollEmailMFA);
const mockConfirmEmailMFAEnroll = vi.mocked(confirmEmailMFAEnroll);
const mockGenerateBackupCodes = vi.mocked(generateBackupCodes);
const mockRegenerateBackupCodes = vi.mocked(regenerateBackupCodes);
const mockGetBackupCodeCount = vi.mocked(getBackupCodeCount);
const mockGetMFAFactors = vi.mocked(getMFAFactors);

const TOTP_ENROLLMENT: TOTPEnrollment = {
  factor_id: "factor-123",
  uri: "otpauth://totp/AYB:user@test.com?secret=JBSWY3DPEHPK3PXP&issuer=AYB",
  secret: "JBSWY3DPEHPK3PXP",
};

const BACKUP_CODES = [
  "abc12-def34",
  "ghi56-jkl78",
  "mno90-pqr12",
  "stu34-vwx56",
  "yza78-bcd90",
  "efg12-hij34",
  "klm56-nop78",
  "qrs90-tuv12",
  "wxy34-zab56",
  "cde78-fgh90",
];

describe("MFAEnrollment", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockGetMFAFactors.mockResolvedValue({ factors: [] });
    mockGetBackupCodeCount.mockResolvedValue({ remaining: 0 });
  });

  it("renders heading and shows no enrolled factors initially", async () => {
    render(<MFAEnrollment />);
    await waitFor(() => {
      expect(screen.getByRole("heading", { name: /multi-factor authentication/i })).toBeInTheDocument();
    });
    expect(screen.getByText(/no mfa methods enrolled/i)).toBeInTheDocument();
  });

  it("shows enrolled factors when they exist", async () => {
    const factors: MFAFactor[] = [
      { id: "f1", method: "totp" },
      { id: "f2", method: "sms", phone: "***1234" },
    ];
    mockGetMFAFactors.mockResolvedValue({ factors });
    render(<MFAEnrollment />);
    await waitFor(() => {
      const enrolled = screen.getByTestId("mfa-enrolled-methods");
      expect(within(enrolled).getByText(/authenticator app/i)).toBeInTheDocument();
      expect(within(enrolled).getByText(/^sms$/i)).toBeInTheDocument();
    });
  });

  it("scopes enrolled email factor assertions to enrolled methods container", async () => {
    render(<MFAEnrollment />);
    await waitFor(() => {
      expect(screen.getByRole("button", { name: /set up email mfa/i })).toBeInTheDocument();
    });
    const enrolled = screen.getByTestId("mfa-enrolled-methods");
    expect(within(enrolled).queryByText(/^email$/i)).not.toBeInTheDocument();
  });

  // TOTP enrollment flow
  describe("TOTP enrollment", () => {
    it("starts TOTP enrollment and shows secret + QR URI", async () => {
      mockEnrollTOTP.mockResolvedValue(TOTP_ENROLLMENT);
      const user = userEvent.setup();
      render(<MFAEnrollment />);
      await waitFor(() => expect(mockGetMFAFactors).toHaveBeenCalled());

      const enrollButton = screen.getByRole("button", { name: /set up authenticator/i });
      await user.click(enrollButton);

      await waitFor(() => {
        expect(screen.getByText(TOTP_ENROLLMENT.secret)).toBeInTheDocument();
      });
      expect(screen.getByTestId("totp-uri")).toHaveTextContent(TOTP_ENROLLMENT.uri);
    });

    it("confirms TOTP enrollment with valid code", async () => {
      mockEnrollTOTP.mockResolvedValue(TOTP_ENROLLMENT);
      mockConfirmTOTPEnroll.mockResolvedValue({ message: "TOTP MFA enrollment confirmed" });
      const user = userEvent.setup();
      render(<MFAEnrollment />);
      await waitFor(() => expect(mockGetMFAFactors).toHaveBeenCalled());

      await user.click(screen.getByRole("button", { name: /set up authenticator/i }));
      await waitFor(() => expect(screen.getByText(TOTP_ENROLLMENT.secret)).toBeInTheDocument());

      const codeInput = screen.getByTestId("totp-confirm-code");
      await user.type(codeInput, "123456");
      await user.click(screen.getByRole("button", { name: /verify code/i }));

      await waitFor(() => {
        expect(mockConfirmTOTPEnroll).toHaveBeenCalledWith("123456");
      });
      await waitFor(() => {
        expect(screen.getByText(/totp.*enrolled/i)).toBeInTheDocument();
      });
    });

    it("shows error when TOTP enrollment fails", async () => {
      mockEnrollTOTP.mockRejectedValue(new Error("TOTP MFA already enrolled"));
      const user = userEvent.setup();
      render(<MFAEnrollment />);
      await waitFor(() => expect(mockGetMFAFactors).toHaveBeenCalled());

      await user.click(screen.getByRole("button", { name: /set up authenticator/i }));
      await waitFor(() => {
        expect(screen.getByText(/already enrolled/i)).toBeInTheDocument();
      });
    });

    it("shows error when TOTP confirm fails with invalid code", async () => {
      mockEnrollTOTP.mockResolvedValue(TOTP_ENROLLMENT);
      mockConfirmTOTPEnroll.mockRejectedValue(new Error("invalid TOTP code"));
      const user = userEvent.setup();
      render(<MFAEnrollment />);
      await waitFor(() => expect(mockGetMFAFactors).toHaveBeenCalled());

      await user.click(screen.getByRole("button", { name: /set up authenticator/i }));
      await waitFor(() => expect(screen.getByText(TOTP_ENROLLMENT.secret)).toBeInTheDocument());

      await user.type(screen.getByTestId("totp-confirm-code"), "000000");
      await user.click(screen.getByRole("button", { name: /verify code/i }));

      await waitFor(() => {
        expect(screen.getByText(/invalid.*code/i)).toBeInTheDocument();
      });
    });
  });

  // Email MFA enrollment flow
  describe("Email MFA enrollment", () => {
    it("starts email MFA enrollment and shows code input", async () => {
      mockEnrollEmailMFA.mockResolvedValue({ message: "verification code sent to your email" });
      const user = userEvent.setup();
      render(<MFAEnrollment />);
      await waitFor(() => expect(mockGetMFAFactors).toHaveBeenCalled());

      await user.click(screen.getByRole("button", { name: /set up email mfa/i }));
      await waitFor(() => {
        expect(screen.getByText(/^verification code sent to your email$/i)).toBeInTheDocument();
      });
      expect(screen.getByTestId("email-mfa-confirm-code")).toBeInTheDocument();
    });

    it("confirms email MFA enrollment with valid code", async () => {
      mockEnrollEmailMFA.mockResolvedValue({ message: "verification code sent to your email" });
      mockConfirmEmailMFAEnroll.mockResolvedValue({ message: "email MFA enrollment confirmed" });
      const user = userEvent.setup();
      render(<MFAEnrollment />);
      await waitFor(() => expect(mockGetMFAFactors).toHaveBeenCalled());

      await user.click(screen.getByRole("button", { name: /set up email mfa/i }));
      await waitFor(() => expect(screen.getByTestId("email-mfa-confirm-code")).toBeInTheDocument());

      await user.type(screen.getByTestId("email-mfa-confirm-code"), "654321");
      await user.click(screen.getByRole("button", { name: /confirm email mfa/i }));

      await waitFor(() => {
        expect(mockConfirmEmailMFAEnroll).toHaveBeenCalledWith("654321");
      });
      await waitFor(() => {
        expect(screen.getByText(/email mfa.*enrolled/i)).toBeInTheDocument();
      });
    });
  });

  // Backup codes
  describe("Backup codes", () => {
    it("generates and displays backup codes", async () => {
      mockGenerateBackupCodes.mockResolvedValue({ codes: BACKUP_CODES });
      // Need at least one MFA factor enrolled to generate backup codes
      mockGetMFAFactors.mockResolvedValue({ factors: [{ id: "f1", method: "totp" }] });
      const user = userEvent.setup();
      render(<MFAEnrollment />);
      await waitFor(() => expect(mockGetMFAFactors).toHaveBeenCalled());

      await user.click(screen.getByRole("button", { name: /generate backup codes/i }));
      await waitFor(() => {
        expect(mockGenerateBackupCodes).toHaveBeenCalled();
      });
      for (const code of BACKUP_CODES) {
        expect(screen.getByText(code)).toBeInTheDocument();
      }
    });

    it("shows remaining backup code count", async () => {
      mockGetMFAFactors.mockResolvedValue({ factors: [{ id: "f1", method: "totp" }] });
      mockGetBackupCodeCount.mockResolvedValue({ remaining: 8 });
      render(<MFAEnrollment />);
      await waitFor(() => {
        expect(screen.getByText(/8.*remaining/i)).toBeInTheDocument();
      });
    });

    it("regenerates backup codes", async () => {
      mockGetMFAFactors.mockResolvedValue({ factors: [{ id: "f1", method: "totp" }] });
      mockGetBackupCodeCount.mockResolvedValue({ remaining: 5 });
      mockRegenerateBackupCodes.mockResolvedValue({ codes: BACKUP_CODES });
      const user = userEvent.setup();
      render(<MFAEnrollment />);
      await waitFor(() => expect(screen.getByText(/5.*remaining/i)).toBeInTheDocument());

      await user.click(screen.getByRole("button", { name: /regenerate/i }));
      await waitFor(() => {
        expect(mockRegenerateBackupCodes).toHaveBeenCalled();
      });
      for (const code of BACKUP_CODES) {
        expect(screen.getByText(code)).toBeInTheDocument();
      }
    });

    it("updates backup count after generating codes", async () => {
      mockGetMFAFactors.mockResolvedValue({ factors: [{ id: "f1", method: "totp" }] });
      mockGetBackupCodeCount.mockResolvedValue({ remaining: 0 });
      mockGenerateBackupCodes.mockResolvedValue({ codes: BACKUP_CODES });
      const user = userEvent.setup();
      render(<MFAEnrollment />);

      await waitFor(() => expect(screen.getByText(/0.*remaining/i)).toBeInTheDocument());

      await user.click(screen.getByRole("button", { name: /generate backup codes/i }));
      await waitFor(() => expect(screen.getByText(BACKUP_CODES[0])).toBeInTheDocument());

      await user.click(screen.getByRole("button", { name: /^done$/i }));

      await waitFor(() => {
        expect(screen.getByText(/10.*backup codes remaining/i)).toBeInTheDocument();
      });
      expect(screen.getByRole("button", { name: /regenerate/i })).toBeInTheDocument();
    });

    it("keeps regenerated backup codes visible when refresh data is slow", async () => {
      mockGetMFAFactors
        .mockResolvedValueOnce({ factors: [{ id: "f1", method: "totp" }] })
        .mockReturnValueOnce(new Promise(() => {}));
      mockGetBackupCodeCount
        .mockResolvedValueOnce({ remaining: 5 })
        .mockReturnValueOnce(new Promise(() => {}));
      mockRegenerateBackupCodes.mockResolvedValue({ codes: BACKUP_CODES });

      const user = userEvent.setup();
      render(<MFAEnrollment />);
      await waitFor(() => expect(screen.getByText(/5.*remaining/i)).toBeInTheDocument());

      await user.click(screen.getByRole("button", { name: /regenerate/i }));

      await waitFor(() => {
        expect(mockRegenerateBackupCodes).toHaveBeenCalled();
      });
      expect(screen.getByText(BACKUP_CODES[0])).toBeInTheDocument();
      expect(screen.queryByText(/^loading/i)).not.toBeInTheDocument();
    });
  });

  it("shows loading state while fetching factors", () => {
    mockGetMFAFactors.mockReturnValue(new Promise(() => {})); // never resolves
    render(<MFAEnrollment />);
    expect(screen.getByText(/loading/i)).toBeInTheDocument();
  });

  it("keeps page heading visible while initial data is loading", () => {
    mockGetMFAFactors.mockReturnValue(new Promise(() => {})); // never resolves
    render(<MFAEnrollment />);
    expect(screen.getByRole("heading", { name: /multi-factor authentication/i })).toBeInTheDocument();
    expect(screen.getByText(/loading/i)).toBeInTheDocument();
  });

  it("shows error state when factor fetch fails", async () => {
    mockGetMFAFactors.mockRejectedValue(new Error("network error"));
    render(<MFAEnrollment />);
    await waitFor(() => {
      expect(screen.getByText(/network error/i)).toBeInTheDocument();
    });
  });

  it("treats null factors payload as empty list instead of crashing", async () => {
    mockGetMFAFactors.mockResolvedValue({ factors: null as unknown as MFAFactor[] });
    render(<MFAEnrollment />);
    await waitFor(() => {
      expect(screen.getByRole("heading", { name: /multi-factor authentication/i })).toBeInTheDocument();
    });
    expect(screen.getByText(/no mfa methods enrolled/i)).toBeInTheDocument();
  });
});
