from __future__ import annotations

import os
import uuid

import pytest

from allyourbase import AYBClient, AYBError


@pytest.mark.skipif("AYB_TEST_URL" not in os.environ, reason="AYB_TEST_URL is not set")
@pytest.mark.asyncio
async def test_e2e_contract_live_server() -> None:
    base_url = os.environ["AYB_TEST_URL"]
    collection = os.environ.get("AYB_TEST_COLLECTION", "posts")
    email = f"sdkpy-{uuid.uuid4().hex[:12]}@example.com"
    password = "P@ssw0rd!123"

    client = AYBClient(base_url)
    try:
        auth = await client.auth.register(email, password)
        assert auth.token
        assert auth.refresh_token
        assert auth.user.email == email

        created = await client.records.create(collection, {"title": "sdk python e2e"})
        record_id = str(created.get("id"))
        assert record_id

        listed = await client.records.list(collection)
        assert any(str(item.get("id")) == record_id for item in listed.items)

        fetched = await client.records.get(collection, record_id)
        assert str(fetched.get("id")) == record_id

        updated = await client.records.update(collection, record_id, {"title": "sdk python e2e updated"})
        assert str(updated.get("id")) == record_id

        await client.records.delete(collection, record_id)

        with pytest.raises(AYBError) as exc:
            await client.records.get(collection, record_id)
        assert exc.value.status == 404

        await client.auth.logout()
        assert client.token is None
    finally:
        await client.close()
