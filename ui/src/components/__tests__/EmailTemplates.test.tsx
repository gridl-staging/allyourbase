import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { render, screen, waitFor, fireEvent, act } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { EmailTemplates } from "../EmailTemplates";
import {
  listEmailTemplates,
  getEmailTemplate,
  upsertEmailTemplate,
  deleteEmailTemplate,
  setEmailTemplateEnabled,
  previewEmailTemplate,
  sendTemplateEmail,
} from "../../api";
import type {
  EmailTemplateEffective,
  EmailTemplateListItem,
  EmailTemplateListResponse,
  PreviewEmailTemplateResponse,
} from "../../types";

vi.mock("../../api", () => ({
  listEmailTemplates: vi.fn(),
  getEmailTemplate: vi.fn(),
  upsertEmailTemplate: vi.fn(),
  deleteEmailTemplate: vi.fn(),
  setEmailTemplateEnabled: vi.fn(),
  previewEmailTemplate: vi.fn(),
  sendTemplateEmail: vi.fn(),
}));

vi.mock("../Toast", () => ({
  ToastContainer: () => null,
  useToast: () => ({
    toasts: [],
    addToast: vi.fn(),
    removeToast: vi.fn(),
  }),
}));

const mockListEmailTemplates = vi.mocked(listEmailTemplates);
const mockGetEmailTemplate = vi.mocked(getEmailTemplate);
const mockUpsertEmailTemplate = vi.mocked(upsertEmailTemplate);
const mockDeleteEmailTemplate = vi.mocked(deleteEmailTemplate);
const mockSetEmailTemplateEnabled = vi.mocked(setEmailTemplateEnabled);
const mockPreviewEmailTemplate = vi.mocked(previewEmailTemplate);
const mockSendTemplateEmail = vi.mocked(sendTemplateEmail);

function makeListItem(overrides: Partial<EmailTemplateListItem> = {}): EmailTemplateListItem {
  return {
    templateKey: "auth.password_reset",
    source: "builtin",
    subjectTemplate: "Reset your password",
    enabled: true,
    updatedAt: "2026-02-22T09:00:00Z",
    ...overrides,
  };
}

function makeEffective(overrides: Partial<EmailTemplateEffective> = {}): EmailTemplateEffective {
  return {
    source: "builtin",
    templateKey: "auth.password_reset",
    subjectTemplate: "Reset your password",
    htmlTemplate: "<p>Click {{.ActionURL}}</p>",
    enabled: true,
    variables: ["AppName", "ActionURL"],
    ...overrides,
  };
}

function makePreview(overrides: Partial<PreviewEmailTemplateResponse> = {}): PreviewEmailTemplateResponse {
  return {
    subject: "Reset your password",
    html: "<p>Click https://example.com</p>",
    text: "Click https://example.com",
    ...overrides,
  };
}

describe("EmailTemplates", () => {
  beforeEach(() => {
    vi.clearAllMocks();

    const listResponse: EmailTemplateListResponse = {
      items: [
        makeListItem(),
        makeListItem({
          templateKey: "app.club_invite",
          source: "custom",
          subjectTemplate: "You're invited",
          enabled: false,
        }),
      ],
      count: 2,
    };

    mockListEmailTemplates.mockResolvedValue(listResponse);
    mockGetEmailTemplate.mockResolvedValue(makeEffective());
    mockUpsertEmailTemplate.mockResolvedValue(
      makeEffective({ source: "custom" }),
    );
    mockDeleteEmailTemplate.mockResolvedValue(undefined);
    mockSetEmailTemplateEnabled.mockResolvedValue({
      templateKey: "app.club_invite",
      enabled: true,
    });
    mockPreviewEmailTemplate.mockResolvedValue(makePreview());
    mockSendTemplateEmail.mockResolvedValue({ status: "sent" });
  });

  afterEach(() => {
    vi.useRealTimers();
  });

  it("renders template list and loads first effective template", async () => {
    render(<EmailTemplates />);

    await waitFor(() => {
      expect(screen.getByRole("heading", { name: "Email Templates" })).toBeInTheDocument();
      expect(screen.getByText("auth.password_reset")).toBeInTheDocument();
      expect(screen.getByText("app.club_invite")).toBeInTheDocument();
    });

    await waitFor(() => {
      expect(mockGetEmailTemplate).toHaveBeenCalledWith("auth.password_reset");
    });
  });

  it("switches selected template and loads editor values", async () => {
    mockGetEmailTemplate
      .mockResolvedValueOnce(makeEffective())
      .mockResolvedValueOnce(
        makeEffective({
          source: "custom",
          templateKey: "app.club_invite",
          subjectTemplate: "Invite {{.Name}}",
          htmlTemplate: "<p>Hello {{.Name}}</p>",
          enabled: false,
          variables: [],
        }),
      );

    render(<EmailTemplates />);

    const user = userEvent.setup();
    await waitFor(() => {
      expect(screen.getByText("app.club_invite")).toBeInTheDocument();
    });

    await user.click(screen.getByText("app.club_invite"));

    await waitFor(() => {
      expect(mockGetEmailTemplate).toHaveBeenCalledWith("app.club_invite");
      expect(screen.getByLabelText("Subject Template")).toHaveValue("Invite {{.Name}}");
      expect(screen.getByLabelText("HTML Template")).toHaveValue("<p>Hello {{.Name}}</p>");
    });
  });

  it("toggles enabled state for custom templates", async () => {
    mockGetEmailTemplate
      .mockResolvedValueOnce(makeEffective())
      .mockResolvedValueOnce(
        makeEffective({
          source: "custom",
          templateKey: "app.club_invite",
          subjectTemplate: "Invite {{.Name}}",
          htmlTemplate: "<p>Hello {{.Name}}</p>",
          enabled: false,
          variables: [],
        }),
      );

    render(<EmailTemplates />);

    const user = userEvent.setup();
    await waitFor(() => {
      expect(screen.getByText("app.club_invite")).toBeInTheDocument();
    });

    await user.click(screen.getByText("app.club_invite"));
    await waitFor(() => {
      expect(screen.getByRole("button", { name: "Enable Override" })).toBeInTheDocument();
    });

    await user.click(screen.getByRole("button", { name: "Enable Override" }));

    await waitFor(() => {
      expect(mockSetEmailTemplateEnabled).toHaveBeenCalledWith("app.club_invite", true);
    });
  });

  it("debounces preview and sends latest template values", async () => {
    mockGetEmailTemplate
      .mockResolvedValueOnce(makeEffective())
      .mockResolvedValueOnce(
        makeEffective({
          source: "custom",
          templateKey: "app.club_invite",
          subjectTemplate: "Invite {{.Name}}",
          htmlTemplate: "<p>Hello {{.Name}}</p>",
          enabled: true,
          variables: [],
        }),
      );

    render(<EmailTemplates />);

    await waitFor(() => {
      expect(screen.getByText("app.club_invite")).toBeInTheDocument();
    });

    fireEvent.click(screen.getByText("app.club_invite"));

    await waitFor(() => {
      expect(screen.getByLabelText("Subject Template")).toHaveValue("Invite {{.Name}}");
    });

    vi.useFakeTimers();
    mockPreviewEmailTemplate.mockClear();

    fireEvent.change(screen.getByLabelText("Subject Template"), {
      target: { value: "Invite NOW {{.Name}}" },
    });
    fireEvent.change(screen.getByLabelText("Subject Template"), {
      target: { value: "Invite FINAL {{.Name}}" },
    });
    fireEvent.change(screen.getByLabelText("Preview Variables (JSON)"), {
      target: { value: '{"Name":"Alex"}' },
    });

    expect(mockPreviewEmailTemplate).not.toHaveBeenCalled();

    await act(async () => {
      vi.advanceTimersByTime(450);
      await Promise.resolve();
    });

    expect(mockPreviewEmailTemplate).toHaveBeenCalledTimes(1);
    expect(mockPreviewEmailTemplate).toHaveBeenCalledWith("app.club_invite", {
      subjectTemplate: "Invite FINAL {{.Name}}",
      htmlTemplate: "<p>Hello {{.Name}}</p>",
      variables: { Name: "Alex" },
    });
  });

  it("sends a test email for the selected template", async () => {
    mockGetEmailTemplate
      .mockResolvedValueOnce(makeEffective())
      .mockResolvedValueOnce(
        makeEffective({
          source: "custom",
          templateKey: "app.club_invite",
          subjectTemplate: "Invite {{.Name}}",
          htmlTemplate: "<p>Hello {{.Name}}</p>",
          enabled: true,
          variables: [],
        }),
      );

    render(<EmailTemplates />);

    const user = userEvent.setup();
    await waitFor(() => {
      expect(screen.getByText("app.club_invite")).toBeInTheDocument();
    });

    await user.click(screen.getByText("app.club_invite"));
    await waitFor(() => {
      expect(screen.getByLabelText("Subject Template")).toHaveValue("Invite {{.Name}}");
    });

    await user.type(screen.getByLabelText("Test Recipient"), "user@example.com");
    await user.click(screen.getByRole("button", { name: "Send Test Email" }));

    await waitFor(() => {
      expect(mockSendTemplateEmail).toHaveBeenCalledWith({
        templateKey: "app.club_invite",
        to: "user@example.com",
        variables: {},
      });
    });
  });
});
