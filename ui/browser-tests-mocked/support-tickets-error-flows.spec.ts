import { test, expect, bootstrapMockedAdminApp, mockAdminSupportTicketApis } from "./fixtures";

test.describe("Support Tickets Error Flows (Browser Mocked)", () => {
  test.beforeEach(async ({ page }) => {
    await bootstrapMockedAdminApp(page);
  });

  test("list-500: shows error text with heading", async ({ page }) => {
    await mockAdminSupportTicketApis(page, {
      listResponder: () => ({ status: 500, body: { message: "failed to list support tickets" } }),
    });
    await page.goto("/admin/");
    await page.getByRole("button", { name: /Support Tickets/i }).click();

    await expect(page.getByRole("heading", { name: /Support Tickets/i })).toBeVisible();
    await expect(page.getByText(/failed to list support tickets/i)).toBeVisible();
  });

  test("empty-state: shows AdminTable empty message", async ({ page }) => {
    await mockAdminSupportTicketApis(page);
    await page.goto("/admin/");
    await page.getByRole("button", { name: /Support Tickets/i }).click();

    await expect(page.getByRole("heading", { name: /Support Tickets/i })).toBeVisible();
    await expect(page.getByText(/No support tickets/i)).toBeVisible();
  });

  test("update-500: shows error after status change attempt", async ({ page }) => {
    // Provide a ticket in the list, then fail the update
    await mockAdminSupportTicketApis(page, {
      listResponder: () => ({
        status: 200,
        body: [
          {
            id: "ticket-001",
            tenant_id: "tenant-001",
            user_id: "user-001",
            subject: "Login broken",
            status: "open",
            priority: "high",
            created_at: "2026-02-28T12:00:00Z",
            updated_at: "2026-02-28T12:00:00Z",
          },
        ],
      }),
      updateResponder: () => ({ status: 500, body: { message: "database write failed" } }),
    });
    await page.goto("/admin/");
    await page.getByRole("button", { name: /Support Tickets/i }).click();

    // Expand ticket detail to access status/priority selects
    await page.getByText("Details").click();
    await expect(page.getByPlaceholder(/Type a reply/i)).toBeVisible();

    const statusSelect = page.getByLabel(/Ticket status/i);
    await statusSelect.selectOption("resolved");

    await expect(page.getByText(/database write failed/i)).toBeVisible();
  });

  test("reply-500: shows error after sending reply", async ({ page }) => {
    await mockAdminSupportTicketApis(page, {
      listResponder: () => ({
        status: 200,
        body: [
          {
            id: "ticket-002",
            tenant_id: "tenant-001",
            user_id: "user-001",
            subject: "Cannot upload files",
            status: "open",
            priority: "normal",
            created_at: "2026-02-28T12:00:00Z",
            updated_at: "2026-02-28T12:00:00Z",
          },
        ],
      }),
      replyResponder: () => ({ status: 500, body: { message: "message delivery failed" } }),
    });
    await page.goto("/admin/");
    await page.getByRole("button", { name: /Support Tickets/i }).click();

    // Expand ticket
    await page.getByText("Details").click();

    // Type a reply and send
    await page.getByPlaceholder(/Type a reply/i).fill("We're looking into this.");
    await page.getByRole("button", { name: /Send/i }).click();

    await expect(page.getByText(/message delivery failed/i)).toBeVisible();
  });
});
