"""Contract tests for pydantic models — matches Dart contract_test.dart fixtures."""

from __future__ import annotations

from allyourbase.types import (
    AuthResponse,
    BatchResult,
    ListResponse,
    RealtimeEvent,
    StorageObject,
    User,
)


def test_auth_response_parses_server_shape() -> None:
    raw = {
        "token": "eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiJ1c3JfMSIsImV4cCI6OTk5OTk5OTk5OX0.sig",
        "refreshToken": "rt_abc123",
        "user": {
            "id": "usr_1",
            "email": "alice@example.com",
            "createdAt": "2026-02-22T10:00:00Z",
            "updatedAt": "2026-02-22T11:00:00Z",
        },
    }
    auth = AuthResponse.model_validate(raw)
    assert auth.token.startswith("eyJ")
    assert auth.refresh_token == "rt_abc123"
    assert auth.user.id == "usr_1"
    assert auth.user.email == "alice@example.com"
    assert auth.user.created_at == "2026-02-22T10:00:00Z"
    assert auth.user.updated_at == "2026-02-22T11:00:00Z"


def test_user_parses_minimal_fields() -> None:
    raw = {
        "id": "usr_2",
        "email": "bob@example.com",
        "createdAt": "2026-02-22T10:00:00Z",
        "updatedAt": "2026-02-22T10:00:00Z",
    }
    user = User.model_validate(raw)
    assert user.id == "usr_2"
    assert user.email == "bob@example.com"
    assert user.email_verified is None


def test_list_response_parses_server_paginated_shape() -> None:
    raw = {
        "page": 1,
        "perPage": 20,
        "totalItems": 42,
        "totalPages": 3,
        "items": [
            {"id": "rec_1", "title": "Hello", "published": True},
            {"id": "rec_2", "title": "World", "published": False},
        ],
    }
    resp = ListResponse.model_validate(raw)
    assert resp.page == 1
    assert resp.per_page == 20
    assert resp.total_items == 42
    assert resp.total_pages == 3
    assert len(resp.items) == 2
    assert resp.items[0]["id"] == "rec_1"


def test_realtime_event_parses_server_sse_shape() -> None:
    raw = {
        "action": "INSERT",
        "table": "posts",
        "record": {
            "id": "rec_1",
            "title": "New Post",
            "created_at": "2026-02-22T10:00:00Z",
        },
    }
    event = RealtimeEvent.model_validate(raw)
    assert event.action == "INSERT"
    assert event.table == "posts"
    assert event.record["id"] == "rec_1"


def test_storage_object_with_user_id() -> None:
    raw = {
        "id": "obj_abc",
        "bucket": "avatars",
        "name": "profile.jpg",
        "size": 524288,
        "contentType": "image/jpeg",
        "userId": "usr_1",
        "createdAt": "2026-02-22T10:00:00Z",
        "updatedAt": "2026-02-22T11:00:00Z",
    }
    obj = StorageObject.model_validate(raw)
    assert obj.id == "obj_abc"
    assert obj.bucket == "avatars"
    assert obj.name == "profile.jpg"
    assert obj.size == 524288
    assert obj.content_type == "image/jpeg"
    assert obj.user_id == "usr_1"


def test_storage_object_without_user_id() -> None:
    raw = {
        "id": "obj_xyz",
        "bucket": "public",
        "name": "logo.png",
        "size": 1024,
        "contentType": "image/png",
        "createdAt": "2026-02-22T10:00:00Z",
        "updatedAt": "2026-02-22T10:00:00Z",
    }
    obj = StorageObject.model_validate(raw)
    assert obj.user_id is None


def test_batch_result_with_body() -> None:
    raw = {
        "index": 0,
        "status": 201,
        "body": {"id": "rec_new", "title": "Created"},
    }
    result = BatchResult.model_validate(raw)
    assert result.index == 0
    assert result.status == 201
    assert result.body["id"] == "rec_new"


def test_batch_result_without_body() -> None:
    raw = {
        "index": 2,
        "status": 204,
    }
    result = BatchResult.model_validate(raw)
    assert result.index == 2
    assert result.status == 204
    assert result.body is None


def test_models_accept_snake_case_names() -> None:
    """Models should accept both camelCase (from API) and snake_case (from Python)."""
    user = User(id="u1", email="a@b.com", email_verified=True, created_at="2026-01-01")
    assert user.email_verified is True
    assert user.created_at == "2026-01-01"
