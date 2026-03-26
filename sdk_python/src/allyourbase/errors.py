"""AYB error types."""

from __future__ import annotations

from typing import Any, Dict, Optional


class AYBError(Exception):
    """Error thrown when the AYB API returns a non-2xx response."""

    def __init__(
        self,
        status: int,
        message: str,
        code: Optional[str] = None,
        data: Optional[Dict[str, Any]] = None,
        doc_url: Optional[str] = None,
    ) -> None:
        super().__init__(message)
        self.status = status
        self.message = message
        self.code = code
        self.data = data
        self.doc_url = doc_url

    def __str__(self) -> str:
        return f"AYBError(status={self.status}, message={self.message})"
