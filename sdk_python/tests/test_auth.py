from __future__ import annotations

import pytest

from allyourbase.client import AYBClient
from allyourbase.errors import AYBError
from allyourbase.types import AuthResponse, User


def _auth_payload(token: str = "tok", refresh: str = "ref") -> dict[str, object]:
    return {
        "token": token,
        "refreshToken": refresh,
        "user": {
            "id": "usr_1",
            "email": "alice@example.com",
            "createdAt": "2026-02-22T10:00:00Z",
            "updatedAt": "2026-02-22T11:00:00Z",
        },
    }


async def test_register_sends_request_stores_tokens_emits_event(httpx_mock: pytest.fixture) -> None:
    httpx_mock.add_response(json=_auth_payload("tok_reg", "ref_reg"))
    client = AYBClient("https://api.example.com")

    events: list[tuple[str, dict[str, str] | None]] = []
    client.on_auth_state_change(lambda e, s: events.append((e, s)))

    result = await client.auth.register("alice@example.com", "secret")

    assert isinstance(result, AuthResponse)
    assert client.token == "tok_reg"
    assert client.refresh_token == "ref_reg"
    req = httpx_mock.get_request()
    assert req is not None
    assert req.method == "POST"
    assert str(req.url) == "https://api.example.com/api/auth/register"
    assert req.content == b'{"email":"alice@example.com","password":"secret"}'
    assert events == [
        ("SIGNED_IN", {"token": "tok_reg", "refresh_token": "ref_reg"}),
    ]


async def test_login_sends_request_stores_tokens_emits_event(httpx_mock: pytest.fixture) -> None:
    httpx_mock.add_response(json=_auth_payload("tok_login", "ref_login"))
    client = AYBClient("https://api.example.com")

    events: list[str] = []
    client.on_auth_state_change(lambda e, _: events.append(e))

    await client.auth.login("alice@example.com", "secret")

    assert client.token == "tok_login"
    assert client.refresh_token == "ref_login"
    req = httpx_mock.get_request()
    assert req is not None
    assert req.method == "POST"
    assert str(req.url) == "https://api.example.com/api/auth/login"
    assert events == ["SIGNED_IN"]


async def test_me_gets_user_with_auth_header(httpx_mock: pytest.fixture) -> None:
    httpx_mock.add_response(
        json={
            "id": "usr_1",
            "email": "alice@example.com",
            "createdAt": "2026-02-22T10:00:00Z",
            "updatedAt": "2026-02-22T11:00:00Z",
        }
    )
    client = AYBClient("https://api.example.com")
    client.set_tokens("tok", "ref")

    user = await client.auth.me()

    assert isinstance(user, User)
    req = httpx_mock.get_request()
    assert req is not None
    assert req.method == "GET"
    assert req.headers["authorization"] == "Bearer tok"


async def test_refresh_sends_refresh_token_updates_state_emits_event(
    httpx_mock: pytest.fixture,
) -> None:
    httpx_mock.add_response(json=_auth_payload("tok_new", "ref_new"))
    client = AYBClient("https://api.example.com")
    client.set_tokens("tok_old", "ref_old")

    events: list[tuple[str, dict[str, str] | None]] = []
    client.on_auth_state_change(lambda e, s: events.append((e, s)))

    await client.auth.refresh()

    req = httpx_mock.get_request()
    assert req is not None
    assert req.content == b'{"refreshToken":"ref_old"}'
    assert client.token == "tok_new"
    assert client.refresh_token == "ref_new"
    assert events == [
        ("TOKEN_REFRESHED", {"token": "tok_new", "refresh_token": "ref_new"})
    ]


async def test_logout_sends_refresh_token_clears_tokens_emits_signed_out(
    httpx_mock: pytest.fixture,
) -> None:
    httpx_mock.add_response(status_code=204)
    client = AYBClient("https://api.example.com")
    client.set_tokens("tok", "ref")

    events: list[tuple[str, dict[str, str] | None]] = []
    client.on_auth_state_change(lambda e, s: events.append((e, s)))

    await client.auth.logout()

    req = httpx_mock.get_request()
    assert req is not None
    assert req.content == b'{"refreshToken":"ref"}'
    assert client.token is None
    assert client.refresh_token is None
    assert events == [("SIGNED_OUT", None)]


async def test_delete_account_clears_tokens_emits_signed_out(httpx_mock: pytest.fixture) -> None:
    httpx_mock.add_response(status_code=204)
    client = AYBClient("https://api.example.com")
    client.set_tokens("tok", "ref")

    events: list[str] = []
    client.on_auth_state_change(lambda e, _: events.append(e))

    await client.auth.delete_account()

    req = httpx_mock.get_request()
    assert req is not None
    assert req.method == "DELETE"
    assert str(req.url) == "https://api.example.com/api/auth/me"
    assert client.token is None
    assert client.refresh_token is None
    assert events == ["SIGNED_OUT"]


async def test_request_password_reset_payload(httpx_mock: pytest.fixture) -> None:
    httpx_mock.add_response(status_code=204)
    client = AYBClient("https://api.example.com")

    await client.auth.request_password_reset("alice@example.com")

    req = httpx_mock.get_request()
    assert req is not None
    assert req.method == "POST"
    assert str(req.url) == "https://api.example.com/api/auth/password-reset"
    assert req.content == b'{"email":"alice@example.com"}'


async def test_confirm_password_reset_payload(httpx_mock: pytest.fixture) -> None:
    httpx_mock.add_response(status_code=204)
    client = AYBClient("https://api.example.com")

    await client.auth.confirm_password_reset("reset-token", "new-password")

    req = httpx_mock.get_request()
    assert req is not None
    assert str(req.url) == "https://api.example.com/api/auth/password-reset/confirm"
    assert req.content == b'{"token":"reset-token","password":"new-password"}'


async def test_verify_email_payload(httpx_mock: pytest.fixture) -> None:
    httpx_mock.add_response(status_code=204)
    client = AYBClient("https://api.example.com")

    await client.auth.verify_email("verify-token")

    req = httpx_mock.get_request()
    assert req is not None
    assert str(req.url) == "https://api.example.com/api/auth/verify"
    assert req.content == b'{"token":"verify-token"}'


async def test_resend_verification_sends_auth(httpx_mock: pytest.fixture) -> None:
    httpx_mock.add_response(status_code=204)
    client = AYBClient("https://api.example.com")
    client.set_tokens("tok", "ref")

    await client.auth.resend_verification()

    req = httpx_mock.get_request()
    assert req is not None
    assert str(req.url) == "https://api.example.com/api/auth/verify/resend"
    assert req.method == "POST"
    assert req.headers["authorization"] == "Bearer tok"


async def test_auth_error_propagates_as_ayberror(httpx_mock: pytest.fixture) -> None:
    httpx_mock.add_response(status_code=401, json={"message": "Invalid email or password"})
    client = AYBClient("https://api.example.com")

    with pytest.raises(AYBError) as exc:
        await client.auth.login("alice@example.com", "wrong")

    assert exc.value.status == 401
    assert exc.value.message == "Invalid email or password"
