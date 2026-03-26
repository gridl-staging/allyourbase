from __future__ import annotations

from io import BytesIO

import pytest

from allyourbase.client import AYBClient
from allyourbase.types import StorageListResponse, StorageObject


_STORAGE_OBJECT = {
    "id": "obj_1",
    "bucket": "avatars",
    "name": "profile.jpg",
    "size": 3,
    "contentType": "image/jpeg",
    "createdAt": "2026-02-22T10:00:00Z",
    "updatedAt": "2026-02-22T10:00:00Z",
}


async def test_upload_bytes_multipart(httpx_mock: pytest.fixture) -> None:
    httpx_mock.add_response(status_code=201, json=_STORAGE_OBJECT)
    client = AYBClient("https://api.example.com")
    client.set_tokens("tok", "ref")

    result = await client.storage.upload("avatars", b"abc", "profile.jpg")

    assert isinstance(result, StorageObject)
    req = httpx_mock.get_request()
    assert req is not None
    assert req.method == "POST"
    assert str(req.url) == "https://api.example.com/api/storage/avatars"
    assert req.headers["authorization"] == "Bearer tok"
    assert "multipart/form-data" in req.headers["content-type"]
    assert b'filename="profile.jpg"' in req.content


async def test_upload_with_content_type(httpx_mock: pytest.fixture) -> None:
    obj = dict(_STORAGE_OBJECT)
    obj["contentType"] = "text/plain"
    obj["name"] = "note.txt"
    httpx_mock.add_response(status_code=201, json=obj)
    client = AYBClient("https://api.example.com")

    result = await client.storage.upload("avatars", BytesIO(b"abc"), "note.txt", content_type="text/plain")

    assert result.content_type == "text/plain"
    req = httpx_mock.get_request()
    assert req is not None
    assert b"Content-Type: text/plain" in req.content


def test_download_url() -> None:
    client = AYBClient("https://api.example.com/")

    assert client.storage.download_url("avatars", "a/b.jpg") == "https://api.example.com/api/storage/avatars/a/b.jpg"


async def test_delete(httpx_mock: pytest.fixture) -> None:
    httpx_mock.add_response(status_code=204)
    client = AYBClient("https://api.example.com")

    await client.storage.delete("avatars", "profile.jpg")

    req = httpx_mock.get_request()
    assert req is not None
    assert req.method == "DELETE"
    assert str(req.url) == "https://api.example.com/api/storage/avatars/profile.jpg"


async def test_list_no_params(httpx_mock: pytest.fixture) -> None:
    httpx_mock.add_response(json={"items": [_STORAGE_OBJECT], "totalItems": 1})
    client = AYBClient("https://api.example.com")

    result = await client.storage.list("avatars")

    assert isinstance(result, StorageListResponse)
    req = httpx_mock.get_request()
    assert req is not None
    assert str(req.url) == "https://api.example.com/api/storage/avatars"


async def test_list_with_params(httpx_mock: pytest.fixture) -> None:
    httpx_mock.add_response(json={"items": [], "totalItems": 0})
    client = AYBClient("https://api.example.com")

    await client.storage.list("avatars", prefix="u1/", limit=10, offset=20)

    req = httpx_mock.get_request()
    assert req is not None
    assert req.url.params["prefix"] == "u1/"
    assert req.url.params["limit"] == "10"
    assert req.url.params["offset"] == "20"


async def test_get_signed_url(httpx_mock: pytest.fixture) -> None:
    httpx_mock.add_response(json={"url": "https://signed.example.com/file"})
    client = AYBClient("https://api.example.com")

    url = await client.storage.get_signed_url("avatars", "profile.jpg", expires_in=120)

    assert url == "https://signed.example.com/file"
    req = httpx_mock.get_request()
    assert req is not None
    assert req.method == "POST"
    assert req.content == b'{"expiresIn":120}'
