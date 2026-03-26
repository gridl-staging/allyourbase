/**
 * @module Stub summary for /Users/stuart/parallel_development/allyourbase_dev/MAR18_WS_C_phase5_features_and_phase6/allyourbase_dev/ui/src/types/domains.ts.
 */
/**
 * TODO: Document DomainBinding.
 */
export interface DomainBinding {
  id: string;
  hostname: string;
  environment: string;
  status: string;
  verificationToken: string;
  verificationRecord?: string;
  certRef?: string;
  certExpiry?: string;
  redirectMode?: string;
  lastError?: string;
  healthStatus: string;
  lastHealthCheck?: string;
  createdAt: string;
  updatedAt: string;
}

export interface DomainBindingListResult {
  items: DomainBinding[];
  page: number;
  perPage: number;
  totalItems: number;
  totalPages: number;
}

export interface CreateDomainRequest {
  hostname: string;
  environment: string;
  redirectMode: string;
}
