const DEFAULT_BASE_URL = 'http://127.0.0.1:8090';
const DEFAULT_VUS = 1;
const DEFAULT_ITERATIONS = 1;
const DEFAULT_SOAK_DURATION = '5m';
const DEFAULT_SOAK_GRACEFUL_STOP = '30s';
const DEFAULT_HARNESS_TAG = 'stage2-foundation';
const DEFAULT_COMMON_THRESHOLDS = {
  http_req_failed: ['rate<0.01'],
};

export function readEnv(name) {
  if (typeof __ENV === 'undefined') {
    return '';
  }
  const value = __ENV[name];
  return typeof value === 'string' ? value.trim() : '';
}

export function trimTrailingSlashes(value) {
  return value.replace(/\/+$/, '');
}

export function parsePositiveInt(value, fallback) {
  const parsed = Number.parseInt(value, 10);
  if (!Number.isFinite(parsed) || parsed < 1) {
    return fallback;
  }
  return parsed;
}

export function loadBaseURL() {
  const configured = readEnv('AYB_BASE_URL');
  const rawBaseURL = configured === '' ? DEFAULT_BASE_URL : configured;
  return trimTrailingSlashes(rawBaseURL);
}

function toDurationThresholds(endpointThresholds) {
  return Object.entries(endpointThresholds).reduce((accumulator, [endpointTag, thresholdValues]) => {
    accumulator[`http_req_duration{endpoint:${endpointTag}}`] = thresholdValues;
    return accumulator;
  }, {});
}

function readDuration(name, fallback) {
  const configuredDuration = readEnv(name);
  return configuredDuration === '' ? fallback : configuredDuration;
}

export function loadScenarioOptions({
  scenarioName,
  endpointThresholds = {},
  tags = {},
  thresholds = {},
  executionMode = 'iterations',
  durationEnvVar = 'AYB_SOAK_DURATION',
  defaultDuration = DEFAULT_SOAK_DURATION,
  gracefulStop = DEFAULT_SOAK_GRACEFUL_STOP,
}) {
  const vus = parsePositiveInt(readEnv('K6_VUS'), DEFAULT_VUS);
  const iterations = parsePositiveInt(readEnv('K6_ITERATIONS'), DEFAULT_ITERATIONS);
  const baseOptions = {
    tags: {
      harness: DEFAULT_HARNESS_TAG,
      scenario: scenarioName,
      ...tags,
    },
    thresholds: {
      ...DEFAULT_COMMON_THRESHOLDS,
      ...toDurationThresholds(endpointThresholds),
      ...thresholds,
    },
  };

  if (executionMode === 'duration') {
    return {
      ...baseOptions,
      scenarios: {
        default: {
          executor: 'constant-vus',
          vus,
          duration: readDuration(durationEnvVar, defaultDuration),
          gracefulStop,
        },
      },
    };
  }

  return {
    ...baseOptions,
    vus,
    iterations,
  };
}

export function loadCommonOptions(scenarioName) {
  return loadScenarioOptions({
    scenarioName,
    endpointThresholds: {
      admin_status: ['p(95)<1000'],
    },
  });
}

export function loadSustainedSoakOptions({
  scenarioName,
  endpointThresholds = {},
  tags = {},
  thresholds = {},
}) {
  return loadScenarioOptions({
    scenarioName,
    endpointThresholds,
    tags,
    thresholds,
    executionMode: 'duration',
  });
}
