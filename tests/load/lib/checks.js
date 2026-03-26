import { check } from 'k6';

export function parseJSONResponse(response) {
  try {
    return response.json();
  } catch (_) {
    return null;
  }
}

export function assertResponseChecks(response, assertions) {
  return check(response, assertions);
}

export function assertAdminStatusResponse(response) {
  const payload = parseJSONResponse(response);
  return assertResponseChecks(response, {
    'admin status responds with HTTP 200': (res) => res.status === 200,
    'admin status payload includes auth boolean': () => payload !== null && typeof payload.auth === 'boolean',
  });
}
