"""Tests for AYBError."""

from __future__ import annotations

from allyourbase.errors import AYBError


def test_construction_with_all_fields() -> None:
    err = AYBError(
        status=409,
        message="unique violation",
        code="db/unique",
        data={"field": "email"},
        doc_url="https://docs.example.com/errors",
    )
    assert err.status == 409
    assert err.message == "unique violation"
    assert err.code == "db/unique"
    assert err.data == {"field": "email"}
    assert err.doc_url == "https://docs.example.com/errors"


def test_default_none_fields() -> None:
    err = AYBError(status=500, message="internal error")
    assert err.code is None
    assert err.data is None
    assert err.doc_url is None


def test_str_output() -> None:
    err = AYBError(status=404, message="not found")
    assert str(err) == "AYBError(status=404, message=not found)"


def test_is_exception() -> None:
    err = AYBError(status=500, message="fail")
    assert isinstance(err, Exception)


def test_can_be_raised_and_caught() -> None:
    try:
        raise AYBError(status=401, message="unauthorized", code="auth/invalid")
    except AYBError as e:
        assert e.status == 401
        assert e.code == "auth/invalid"
