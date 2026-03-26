/**
 * @module Stub summary for /Users/stuart/parallel_development/allyourbase_dev/mar24_pm_2_auth_jwt_and_private_function_proof/allyourbase_dev/ui/browser-tests-unmocked/admin-bootstrap.ts.
 */
import type { APIRequestContext } from "@playwright/test";
import { readFileSync } from "fs";
import { join } from "path";
import { homedir } from "os";

export type AdminBootstrapCredential = {
  source: "env-password" | "saved-admin-auth";
  value: string;
};

function readSavedAdminAuth(): string {
  const tokenPath = join(homedir(), ".ayb", "admin-token");
  const savedAdminAuth = readFileSync(tokenPath, "utf-8").trim();
  if (savedAdminAuth.length === 0) {
    throw new Error("Saved admin auth file is empty");
  }
  return savedAdminAuth;
}

/**
 * TODO: Document resolveAdminBootstrapCredential.
 */
export function resolveAdminBootstrapCredential(): AdminBootstrapCredential {
  if (process.env.AYB_ADMIN_PASSWORD) {
    return { source: "env-password", value: process.env.AYB_ADMIN_PASSWORD };
  }

  try {
    return {
      source: "saved-admin-auth",
      value: readSavedAdminAuth(),
    };
  } catch {
    throw new Error(
      "No admin password found. Either set AYB_ADMIN_PASSWORD or ensure `ayb start` is running (writes ~/.ayb/admin-token).",
    );
  }
}

// The standalone login smoke needs an actual password to exercise the form.
// If the saved file already contains a bearer token, skip that positive-path
// form login instead of submitting the token as if it were a password.
/**
 * TODO: Document resolveAdminPasswordForBrowserLogin.
 */
export async function resolveAdminPasswordForBrowserLogin(
  request: APIRequestContext,
): Promise<string | null> {
  const credential = resolveAdminBootstrapCredential();
  if (credential.source === "env-password") {
    return credential.value;
  }

  const loginRes = await request.post("/api/admin/auth", {
    data: { password: credential.value },
  });
  if (loginRes.ok()) {
    return credential.value;
  }
  if (loginRes.status() === 401) {
    return null;
  }

  throw new Error(
    `Admin password probe failed with status ${loginRes.status()} while checking saved admin auth`,
  );
}
