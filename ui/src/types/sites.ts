export type DeployStatus = "uploading" | "live" | "superseded" | "failed";

export interface Site {
  id: string;
  name: string;
  slug: string;
  spaMode: boolean;
  customDomainId?: string;
  createdAt: string;
  updatedAt: string;
  liveDeployId?: string;
}

export interface Deploy {
  id: string;
  siteId: string;
  status: DeployStatus;
  fileCount: number;
  totalBytes: number;
  errorMessage?: string;
  createdAt: string;
  updatedAt: string;
}

export interface SiteListResult {
  sites: Site[];
  totalCount: number;
  page: number;
  perPage: number;
}

export interface DeployListResult {
  deploys: Deploy[];
  totalCount: number;
  page: number;
  perPage: number;
}

export interface CreateSiteRequest {
  name: string;
  slug: string;
  spaMode: boolean;
  customDomainId?: string;
}

export interface UpdateSiteRequest {
  name?: string;
  spaMode?: boolean;
  customDomainId?: string;
  clearCustomDomain?: boolean;
}
