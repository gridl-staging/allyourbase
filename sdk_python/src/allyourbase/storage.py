"""
Stub summary for /Users/stuart/parallel_development/allyourbase_dev/MAR18_WS_C_phase5_features_and_phase6/allyourbase_dev/sdk_python/src/allyourbase/storage.py.
"""

from __future__ import annotations

from urllib.parse import urlencode
from typing import TYPE_CHECKING, Any, BinaryIO, Dict, Optional, Union

from allyourbase.types import StorageListResponse, StorageObject

if TYPE_CHECKING:
    from allyourbase.client import AYBClient


class StorageClient:
    """Handles file storage operations."""

    def __init__(self, client: AYBClient) -> None:
        self._client = client

    @staticmethod
    def _build_path(path: str, params: Dict[str, str]) -> str:
        if not params:
            return path
        return f"{path}?{urlencode(params)}"

    async def upload(
        self,
        bucket: str,
        file: Union[bytes, BinaryIO],
        name: str,
        *,
        content_type: Optional[str] = None,
    ) -> StorageObject:
        """TODO: Document StorageClient.upload."""
        url = f"{self._client.base_url}/api/storage/{bucket}"
        headers: Dict[str, str] = {}
        if self._client.token is not None:
            headers["Authorization"] = f"Bearer {self._client.token}"

        file_data = file if isinstance(file, bytes) else file.read()
        files = {"file": (name, file_data, content_type or "application/octet-stream")}

        resp = await self._client._http.post(url, headers=headers, files=files)

        if resp.status_code < 200 or resp.status_code >= 300:
            from allyourbase.client import _raise_error

            _raise_error(resp)

        return StorageObject.model_validate(resp.json())

    def download_url(self, bucket: str, name: str) -> str:
        return f"{self._client.base_url}/api/storage/{bucket}/{name}"

    async def delete(self, bucket: str, name: str) -> None:
        await self._client._request(
            f"/api/storage/{bucket}/{name}",
            method="DELETE",
        )

    async def list(
        self,
        bucket: str,
        *,
        prefix: Optional[str] = None,
        limit: Optional[int] = None,
        offset: Optional[int] = None,
    ) -> StorageListResponse:
        """TODO: Document StorageClient.list."""
        params: Dict[str, str] = {}
        if prefix is not None:
            params["prefix"] = prefix
        if limit is not None:
            params["limit"] = str(limit)
        if offset is not None:
            params["offset"] = str(offset)

        path = self._build_path(f"/api/storage/{bucket}", params)
        resp = await self._client._request(path)
        if resp is None:
            raise RuntimeError("Expected response body for storage list")
        return StorageListResponse.model_validate(resp.json())

    async def get_signed_url(
        self,
        bucket: str,
        name: str,
        *,
        expires_in: int = 3600,
    ) -> str:
        """TODO: Document StorageClient.get_signed_url."""
        resp = await self._client._request(
            f"/api/storage/{bucket}/{name}/sign",
            method="POST",
            json={"expiresIn": expires_in},
        )
        if resp is None:
            raise RuntimeError("Expected response body for signed URL")
        result: Dict[str, Any] = resp.json()
        return result["url"]
