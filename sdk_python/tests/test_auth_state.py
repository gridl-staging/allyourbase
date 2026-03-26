from __future__ import annotations

import pytest

from allyourbase.client import AYBClient


def _auth_payload(token: str, refresh: str) -> dict[str, object]:
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


async def test_listener_fires_signed_in_on_login(httpx_mock: pytest.fixture) -> None:
    httpx_mock.add_response(json=_auth_payload("tok", "ref"))
    client = AYBClient("https://api.example.com")

    events: list[tuple[str, dict[str, str] | None]] = []
    client.on_auth_state_change(lambda e, s: events.append((e, s)))

    await client.auth.login("alice@example.com", "secret")

    assert events == [("SIGNED_IN", {"token": "tok", "refresh_token": "ref"})]


async def test_listener_fires_signed_out_on_logout(httpx_mock: pytest.fixture) -> None:
    httpx_mock.add_response(status_code=204)
    client = AYBClient("https://api.example.com")
    client.set_tokens("tok", "ref")

    events: list[tuple[str, dict[str, str] | None]] = []
    client.on_auth_state_change(lambda e, s: events.append((e, s)))

    await client.auth.logout()

    assert events == [("SIGNED_OUT", None)]


async def test_listener_fires_token_refreshed(httpx_mock: pytest.fixture) -> None:
    httpx_mock.add_response(json=_auth_payload("tok_new", "ref_new"))
    client = AYBClient("https://api.example.com")
    client.set_tokens("tok_old", "ref_old")

    events: list[str] = []
    client.on_auth_state_change(lambda e, _: events.append(e))

    await client.auth.refresh()

    assert events == ["TOKEN_REFRESHED"]


async def test_unsubscribe_prevents_callbacks(httpx_mock: pytest.fixture) -> None:
    httpx_mock.add_response(json=_auth_payload("tok", "ref"))
    httpx_mock.add_response(json=_auth_payload("tok2", "ref2"))
    client = AYBClient("https://api.example.com")

    events: list[str] = []
    unsub = client.on_auth_state_change(lambda e, _: events.append(e))

    await client.auth.login("a@example.com", "pw")
    unsub()
    await client.auth.login("a@example.com", "pw")

    assert events == ["SIGNED_IN"]


async def test_multiple_listeners_fire(httpx_mock: pytest.fixture) -> None:
    httpx_mock.add_response(json=_auth_payload("tok", "ref"))
    client = AYBClient("https://api.example.com")

    a: list[str] = []
    b: list[str] = []
    client.on_auth_state_change(lambda e, _: a.append(e))
    client.on_auth_state_change(lambda e, _: b.append(e))

    await client.auth.register("a@example.com", "pw")

    assert a == ["SIGNED_IN"]
    assert b == ["SIGNED_IN"]


async def test_listener_self_unsubscribe_during_emit(httpx_mock: pytest.fixture) -> None:
    httpx_mock.add_response(json=_auth_payload("tok", "ref"))
    httpx_mock.add_response(json=_auth_payload("tok2", "ref2"))
    client = AYBClient("https://api.example.com")

    events: list[str] = []
    unsub_holder: list[callable] = []

    def listener(event: str, _session: dict[str, str] | None) -> None:
        events.append(event)
        unsub_holder[0]()

    unsub_holder.append(client.on_auth_state_change(listener))

    await client.auth.login("a@example.com", "pw")
    await client.auth.login("a@example.com", "pw")

    assert events == ["SIGNED_IN"]
