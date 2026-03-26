"""Pydantic models for AYB API responses."""

from __future__ import annotations

from typing import Any, Dict, Generic, List, Literal, Optional, TypeVar

from pydantic import BaseModel, ConfigDict, Field

T = TypeVar("T")


class User(BaseModel):
    model_config = ConfigDict(populate_by_name=True)

    id: str
    email: str
    email_verified: Optional[bool] = Field(default=None, alias="emailVerified")
    created_at: Optional[str] = Field(default=None, alias="createdAt")
    updated_at: Optional[str] = Field(default=None, alias="updatedAt")


class AuthResponse(BaseModel):
    model_config = ConfigDict(populate_by_name=True)

    token: str
    refresh_token: str = Field(alias="refreshToken")
    user: User


class ListResponse(BaseModel, Generic[T]):
    model_config = ConfigDict(populate_by_name=True)

    items: List[T]  # type: ignore[type-arg]
    page: int
    per_page: int = Field(alias="perPage")
    total_items: int = Field(alias="totalItems")
    total_pages: int = Field(alias="totalPages")


class RealtimeEvent(BaseModel):
    model_config = ConfigDict(populate_by_name=True)

    action: str
    table: str
    record: Dict[str, Any]
    old_record: Optional[Dict[str, Any]] = Field(default=None, alias="oldRecord")


class StorageObject(BaseModel):
    model_config = ConfigDict(populate_by_name=True)

    id: str
    bucket: str
    name: str
    size: int
    content_type: str = Field(alias="contentType")
    user_id: Optional[str] = Field(default=None, alias="userId")
    created_at: str = Field(alias="createdAt")
    updated_at: Optional[str] = Field(default=None, alias="updatedAt")


class StorageListResponse(BaseModel):
    model_config = ConfigDict(populate_by_name=True)

    items: List[StorageObject]
    total_items: int = Field(alias="totalItems")


class BatchOperation(BaseModel):
    model_config = ConfigDict(populate_by_name=True)

    method: Literal["create", "update", "delete"]
    id: Optional[str] = None
    body: Optional[Dict[str, Any]] = None


class BatchResult(BaseModel, Generic[T]):
    model_config = ConfigDict(populate_by_name=True)

    index: int
    status: int
    body: Optional[T] = None  # type: ignore[type-arg]
