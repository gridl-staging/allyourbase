import { describe, it, expect, vi, beforeEach } from "vitest";
import { screen, fireEvent, waitFor } from "@testing-library/react";
import { renderWithProviders } from "../../test-utils";
import { Notifications } from "../Notifications";
import { ApiError } from "../../api_client";

vi.mock("../../api_notifications", () => ({
  createNotification: vi.fn(),
}));

import * as api from "../../api_notifications";

type NotificationRecord = {
  id: string;
  user_id: string;
  title: string;
  body: string;
  channel: string;
  created_at: string;
};

type NotificationFormValues = {
  userId: string;
  title: string;
  body?: string;
  channel: string;
};

const createNotificationMock = vi.mocked(api.createNotification);
const defaultNotification = notificationRecord();

function notificationRecord(overrides: Partial<NotificationRecord> = {}): NotificationRecord {
  return {
    id: "n-1",
    user_id: "user-1",
    title: "Test",
    body: "Hello",
    channel: "email",
    created_at: "2026-03-12T14:00:00Z",
    ...overrides,
  };
}

function createDeferredNotification() {
  let resolvePromise: ((value: NotificationRecord) => void) | undefined;
  const promise = new Promise<NotificationRecord>((resolve) => {
    resolvePromise = resolve;
  });

  return {
    promise,
    resolve(value: NotificationRecord) {
      resolvePromise?.(value);
    },
  };
}

function renderNotifications() {
  renderWithProviders(<Notifications />);
}

function notificationButton() {
  return screen.getByRole("button", { name: /send notification/i });
}

function fillNotificationForm({ userId, title, body, channel }: Partial<NotificationFormValues>) {
  if (userId !== undefined) {
    fireEvent.change(screen.getByLabelText("User ID"), { target: { value: userId } });
  }
  if (title !== undefined) {
    fireEvent.change(screen.getByLabelText("Title"), { target: { value: title } });
  }
  if (body !== undefined) {
    fireEvent.change(screen.getByLabelText("Body"), { target: { value: body } });
  }
  if (channel !== undefined) {
    fireEvent.change(screen.getByLabelText("Channel"), { target: { value: channel } });
  }
}

function expectFormValues({ userId, title, body, channel }: NotificationFormValues) {
  expect(screen.getByLabelText("User ID")).toHaveValue(userId);
  expect(screen.getByLabelText("Title")).toHaveValue(title);
  expect(screen.getByLabelText("Body")).toHaveValue(body ?? "");
  expect(screen.getByLabelText("Channel")).toHaveValue(channel);
}

beforeEach(() => {
  vi.clearAllMocks();
  createNotificationMock.mockResolvedValue(defaultNotification);
});

describe("Notifications", () => {
  it("validates user_id, title, and channel are required", () => {
    renderNotifications();
    const submitBtn = notificationButton();
    expect(submitBtn).toBeDisabled();
  });

  it("submits form and calls createNotification", async () => {
    renderNotifications();
    fillNotificationForm({
      userId: "user-1",
      title: "Test Alert",
      body: "Hello world",
      channel: "email",
    });

    fireEvent.click(notificationButton());

    await waitFor(() => {
      expect(createNotificationMock).toHaveBeenCalledWith({
        user_id: "user-1",
        title: "Test Alert",
        body: "Hello world",
        channel: "email",
      });
    });
  });

  it("shows success feedback after sending", async () => {
    renderNotifications();
    fillNotificationForm({
      userId: "user-1",
      title: "Test",
      channel: "push",
    });

    fireEvent.click(notificationButton());

    await waitFor(() => {
      expect(screen.getByText(/notification sent/i)).toBeInTheDocument();
    });
  });

  it("shows an accessible error, preserves form values, and re-enables submit after failure", async () => {
    createNotificationMock.mockRejectedValueOnce(
      new ApiError(500, "delivery service down"),
    );

    renderNotifications();
    fillNotificationForm({
      userId: "user-1",
      title: "Test Alert",
      body: "Hello world",
      channel: "email",
    });

    const submitBtn = notificationButton();
    fireEvent.click(submitBtn);
    expect(submitBtn).toBeDisabled();

    await waitFor(() => {
      expect(screen.getByRole("alert")).toHaveTextContent("Failed to send notification.");
    });

    expect(submitBtn).toBeEnabled();
    expectFormValues({
      userId: "user-1",
      title: "Test Alert",
      body: "Hello world",
      channel: "email",
    });
  });

  it("clears stale error feedback when a new submit starts", async () => {
    const secondSubmit = createDeferredNotification();

    createNotificationMock
      .mockRejectedValueOnce(new ApiError(500, "delivery service down"))
      .mockImplementationOnce(() => secondSubmit.promise);

    renderNotifications();
    fillNotificationForm({
      userId: "user-1",
      title: "Test Alert",
      body: "Hello world",
      channel: "email",
    });

    const submitBtn = notificationButton();
    fireEvent.click(submitBtn);

    await waitFor(() => {
      expect(screen.getByRole("alert")).toHaveTextContent("Failed to send notification.");
    });

    fireEvent.click(submitBtn);
    expect(screen.queryByRole("alert")).not.toBeInTheDocument();
    expect(submitBtn).toBeDisabled();

    secondSubmit.resolve(
      notificationRecord({
        id: "n-2",
        user_id: "user-1",
        title: "Test Alert",
        body: "Hello world",
        channel: "email",
      }),
    );

    await waitFor(() => {
      expect(screen.getByText(/notification sent/i)).toBeInTheDocument();
    });
  });

  it("clears stale success feedback when a new submit starts", async () => {
    const secondSubmit = createDeferredNotification();

    createNotificationMock
      .mockResolvedValueOnce(
        notificationRecord({
          title: "Test Alert",
          body: "Hello world",
        }),
      )
      .mockImplementationOnce(() => secondSubmit.promise);

    renderNotifications();
    fillNotificationForm({
      userId: "user-1",
      title: "Test Alert",
      channel: "email",
    });

    const submitBtn = notificationButton();
    fireEvent.click(submitBtn);

    await waitFor(() => {
      expect(screen.getByRole("status")).toHaveTextContent("Notification sent successfully.");
    });

    fireEvent.change(screen.getByLabelText("User ID"), {
      target: { value: "user-2" },
    });
    fireEvent.change(screen.getByLabelText("Title"), {
      target: { value: "Second Alert" },
    });
    fireEvent.change(screen.getByLabelText("Channel"), {
      target: { value: "push" },
    });

    fireEvent.click(submitBtn);
    expect(screen.queryByRole("status")).not.toBeInTheDocument();
    expect(submitBtn).toBeDisabled();

    secondSubmit.resolve(
      notificationRecord({
        id: "n-2",
        user_id: "user-2",
        title: "Second Alert",
        body: "",
        channel: "push",
        created_at: "2026-03-12T15:00:00Z",
      }),
    );

    await waitFor(() => {
      expect(screen.getByRole("status")).toHaveTextContent("Notification sent successfully.");
    });
  });
});
