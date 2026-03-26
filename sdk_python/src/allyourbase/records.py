"""
Stub summary for /Users/stuart/parallel_development/allyourbase_dev/MAR18_WS_C_phase5_features_and_phase6/allyourbase_dev/sdk_python/src/allyourbase/records.py.
"""

from __future__ import annotations

from urllib.parse import urlencode
from typing import TYPE_CHECKING, Any, Dict, List, Optional

from allyourbase.types import BatchOperation, BatchResult, ListResponse

if TYPE_CHECKING:
    from allyourbase.client import AYBClient


class RecordsClient:
    """Handles CRUD operations on collections."""

    def __init__(self, client: AYBClient) -> None:
        self._client = client

    @staticmethod
    def _build_path(path: str, params: Dict[str, str]) -> str:
        if not params:
            return path
        return f"{path}?{urlencode(params)}"

    async def list(
        self,
        collection: str,
        *,
        page: Optional[int] = None,
        per_page: Optional[int] = None,
        sort: Optional[str] = None,
        filter: Optional[str] = None,
        search: Optional[str] = None,
        fields: Optional[str] = None,
        expand: Optional[str] = None,
        skip_total: bool = False,
    ) -> ListResponse[Dict[str, Any]]:
        """TODO: Document RecordsClient.list."""
        params: Dict[str, str] = {}
        if page is not None:
            params["page"] = str(page)
        if per_page is not None:
            params["perPage"] = str(per_page)
        if sort is not None:
            params["sort"] = sort
        if filter is not None:
            params["filter"] = filter
        if search is not None:
            params["search"] = search
        if fields is not None:
            params["fields"] = fields
        if expand is not None:
            params["expand"] = expand
        if skip_total:
            params["skipTotal"] = "true"

        path = self._build_path(f"/api/collections/{collection}", params)
        resp = await self._client._request(path)
        if resp is None:
            raise RuntimeError("Expected response body for list")
        return ListResponse[Dict[str, Any]].model_validate(resp.json())

    async def get(
        self,
        collection: str,
        id: str,
        *,
        fields: Optional[str] = None,
        expand: Optional[str] = None,
    ) -> Dict[str, Any]:
        """TODO: Document RecordsClient.get."""
        params: Dict[str, str] = {}
        if fields is not None:
            params["fields"] = fields
        if expand is not None:
            params["expand"] = expand

        path = self._build_path(f"/api/collections/{collection}/{id}", params)
        resp = await self._client._request(path)
        if resp is None:
            raise RuntimeError("Expected response body for get")
        result: Dict[str, Any] = resp.json()
        return result

    async def create(
        self,
        collection: str,
        data: Dict[str, Any],
    ) -> Dict[str, Any]:
        resp = await self._client._request(
            f"/api/collections/{collection}",
            method="POST",
            json=data,
        )
        if resp is None:
            raise RuntimeError("Expected response body for create")
        result: Dict[str, Any] = resp.json()
        return result

    async def update(
        self,
        collection: str,
        id: str,
        data: Dict[str, Any],
    ) -> Dict[str, Any]:
        resp = await self._client._request(
            f"/api/collections/{collection}/{id}",
            method="PATCH",
            json=data,
        )
        if resp is None:
            raise RuntimeError("Expected response body for update")
        result: Dict[str, Any] = resp.json()
        return result

    async def delete(self, collection: str, id: str) -> None:
        await self._client._request(
            f"/api/collections/{collection}/{id}",
            method="DELETE",
        )

    async def batch(
        self,
        collection: str,
        operations: List[BatchOperation],
    ) -> List[BatchResult[Dict[str, Any]]]:
        """TODO: Document RecordsClient.batch."""
        resp = await self._client._request(
            f"/api/collections/{collection}/batch",
            method="POST",
            json={
                "operations": [op.model_dump(exclude_none=True) for op in operations]
            },
        )
        if resp is None:
            raise RuntimeError("Expected response body for batch")
        raw_list: List[Dict[str, Any]] = resp.json()
        return [BatchResult[Dict[str, Any]].model_validate(item) for item in raw_list]
