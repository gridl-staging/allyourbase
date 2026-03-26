import http from 'k6/http';

import { adminStatusURL } from '../lib/admin.js';
import { assertAdminStatusResponse } from '../lib/checks.js';
import { loadBaseURL, loadCommonOptions } from '../lib/env.js';

const BASE_URL = loadBaseURL();

export const options = loadCommonOptions('admin_status_baseline');

export default function runAdminStatusBaseline() {
  const response = http.get(adminStatusURL(BASE_URL), {
    tags: {
      endpoint: 'admin_status',
      method: 'GET',
    },
  });

  assertAdminStatusResponse(response);
}
