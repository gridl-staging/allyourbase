import http from 'k6/http';

import {
  bootstrapDataUserHeaders,
  createDataFixture,
  dataAdminRequestHeaders,
  dataCollectionListURL,
  dropDataFixture,
  loadDataRunTableName,
  runDataPathCRUDAndBatchFlow,
} from '../lib/data.js';
import { loadBaseURL, loadScenarioOptions } from '../lib/env.js';

const BASE_URL = loadBaseURL();

export const options = loadScenarioOptions({
  scenarioName: 'data_path_crud_batch',
  endpointThresholds: {
    data_list: ['p(95)<1200'],
    data_create: ['p(95)<1200'],
    data_read: ['p(95)<1200'],
    data_update: ['p(95)<1200'],
    data_batch: ['p(95)<1200'],
    data_batch_rollback_probe: ['p(95)<1200'],
    data_delete: ['p(95)<1200'],
  },
});

export function setup() {
  const tableName = loadDataRunTableName();
  createDataFixture(BASE_URL, dataAdminRequestHeaders(), tableName);
  const userHeaders = bootstrapDataUserHeaders(BASE_URL);
  const authProbeResponse = http.get(
    dataCollectionListURL(BASE_URL, tableName),
    {
      headers: {
        ...userHeaders,
      },
      responseCallback: http.expectedStatuses(200, 401, 403),
      tags: {
        endpoint: 'data_auth_probe',
        method: 'GET',
      },
    },
  );
  const requestHeaders = authProbeResponse.status === 200 ? userHeaders : dataAdminRequestHeaders();
  return {
    tableName,
    requestHeaders,
  };
}

export function teardown(setupData) {
  dropDataFixture(BASE_URL, dataAdminRequestHeaders(), setupData.tableName);
}

export default function runDataPathCrudBatch(setupData) {
  runDataPathCRUDAndBatchFlow({
    baseURL: BASE_URL,
    tableName: setupData.tableName,
    userHeaders: setupData.requestHeaders,
  });
}
