"""AllYourBase Python SDK."""

from __future__ import annotations

from allyourbase.errors import AYBError
from allyourbase.types import (
    AuthResponse,
    BatchOperation,
    BatchResult,
    ListResponse,
    RealtimeEvent,
    StorageListResponse,
    StorageObject,
    User,
)

__all__ = [
    "AYBClient",
    "AYBError",
    "AuthResponse",
    "BatchOperation",
    "BatchResult",
    "ListResponse",
    "RealtimeEvent",
    "StorageListResponse",
    "StorageObject",
    "User",
]


def __getattr__(name: str) -> object:
    if name == "AYBClient":
        from allyourbase.client import AYBClient

        return AYBClient
    raise AttributeError(f"module {__name__!r} has no attribute {name!r}")
