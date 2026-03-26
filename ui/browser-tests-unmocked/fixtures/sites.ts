import type { APIRequestContext } from "@playwright/test";
import { validateResponse } from "./core";

export interface SiteDeploy {
  id: string;
  siteId: string;
  status: string;
  fileCount: number;
  totalBytes: number;
  errorMessage?: string;
}

export interface SiteDeployListResult {
  deploys: SiteDeploy[];
  totalCount: number;
  page: number;
  perPage: number;
}

function decodeSiteDeploy(body: unknown, context: string): SiteDeploy {
  const deploy = body as Record<string, unknown>;
  if (
    typeof deploy?.id !== "string" ||
    typeof deploy?.siteId !== "string" ||
    typeof deploy?.status !== "string" ||
    typeof deploy?.fileCount !== "number" ||
    typeof deploy?.totalBytes !== "number"
  ) {
    throw new Error(`Expected deploy payload fields for ${context}`);
  }

  const errorMessage =
    typeof deploy.errorMessage === "string" ? deploy.errorMessage : undefined;

  return {
    id: deploy.id,
    siteId: deploy.siteId,
    status: deploy.status,
    fileCount: deploy.fileCount,
    totalBytes: deploy.totalBytes,
    errorMessage,
  };
}

function decodeSiteDeployList(body: unknown, siteID: string): SiteDeployListResult {
  const payload = body as Record<string, unknown>;
  if (
    !Array.isArray(payload?.deploys) ||
    typeof payload?.totalCount !== "number" ||
    typeof payload?.page !== "number" ||
    typeof payload?.perPage !== "number"
  ) {
    throw new Error(`Expected deploy list payload for site ${siteID}`);
  }

  return {
    deploys: payload.deploys.map((deploy, index) =>
      decodeSiteDeploy(deploy, `site ${siteID} deploy list item ${index}`),
    ),
    totalCount: payload.totalCount,
    page: payload.page,
    perPage: payload.perPage,
  };
}

export async function createSiteDeploy(
  request: APIRequestContext,
  token: string,
  siteID: string,
): Promise<SiteDeploy> {
  const res = await request.post(`/api/admin/sites/${encodeURIComponent(siteID)}/deploys`, {
    headers: { Authorization: `Bearer ${token}` },
  });
  await validateResponse(res, `Create deploy for site ${siteID}`);
  return decodeSiteDeploy(await res.json(), `create deploy for site ${siteID}`);
}

export async function getSiteDeploy(
  request: APIRequestContext,
  token: string,
  siteID: string,
  deployID: string,
): Promise<SiteDeploy> {
  const res = await request.get(
    `/api/admin/sites/${encodeURIComponent(siteID)}/deploys/${encodeURIComponent(deployID)}`,
    {
      headers: { Authorization: `Bearer ${token}` },
    },
  );
  await validateResponse(res, `Get deploy ${deployID} for site ${siteID}`);
  return decodeSiteDeploy(await res.json(), `get deploy ${deployID} for site ${siteID}`);
}

export async function listSiteDeploys(
  request: APIRequestContext,
  token: string,
  siteID: string,
): Promise<SiteDeployListResult> {
  const res = await request.get(`/api/admin/sites/${encodeURIComponent(siteID)}/deploys`, {
    headers: { Authorization: `Bearer ${token}` },
  });
  await validateResponse(res, `List deploys for site ${siteID}`);
  return decodeSiteDeployList(await res.json(), siteID);
}

export async function uploadSiteDeployFile(
  request: APIRequestContext,
  token: string,
  siteID: string,
  deployID: string,
  options: {
    name: string;
    content: string;
    mimeType?: string;
  },
): Promise<SiteDeploy> {
  const res = await request.post(
    `/api/admin/sites/${encodeURIComponent(siteID)}/deploys/${encodeURIComponent(deployID)}/files`,
    {
      headers: { Authorization: `Bearer ${token}` },
      multipart: {
        name: options.name,
        file: {
          name: options.name,
          mimeType: options.mimeType || "application/octet-stream",
          buffer: Buffer.from(options.content),
        },
      },
    },
  );
  await validateResponse(res, `Upload ${options.name} to deploy ${deployID}`);
  return decodeSiteDeploy(await res.json(), `upload ${options.name} to deploy ${deployID}`);
}

export async function promoteSiteDeploy(
  request: APIRequestContext,
  token: string,
  siteID: string,
  deployID: string,
): Promise<SiteDeploy> {
  const res = await request.post(
    `/api/admin/sites/${encodeURIComponent(siteID)}/deploys/${encodeURIComponent(deployID)}/promote`,
    {
      headers: { Authorization: `Bearer ${token}` },
    },
  );
  await validateResponse(res, `Promote deploy ${deployID} for site ${siteID}`);
  return decodeSiteDeploy(await res.json(), `promote deploy ${deployID} for site ${siteID}`);
}

export async function failSiteDeploy(
  request: APIRequestContext,
  token: string,
  siteID: string,
  deployID: string,
  errorMessage: string,
): Promise<SiteDeploy> {
  const res = await request.post(
    `/api/admin/sites/${encodeURIComponent(siteID)}/deploys/${encodeURIComponent(deployID)}/fail`,
    {
      headers: {
        Authorization: `Bearer ${token}`,
        "Content-Type": "application/json",
      },
      data: { errorMessage },
    },
  );
  await validateResponse(res, `Fail deploy ${deployID} for site ${siteID}`);
  return decodeSiteDeploy(await res.json(), `fail deploy ${deployID} for site ${siteID}`);
}

export async function rollbackSiteDeploy(
  request: APIRequestContext,
  token: string,
  siteID: string,
): Promise<SiteDeploy> {
  const res = await request.post(
    `/api/admin/sites/${encodeURIComponent(siteID)}/deploys/rollback`,
    {
      headers: { Authorization: `Bearer ${token}` },
    },
  );
  await validateResponse(res, `Rollback site ${siteID}`);
  return decodeSiteDeploy(await res.json(), `rollback site ${siteID}`);
}
