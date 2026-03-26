import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { screen, waitFor, fireEvent, act } from "@testing-library/react";
import { renderWithProviders } from "../../test-utils";
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

const mockAddToast = vi.fn();
const mockRemoveToast = vi.fn();

vi.mock("../Toast", () => ({
  ToastContainer: () => null,
  useToast: () => ({
    toasts: [],
    addToast: mockAddToast,
    removeToast: mockRemoveToast,
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

function deferred<T>() {
  let resolve!: (value: T | PromiseLike<T>) => void;
  let reject!: (reason?: unknown) => void;
  const promise = new Promise<T>((res, rej) => {
    resolve = res;
    reject = rej;
  });
  return { promise, resolve, reject };
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
    renderWithProviders(<EmailTemplates />);

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

    renderWithProviders(<EmailTemplates />);

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

    renderWithProviders(<EmailTemplates />);

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

    renderWithProviders(<EmailTemplates />);

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

    renderWithProviders(<EmailTemplates />);

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

  it("renders preview HTML in a dedicated output container for unambiguous assertions", async () => {
    mockPreviewEmailTemplate
      .mockResolvedValueOnce(makePreview({ html: "<p>initial</p>" }))
      .mockResolvedValueOnce(makePreview({ html: "<p>https://sigil.example/reset/123</p>" }));

    renderWithProviders(<EmailTemplates />);

    await waitFor(() => {
      expect(screen.getByLabelText("Preview Variables (JSON)")).toBeInTheDocument();
    });
    await waitFor(() => {
      expect(mockPreviewEmailTemplate).toHaveBeenCalledTimes(1);
      expect(screen.getByTestId("email-template-preview-html")).toHaveTextContent("initial");
    });

    fireEvent.change(screen.getByLabelText("Preview Variables (JSON)"), {
      target: {
        value: JSON.stringify({
          AppName: "Sigil 123",
          ActionURL: "https://sigil.example/reset/123",
        }),
      },
    });

    await waitFor(() => {
      expect(mockPreviewEmailTemplate).toHaveBeenCalledTimes(2);
    });
    expect(mockPreviewEmailTemplate).toHaveBeenLastCalledWith("auth.password_reset", {
      subjectTemplate: "Reset your password",
      htmlTemplate: "<p>Click {{.ActionURL}}</p>",
      variables: {
        AppName: "Sigil 123",
        ActionURL: "https://sigil.example/reset/123",
      },
    });

    await waitFor(() => {
      expect(screen.getByTestId("email-template-preview-html")).toHaveTextContent(
        "https://sigil.example/reset/123",
      );
    });
  });

  it("reloads effective template after reset to default", async () => {
    mockListEmailTemplates
      .mockResolvedValueOnce({
        items: [
          makeListItem({
            templateKey: "auth.password_reset",
            source: "custom",
            subjectTemplate: "Custom subject",
            enabled: true,
          }),
        ],
        count: 1,
      })
      .mockResolvedValueOnce({
        items: [makeListItem({ templateKey: "auth.password_reset", source: "builtin" })],
        count: 1,
      });

    mockGetEmailTemplate
      .mockResolvedValueOnce(
        makeEffective({
          source: "custom",
          templateKey: "auth.password_reset",
          subjectTemplate: "Custom subject",
          htmlTemplate: "<p>Custom body</p>",
        }),
      )
      .mockResolvedValueOnce(
        makeEffective({
          source: "builtin",
          templateKey: "auth.password_reset",
          subjectTemplate: "Reset your password",
          htmlTemplate: "<p>Click {{.ActionURL}}</p>",
        }),
      );

    renderWithProviders(<EmailTemplates />);

    const user = userEvent.setup();

    await waitFor(() => {
      expect(screen.getByRole("heading", { name: "auth.password_reset" })).toBeInTheDocument();
      expect(screen.getByLabelText("Subject Template")).toHaveValue("Custom subject");
      expect(screen.getByRole("button", { name: "Reset to Default" })).toBeInTheDocument();
    });

    await user.click(screen.getByRole("button", { name: "Reset to Default" }));

    await waitFor(() => {
      expect(mockDeleteEmailTemplate).toHaveBeenCalledWith("auth.password_reset");
      expect(mockGetEmailTemplate).toHaveBeenLastCalledWith("auth.password_reset");
      expect(screen.getByLabelText("Subject Template")).toHaveValue("Reset your password");
    });
  });

  it("does not emit an error toast when deleting a custom template that no longer has an effective record", async () => {
    mockListEmailTemplates
      .mockResolvedValueOnce({
        items: [
          makeListItem({
            templateKey: "app.club_invite",
            source: "custom",
            subjectTemplate: "Invite {{.Name}}",
            enabled: true,
          }),
          makeListItem({
            templateKey: "auth.password_reset",
            source: "builtin",
          }),
        ],
        count: 2,
      })
      .mockResolvedValueOnce({
        items: [
          makeListItem({
            templateKey: "auth.password_reset",
            source: "builtin",
          }),
        ],
        count: 1,
      });

    mockGetEmailTemplate
      .mockResolvedValueOnce(
        makeEffective({
          templateKey: "app.club_invite",
          source: "custom",
          subjectTemplate: "Invite {{.Name}}",
          htmlTemplate: "<p>Hello {{.Name}}</p>",
          variables: [],
        }),
      )
      .mockRejectedValueOnce(new Error("template not found"))
      .mockResolvedValueOnce(
        makeEffective({
          templateKey: "auth.password_reset",
          source: "builtin",
          subjectTemplate: "Reset your password",
          htmlTemplate: "<p>Click {{.ActionURL}}</p>",
        }),
      );

    renderWithProviders(<EmailTemplates />);
    const user = userEvent.setup();

    await waitFor(() => {
      expect(screen.getByRole("heading", { name: "app.club_invite" })).toBeInTheDocument();
      expect(screen.getByRole("button", { name: "Delete Template" })).toBeInTheDocument();
    });

    await user.click(screen.getByRole("button", { name: "Delete Template" }));

    await waitFor(() => {
      expect(mockDeleteEmailTemplate).toHaveBeenCalledWith("app.club_invite");
      expect(screen.getByRole("heading", { name: "auth.password_reset" })).toBeInTheDocument();
    });

    expect(screen.getByText("Deleted app.club_invite")).toBeInTheDocument();
    expect(screen.queryByText("template not found")).not.toBeInTheDocument();
  });

  it("ignores stale effective-load failures from a deleted template after selection moves", async () => {
    const deletedTemplateReload = deferred<EmailTemplateEffective>();

    mockListEmailTemplates
      .mockResolvedValueOnce({
        items: [
          makeListItem({
            templateKey: "app.club_invite",
            source: "custom",
            subjectTemplate: "Invite {{.Name}}",
            enabled: true,
          }),
          makeListItem({
            templateKey: "auth.password_reset",
            source: "builtin",
          }),
        ],
        count: 2,
      })
      .mockResolvedValueOnce({
        items: [
          makeListItem({
            templateKey: "auth.password_reset",
            source: "builtin",
          }),
        ],
        count: 1,
      });

    let appLoads = 0;
    mockGetEmailTemplate.mockImplementation(async (key) => {
      if (key === "app.club_invite") {
        appLoads += 1;
        if (appLoads === 1) {
          return makeEffective({
            templateKey: "app.club_invite",
            source: "custom",
            subjectTemplate: "Invite {{.Name}}",
            htmlTemplate: "<p>Hello {{.Name}}</p>",
            variables: [],
          });
        }
        return deletedTemplateReload.promise;
      }

      if (key === "auth.password_reset") {
        return makeEffective({
          templateKey: "auth.password_reset",
          source: "builtin",
          subjectTemplate: "Reset your password",
          htmlTemplate: "<p>Click {{.ActionURL}}</p>",
        });
      }

      throw new Error(`unexpected key ${key}`);
    });

    renderWithProviders(<EmailTemplates />);
    const user = userEvent.setup();

    await waitFor(() => {
      expect(screen.getByRole("heading", { name: "app.club_invite" })).toBeInTheDocument();
    });

    await user.click(screen.getByRole("button", { name: "Delete Template" }));

    await waitFor(() => {
      expect(mockDeleteEmailTemplate).toHaveBeenCalledWith("app.club_invite");
      expect(screen.getByRole("heading", { name: "auth.password_reset" })).toBeInTheDocument();
      expect(screen.getByLabelText("Subject Template")).toHaveValue("Reset your password");
    });

    await act(async () => {
      deletedTemplateReload.reject(new Error("template not found"));
      await Promise.resolve();
    });

    expect(screen.queryByText("template not found")).not.toBeInTheDocument();
    expect(screen.getByLabelText("Subject Template")).toHaveValue("Reset your password");
  });
});
