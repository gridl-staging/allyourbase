#!/usr/bin/env bash
set -euo pipefail
source "$(dirname "$0")/bash_assert_helpers.sh"

line_number() {
  local file_path="$1"
  local needle="$2"
  local line
  line="$(grep -nF -- "$needle" "$file_path" | head -n 1 | cut -d: -f1)"
  [[ -n "$line" ]] || fail "missing expected line '$needle' in ${file_path}"
  printf '%s\n' "$line"
}

assert_order() {
  local file_path="$1"
  local first="$2"
  local second="$3"
  local first_line second_line

  first_line="$(line_number "$file_path" "$first")"
  second_line="$(line_number "$file_path" "$second")"

  if (( first_line >= second_line )); then
    fail "expected '$first' to appear before '$second' in ${file_path}"
  fi
}

api_runner="_dev/manual_smoke_tests/run_all_api_tests.py"
shell_runner="_dev/manual_smoke_tests/run_all_tests.sh"
common_auth_helper="_dev/manual_smoke_tests/common_auth.py"

[[ -f "$api_runner" ]] || fail "missing ${api_runner}"
[[ -f "$shell_runner" ]] || fail "missing ${shell_runner}"
[[ -f "$common_auth_helper" ]] || fail "missing ${common_auth_helper}"

assert_contains "$api_runner" '"full_journey.test.py"' "API runner should include full_journey smoke test"
assert_order "$api_runner" '"11_functions.test.py"' '"full_journey.test.py"'

assert_contains "$shell_runner" 'run_test "full_journey.test.py" "Full Journey Resource Lifecycle" "python" || true' "shell runner should call full_journey smoke test"
assert_order "$shell_runner" 'run_test "13_cli_commands.test.sh" "CLI Commands" "bash" || true' 'run_test "full_journey.test.py" "Full Journey Resource Lifecycle" "python" || true'
assert_order "$shell_runner" 'run_test "full_journey.test.py" "Full Journey Resource Lifecycle" "python" || true' 'run_test "14_stop_restart.test.sh" "Stop/Restart" "bash" || true'

assert_not_contains "$shell_runner" 'API tests only (5-13)' "shell runner usage text should not preserve stale safe-test range"
if ! grep -Eq '^#   ./run_all_tests\.sh\s+# API tests only .*full_journey\.test\.py' "$shell_runner"; then
  fail "shell runner usage header should describe full_journey in the safe test sequence"
fi

python3 - <<'PY'
import importlib.util
from pathlib import Path

module_path = Path("_dev/manual_smoke_tests/common_auth.py")
spec = importlib.util.spec_from_file_location("common_auth", module_path)
module = importlib.util.module_from_spec(spec)
spec.loader.exec_module(module)


class DummyResponse:
    def __init__(self, status_code, payload):
        self.status_code = status_code
        self._payload = payload
        self.text = str(payload)

    def json(self):
        return self._payload


calls = []


def fake_post(url, **kwargs):
    calls.append(("POST", url, kwargs))
    if url.endswith("/api/admin/auth"):
        host = url.split("/api/admin/auth", 1)[0]
        password = kwargs["json"]["password"]
        return DummyResponse(200, {"token": f"{host}::{password}"})
    if url.endswith("/api/admin/tenants"):
        return DummyResponse(201, {"id": "tenant-123"})
    if url.endswith("/api/auth/register"):
        return DummyResponse(201, {"user": {"id": "user-123"}, "token": "user-token"})
    if url.endswith("/members"):
        return DummyResponse(201, {"status": "ok"})
    raise AssertionError(f"unexpected POST url: {url}")


def fake_get(url, **kwargs):
    calls.append(("GET", url, kwargs))
    if url.endswith("/api/admin/tenants"):
        return DummyResponse(200, {"items": [], "totalPages": 1})
    raise AssertionError(f"unexpected GET url: {url}")


module.requests.post = fake_post
module.requests.get = fake_get
module._cached_admin_token = None
module._cached_admin_token_key = None
module._token_cache_time = 0

token = module.get_admin_token(base_url="http://example.test/", admin_password="secret")
assert token == "http://example.test::secret"
assert calls[0][1] == "http://example.test/api/admin/auth"
assert calls[0][2]["timeout"] == module.REQUEST_TIMEOUT

cached_token = module.get_admin_token(base_url="http://example.test", admin_password="secret")
assert cached_token == token
assert len(calls) == 1

fresh_token = module.get_admin_token(base_url="http://other.test", admin_password="secret")
assert fresh_token == "http://other.test::secret"
assert len(calls) == 2

calls.clear()
tenant_id = module.get_or_create_smoke_tenant(
    base_url="http://example.test",
    admin_token="admin-token",
)
assert tenant_id == "tenant-123"
assert [call[0] for call in calls] == ["GET", "POST"]
assert all(call[2]["timeout"] == module.REQUEST_TIMEOUT for call in calls)

calls.clear()
user_id, user_token = module.create_tenant_user(
    base_url="http://example.test",
    admin_token="admin-token",
    tenant_id="tenant-123",
)
assert (user_id, user_token) == ("user-123", "user-token")
assert [call[0] for call in calls] == ["POST", "POST"]
assert all(call[2]["timeout"] == module.REQUEST_TIMEOUT for call in calls)
PY

echo "PASS: API runner, shell runner, and common auth helpers keep the smoke-test contract"
