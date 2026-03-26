import { assertResponseChecks, parseJSONResponse } from '../lib/checks.js';
import {
  createDataFixture,
  dataAdminRequestHeaders,
  dropDataFixture,
  loadDataRunTableName,
  runAdminSQL,
} from '../lib/data.js';
import { loadBaseURL, loadScenarioOptions, parsePositiveInt, readEnv } from '../lib/env.js';

const BASE_URL = loadBaseURL();
const POOL_PRESSURE_QUERY = 'SELECT pg_sleep(2)';
const baseOptions = loadScenarioOptions({
  scenarioName: 'data_pool_pressure',
  endpointThresholds: {
    admin_sql_pool_pressure: ['p(95)<5000'],
  },
});
const scenarioVUs = parsePositiveInt(readEnv('AYB_POOL_PRESSURE_VUS'), baseOptions.vus);
const scenarioIterations = parsePositiveInt(readEnv('AYB_POOL_PRESSURE_ITERATIONS'), baseOptions.iterations);
delete baseOptions.vus;
delete baseOptions.iterations;

export const options = {
  ...baseOptions,
  scenarios: {
    default: {
      executor: 'per-vu-iterations',
      vus: scenarioVUs,
      iterations: scenarioIterations,
      maxDuration: '10m',
      gracefulStop: '30s',
    },
  },
};

export function setup() {
  const tableName = loadDataRunTableName();
  createDataFixture(BASE_URL, dataAdminRequestHeaders(), tableName);
  return {
    tableName,
  };
}

export function teardown(setupData) {
  dropDataFixture(BASE_URL, dataAdminRequestHeaders(), setupData.tableName);
}

export default function runDataPoolPressure() {
  const response = runAdminSQL(
    BASE_URL,
    dataAdminRequestHeaders(),
    POOL_PRESSURE_QUERY,
    {
      endpointTag: 'admin_sql_pool_pressure',
      requireStatus: null,
    },
  );
  const payload = parseJSONResponse(response);

  assertResponseChecks(response, {
    'admin SQL pressure responds with HTTP 200': (res) => res.status === 200,
    'admin SQL pressure response includes row count': () => payload !== null && typeof payload.rowCount === 'number',
  });
}
