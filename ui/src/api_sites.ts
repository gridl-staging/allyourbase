import { request, requestNoBody } from "./api_client";
import type {
  CreateSiteRequest,
  Deploy,
  DeployListResult,
  Site,
  SiteListResult,
  UpdateSiteRequest,
} from "./types/sites";

interface ListParams {
  page?: number;
  perPage?: number;
}

const SITES_API_PATH = "/api/admin/sites";

const noStoreRequest: RequestInit = {
  cache: "no-store",
};

const jsonHeaders = {
  "Content-Type": "application/json",
};

function sendJSON<T>(path: string, method: "POST" | "PUT", body: unknown): Promise<T> {
  return request<T>(path, {
    method,
    headers: jsonHeaders,
    body: JSON.stringify(body),
  });
}

function postAction<T>(path: string): Promise<T> {
  return request<T>(path, { method: "POST" });
}

function sitePath(siteId: string): string {
  return `${SITES_API_PATH}/${encodeURIComponent(siteId)}`;
}

function siteDeploysPath(siteId: string): string {
  return `${sitePath(siteId)}/deploys`;
}

function siteDeployActionPath(siteId: string, deployId: string, action: string): string {
  return `${siteDeploysPath(siteId)}/${encodeURIComponent(deployId)}/${action}`;
}

function withPagination(path: string, params: ListParams = {}): string {
  const query = new URLSearchParams();
  if (params.page) {
    query.set("page", String(params.page));
  }
  if (params.perPage) {
    query.set("perPage", String(params.perPage));
  }
  const suffix = query.toString();
  return suffix ? `${path}?${suffix}` : path;
}

export function listSites(params: ListParams = {}): Promise<SiteListResult> {
  return request<SiteListResult>(withPagination(SITES_API_PATH, params), noStoreRequest);
}

export function createSite(req: CreateSiteRequest): Promise<Site> {
  return sendJSON<Site>(SITES_API_PATH, "POST", req);
}

export function getSite(siteId: string): Promise<Site> {
  return request<Site>(sitePath(siteId), noStoreRequest);
}

export function updateSite(
  siteId: string,
  req: UpdateSiteRequest,
): Promise<Site> {
  return sendJSON<Site>(sitePath(siteId), "PUT", req);
}

export function deleteSite(siteId: string): Promise<void> {
  return requestNoBody(sitePath(siteId), {
    method: "DELETE",
  });
}

export function listDeploys(
  siteId: string,
  params: ListParams = {},
): Promise<DeployListResult> {
  return request<DeployListResult>(
    withPagination(siteDeploysPath(siteId), params),
    noStoreRequest,
  );
}

export function promoteDeploy(
  siteId: string,
  deployId: string,
): Promise<Deploy> {
  return postAction<Deploy>(siteDeployActionPath(siteId, deployId, "promote"));
}

export function rollbackDeploy(siteId: string): Promise<Deploy> {
  return postAction<Deploy>(`${siteDeploysPath(siteId)}/rollback`);
}
