import {
  test,
  expect,
  probeEndpoint,
  seedSupportTicket,
  cleanupSupportTicketByID,
  waitForDashboard,
} from "../fixtures";

/**
 * FULL E2E TEST: Support Tickets Lifecycle
 *
 * Critical Path: Load seeded ticket → view details → change status → send reply
 */

test.describe("Support Tickets Lifecycle (Full E2E)", () => {
  const ticketIDs: string[] = [];

  test.afterEach(async ({ request, adminToken }) => {
    while (ticketIDs.length > 0) {
      const id = ticketIDs.pop();
      if (!id) continue;
      await cleanupSupportTicketByID(request, adminToken, id).catch(() => {});
    }
  });

  test("load-and-verify seeded ticket, then change status, change priority, and reply", async ({ page, request, adminToken }) => {
    const probeStatus = await probeEndpoint(request, adminToken, "/api/admin/support/tickets");
    test.skip(
      probeStatus === 503 || probeStatus === 404 || probeStatus === 501,
      `Support tickets service unavailable (status ${probeStatus})`,
    );

    const runId = Date.now();
    const ticketSubject = `ticket-full-lifecycle-${runId}`;

    const seeded = await seedSupportTicket(request, adminToken, {
      subject: ticketSubject,
      status: "open",
      priority: "normal",
    });
    ticketIDs.push(seeded.id);

    await page.goto("/admin/");
    await waitForDashboard(page);

    await page.locator("aside").getByRole("button", { name: /^Support Tickets$/i }).click();
    await expect(page.getByRole("heading", { name: /Support Tickets/i })).toBeVisible({ timeout: 5000 });

    // Verify seeded ticket in table
    const ticketRow = page.getByRole("row", { name: new RegExp(ticketSubject) }).first();
    await expect(ticketRow).toBeVisible({ timeout: 5000 });
    await expect(ticketRow).toContainText("open");
    await expect(ticketRow).toContainText("normal");

    // Open details panel
    await ticketRow.getByRole("button", { name: /Details/i }).click();

    // Change status to in_progress
    const statusDropdown = page.getByRole("combobox").filter({ hasText: /open/i }).first();
    await statusDropdown.selectOption("in_progress");
    await expect(ticketRow).toContainText("in progress", { timeout: 5000 });

    // Change priority to high
    const priorityDropdown = page.getByRole("combobox").filter({ hasText: /normal/i }).first();
    await priorityDropdown.selectOption("high");
    await expect(ticketRow).toContainText("high", { timeout: 5000 });

    // Send a reply
    const replyMessage = `Support reply at ${runId}`;
    await page.getByPlaceholder(/Type a reply/i).fill(replyMessage);
    await page.getByRole("button", { name: /^Send$/i }).click();

    await expect(page.getByText(replyMessage)).toBeVisible({ timeout: 5000 });
    await expect(page.getByText(/support/i).first()).toBeVisible();
  });
});
