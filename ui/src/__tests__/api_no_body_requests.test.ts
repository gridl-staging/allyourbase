import { beforeEach, describe, expect, it, vi } from "vitest";
import {
  deleteApp,
  deleteAuthProvider,
  deleteBranch,
  deleteCronTrigger,
  deleteDBTrigger,
  deleteEdgeFunction,
  deleteEmailTemplate,
  deleteMatview,
  deleteRecord,
  deleteRlsPolicy,
  deleteSchedule,
  deleteStorageFile,
  deleteStorageTrigger,
  deleteUser,
  deleteWebhook,
  revokeAdminPushDevice,
  revokeApiKey,
  revokeOAuthClient,
} from "../api";

describe("API no-body requests", () => {
  const fetchMock = vi.fn<typeof fetch>();

  beforeEach(() => {
    vi.clearAllMocks();
    localStorage.clear();
    localStorage.setItem("ayb_admin_token", "admin-token");
    vi.stubGlobal("fetch", fetchMock);
  });

  it.each([
    ["deleteAuthProvider", () => deleteAuthProvider("google"), "/api/admin/auth/providers/google"],
    ["deleteWebhook", () => deleteWebhook("wh_1"), "/api/webhooks/wh_1"],
    ["deleteStorageFile", () => deleteStorageFile("uploads", "avatar.png"), "/api/storage/uploads/avatar.png"],
    ["deleteRecord", () => deleteRecord("posts", "row_1"), "/api/collections/posts/row_1"],
    ["deleteUser", () => deleteUser("user_1"), "/api/admin/users/user_1"],
    ["revokeApiKey", () => revokeApiKey("key_1"), "/api/admin/api-keys/key_1"],
    ["deleteApp", () => deleteApp("app_1"), "/api/admin/apps/app_1"],
    ["revokeOAuthClient", () => revokeOAuthClient("client_1"), "/api/admin/oauth/clients/client_1"],
    ["deleteSchedule", () => deleteSchedule("schedule_1"), "/api/admin/schedules/schedule_1"],
    ["deleteMatview", () => deleteMatview("matview_1"), "/api/admin/matviews/matview_1"],
    ["deleteBranch", () => deleteBranch("feature/test"), "/api/admin/branches/feature%2Ftest"],
    ["deleteEdgeFunction", () => deleteEdgeFunction("fn_1"), "/api/admin/functions/fn_1"],
    ["deleteRlsPolicy", () => deleteRlsPolicy("public.users", "policy name"), "/api/admin/rls/public.users/policy%20name"],
    ["deleteEmailTemplate", () => deleteEmailTemplate("welcome/email"), "/api/admin/email/templates/welcome%2Femail"],
    ["revokeAdminPushDevice", () => revokeAdminPushDevice("device_1"), "/api/admin/push/devices/device_1"],
    ["deleteDBTrigger", () => deleteDBTrigger("fn_1", "db_1"), "/api/admin/functions/fn_1/triggers/db/db_1"],
    ["deleteCronTrigger", () => deleteCronTrigger("fn_1", "cron_1"), "/api/admin/functions/fn_1/triggers/cron/cron_1"],
    ["deleteStorageTrigger", () => deleteStorageTrigger("fn_1", "storage_1"), "/api/admin/functions/fn_1/triggers/storage/storage_1"],
  ])("sends DELETE and accepts 204 for %s", async (_name, fn, expectedPath) => {
    fetchMock.mockResolvedValueOnce(new Response(null, { status: 204 }));

    await expect(fn()).resolves.toBeUndefined();

    expect(fetchMock).toHaveBeenCalledWith(expectedPath, {
      method: "DELETE",
      headers: {
        Authorization: "Bearer admin-token",
      },
    });
  });

  it("clears the admin token and dispatches unauthorized for no-body failures", async () => {
    const unauthorizedListener = vi.fn();
    window.addEventListener("ayb:unauthorized", unauthorizedListener);
    fetchMock.mockResolvedValueOnce(
      new Response(JSON.stringify({ message: "nope" }), {
        status: 401,
        headers: { "Content-Type": "application/json" },
      }),
    );

    await expect(deleteWebhook("wh_1")).rejects.toThrow("nope");

    expect(localStorage.getItem("ayb_admin_token")).toBeNull();
    expect(unauthorizedListener).toHaveBeenCalledTimes(1);
    window.removeEventListener("ayb:unauthorized", unauthorizedListener);
  });
});
