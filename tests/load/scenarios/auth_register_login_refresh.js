import {
  buildAuthScenarioOptions,
  runAuthRegisterLoginRefreshFlow,
  uniqueAuthIdentity,
} from '../lib/auth.js';
import { loadBaseURL } from '../lib/env.js';

const BASE_URL = loadBaseURL();

export const options = buildAuthScenarioOptions();

export default function runAuthRegisterLoginRefresh() {
  const identity = uniqueAuthIdentity(__VU, __ITER);
  runAuthRegisterLoginRefreshFlow(BASE_URL, identity);
}
