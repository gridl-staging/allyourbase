import { readEnv, trimTrailingSlashes } from './env.js';

const ADMIN_STATUS_PATH = '/api/admin/status';

export function adminStatusURL(baseURL) {
  return `${trimTrailingSlashes(baseURL)}${ADMIN_STATUS_PATH}`;
}

export function readAdminToken() {
  return readEnv('AYB_ADMIN_TOKEN');
}

export function adminAuthHeaders() {
  const token = readAdminToken();
  if (token === '') {
    return {};
  }
  return {
    Authorization: `Bearer ${token}`,
  };
}
