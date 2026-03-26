"""Auth client for AYB."""

from __future__ import annotations

from typing import TYPE_CHECKING

from allyourbase.types import AuthResponse, User

if TYPE_CHECKING:
    from allyourbase.client import AYBClient


class AuthClient:
    """Handles authentication operations."""

    def __init__(self, client: AYBClient) -> None:
        self._client = client

    async def register(self, email: str, password: str) -> AuthResponse:
        resp = await self._client._request(
            "/api/auth/register",
            method="POST",
            json={"email": email, "password": password},
        )
        if resp is None:
            raise RuntimeError("Expected response body for register")
        auth = AuthResponse.model_validate(resp.json())
        self._client.set_tokens(auth.token, auth.refresh_token)
        self._client._emit_auth_event("SIGNED_IN")
        return auth

    async def login(self, email: str, password: str) -> AuthResponse:
        resp = await self._client._request(
            "/api/auth/login",
            method="POST",
            json={"email": email, "password": password},
        )
        if resp is None:
            raise RuntimeError("Expected response body for login")
        auth = AuthResponse.model_validate(resp.json())
        self._client.set_tokens(auth.token, auth.refresh_token)
        self._client._emit_auth_event("SIGNED_IN")
        return auth

    async def me(self) -> User:
        resp = await self._client._request("/api/auth/me")
        if resp is None:
            raise RuntimeError("Expected response body for me")
        return User.model_validate(resp.json())

    async def refresh(self) -> AuthResponse:
        resp = await self._client._request(
            "/api/auth/refresh",
            method="POST",
            json={"refreshToken": self._client.refresh_token},
        )
        if resp is None:
            raise RuntimeError("Expected response body for refresh")
        auth = AuthResponse.model_validate(resp.json())
        self._client.set_tokens(auth.token, auth.refresh_token)
        self._client._emit_auth_event("TOKEN_REFRESHED")
        return auth

    async def logout(self) -> None:
        await self._client._request(
            "/api/auth/logout",
            method="POST",
            json={"refreshToken": self._client.refresh_token},
        )
        self._client.clear_tokens()
        self._client._emit_auth_event("SIGNED_OUT")

    async def delete_account(self) -> None:
        await self._client._request("/api/auth/me", method="DELETE")
        self._client.clear_tokens()
        self._client._emit_auth_event("SIGNED_OUT")

    async def request_password_reset(self, email: str) -> None:
        await self._client._request(
            "/api/auth/password-reset",
            method="POST",
            json={"email": email},
        )

    async def confirm_password_reset(self, token: str, password: str) -> None:
        await self._client._request(
            "/api/auth/password-reset/confirm",
            method="POST",
            json={"token": token, "password": password},
        )

    async def verify_email(self, token: str) -> None:
        await self._client._request(
            "/api/auth/verify",
            method="POST",
            json={"token": token},
        )

    async def resend_verification(self) -> None:
        await self._client._request(
            "/api/auth/verify/resend",
            method="POST",
        )
