export interface AppResponse {
  id: string;
  name: string;
  description: string;
  ownerUserId: string;
  rateLimitRps: number;
  rateLimitWindowSeconds: number;
  createdAt: string;
  updatedAt: string;
}

export interface AppListResponse {
  items: AppResponse[];
  page: number;
  perPage: number;
  totalItems: number;
  totalPages: number;
}
