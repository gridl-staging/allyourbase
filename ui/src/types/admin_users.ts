export interface AdminUser {
  id: string;
  email: string;
  emailVerified: boolean;
  createdAt: string;
  updatedAt: string;
}

export interface UserListResponse {
  items: AdminUser[];
  page: number;
  perPage: number;
  totalItems: number;
  totalPages: number;
}
