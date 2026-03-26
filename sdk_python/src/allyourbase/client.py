"""
Stub summary for /Users/stuart/parallel_development/allyourbase_dev/MAR18_WS_C_phase5_features_and_phase6/allyourbase_dev/sdk_python/src/allyourbase/client.py.
"""

from __future__ import annotations

import json
from typing import Any, Callable, Dict, Optional, Set

import httpx

from allyourbase.errors import AYBError

AuthStateListener = Callable[[str, Optional[Dict[str, str]]], None]


class AYBClient:
    """AllYourBase Python client."""

    def __init__(
        self,
        base_url: str,
        *,
        http_client: Optional[httpx.AsyncClient] = None,
    ) -> None:
        self.base_url = base_url.rstrip("/")
        self._http = http_client or httpx.AsyncClient()
        self._token: Optional[str] = None
        self._refresh_token: Optional[str] = None
        self._auth_listeners: Set[AuthStateListener] = set()

        from allyourbase.auth import AuthClient
        from allyourbase.records import RecordsClient
        from allyourbase.realtime import RealtimeClient
        from allyourbase.storage import StorageClient

        self.auth = AuthClient(self)
        self.records = RecordsClient(self)
        self.storage = StorageClient(self)
        self.realtime = RealtimeClient(self)

    @property
    def token(self) -> Optional[str]:
        return self._token

    @property
    def refresh_token(self) -> Optional[str]:
        return self._refresh_token

    def set_tokens(self, token: str, refresh_token: str) -> None:
        self._token = token
        self._refresh_token = refresh_token

    def clear_tokens(self) -> None:
        self._token = None
        self._refresh_token = None

    def set_api_key(self, api_key: str) -> None:
        self._token = api_key
        self._refresh_token = None

    def clear_api_key(self) -> None:
        self.clear_tokens()

    def on_auth_state_change(self, listener: AuthStateListener) -> Callable[[], None]:
        self._auth_listeners.add(listener)

        def unsubscribe() -> None:
            self._auth_listeners.discard(listener)

        return unsubscribe

    def _emit_auth_event(self, event: str) -> None:
        session: Optional[Dict[str, str]] = None
        if self._token is not None and self._refresh_token is not None:
            session = {"token": self._token, "refresh_token": self._refresh_token}
        for listener in list(self._auth_listeners):
            listener(event, session)

    async def _request(
        self,
        path: str,
        *,
        method: str = "GET",
        headers: Optional[Dict[str, str]] = None,
        json: Optional[Any] = None,
        data: Optional[Any] = None,
        skip_auth: bool = False,
    ) -> Optional[httpx.Response]:
        """TODO: Document AYBClient._request."""
        req_headers: Dict[str, str] = dict(headers or {})
        if not skip_auth and self._token is not None:
            req_headers["Authorization"] = f"Bearer {self._token}"

        url = f"{self.base_url}{path}"
        resp = await self._http.request(
            method,
            url,
            headers=req_headers,
            json=json,
            data=data,
        )

        if resp.status_code < 200 or resp.status_code >= 300:
            _raise_error(resp)

        if resp.status_code == 204:
            return None
        return resp

    async def rpc(
        self,
        function_name: str,
        args: Optional[Dict[str, Any]] = None,
    ) -> Any:
        has_args = args is not None and len(args) > 0
        resp = await self._request(
            f"/api/rpc/{function_name}",
            method="POST",
            json=args if has_args else None,
        )
        if resp is None or not resp.content:
            return None
        return resp.json()

    async def close(self) -> None:
        await self._http.aclose()

    async def __aenter__(self) -> AYBClient:
        return self

    async def __aexit__(self, *args: Any) -> None:
        await self.close()


def _raise_error(resp: httpx.Response) -> None:
    """TODO: Document _raise_error."""
    status = resp.status_code
    message = resp.reason_phrase or f"HTTP {status}"
    code: Optional[str] = None
    data: Optional[Dict[str, Any]] = None
    doc_url: Optional[str] = None

    try:
        body = resp.json()
        if isinstance(body, dict):
            if isinstance(body.get("message"), str) and body["message"]:
                message = body["message"]
            raw_code = body.get("code")
            if isinstance(raw_code, str) and raw_code:
                code = raw_code
            elif isinstance(raw_code, int):
                code = str(raw_code)
            if isinstance(body.get("data"), dict):
                data = body["data"]
            raw_doc = body.get("doc_url") or body.get("docUrl")
            if isinstance(raw_doc, str) and raw_doc:
                doc_url = raw_doc
    except (json.JSONDecodeError, ValueError):
        pass

    raise AYBError(status, message, code=code, data=data, doc_url=doc_url)
