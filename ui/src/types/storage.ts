export interface StorageObject {
  id: string;
  bucket: string;
  name: string;
  size: number;
  contentType: string;
  createdAt: string;
}

export interface StorageListResponse {
  items: StorageObject[];
  totalItems: number;
}

export type StorageCDNPurgeRequest =
  | { urls: string[] }
  | { purgeAll: true };

export interface StorageCDNPurgeResponse {
  operation: string;
  submitted: number;
  provider: string;
}
