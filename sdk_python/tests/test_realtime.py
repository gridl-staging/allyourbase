from __future__ import annotations

import asyncio
import json
from collections.abc import AsyncIterator
from typing import Callable

import httpx
import pytest

from allyourbase.client import AYBClient
from allyourbase.realtime import RealtimeClient, RealtimeOptions, parse_sse_lines


class SequenceStream(httpx.AsyncByteStream):
    def __init__(self, chunks: list[bytes], delay: float = 0.0) -> None:
        self._chunks = chunks
        self._delay = delay
        self.closed = False

    async def __aiter__(self) -> AsyncIterator[bytes]:
        for chunk in self._chunks:
            if self._delay:
                await asyncio.sleep(self._delay)
            yield chunk

    async def aclose(self) -> None:
        self.closed = True


class InfiniteStream(httpx.AsyncByteStream):
    def __init__(self) -> None:
        self.closed = False

    async def __aiter__(self) -> AsyncIterator[bytes]:
        while True:
            await asyncio.sleep(1)
            yield b": heartbeat\n"

    async def aclose(self) -> None:
        self.closed = True


def test_parse_sse_lines_single_event() -> None:
    messages = parse_sse_lines([
        'data: {"action":"INSERT","table":"posts","record":{"id":"1"}}',
        "",
    ])
    assert len(messages) == 1
    assert messages[0].data == '{"action":"INSERT","table":"posts","record":{"id":"1"}}'


def test_parse_sse_lines_multi_line_data_event_and_comment_and_retry() -> None:
    messages = parse_sse_lines([
        ": comment",
        "event: update",
        "retry: 1500",
        "data: {\"action\":\"UPDATE\"," ,
        "data: \"table\":\"posts\",\"record\":{\"id\":\"1\"}}",
        "",
    ])
    assert len(messages) == 1
    assert messages[0].event == "update"
    assert messages[0].retry == 1500
    assert messages[0].data == '{"action":"UPDATE",\n"table":"posts","record":{"id":"1"}}'


@pytest.mark.asyncio
async def test_subscribe_connects_with_tables_and_token() -> None:
    seen_url: list[str] = []

    def handler(request: httpx.Request) -> httpx.Response:
        seen_url.append(str(request.url))
        return httpx.Response(200, stream=SequenceStream([b"\n"]))

    transport = httpx.MockTransport(handler)
    http_client = httpx.AsyncClient(transport=transport)
    client = AYBClient("https://api.example.com", http_client=http_client)
    client.set_tokens("tok", "ref")

    unsubscribe = await client.realtime.subscribe(["posts", "comments"], lambda _evt: None)
    await asyncio.sleep(0.05)
    unsubscribe()
    await asyncio.sleep(0)

    assert seen_url
    assert seen_url[0] == "https://api.example.com/api/realtime?tables=posts,comments&token=tok"
    await client.close()


@pytest.mark.asyncio
async def test_subscribe_callback_receives_realtime_event() -> None:
    payload = {"action": "INSERT", "table": "posts", "record": {"id": "1"}}

    def handler(_request: httpx.Request) -> httpx.Response:
        data = f"data: {json.dumps(payload)}\n\n".encode()
        return httpx.Response(200, stream=SequenceStream([data]))

    client = AYBClient(
        "https://api.example.com",
        http_client=httpx.AsyncClient(transport=httpx.MockTransport(handler)),
    )

    seen: list[dict[str, object]] = []
    hit = asyncio.Event()

    def on_event(evt: object) -> None:
        seen.append({"action": getattr(evt, "action"), "table": getattr(evt, "table")})
        hit.set()

    unsubscribe = await client.realtime.subscribe(["posts"], on_event)
    await asyncio.wait_for(hit.wait(), timeout=1)
    unsubscribe()
    await client.close()

    assert seen == [{"action": "INSERT", "table": "posts"}]


@pytest.mark.asyncio
async def test_unsubscribe_closes_connection() -> None:
    stream = InfiniteStream()

    def handler(_request: httpx.Request) -> httpx.Response:
        return httpx.Response(200, stream=stream)

    client = AYBClient(
        "https://api.example.com",
        http_client=httpx.AsyncClient(transport=httpx.MockTransport(handler)),
    )

    unsubscribe = await client.realtime.subscribe(["posts"], lambda _evt: None)
    await asyncio.sleep(0.05)
    unsubscribe()
    await asyncio.sleep(0.05)

    assert stream.closed is True
    await client.close()


@pytest.mark.asyncio
async def test_auto_reconnect_on_connection_drop() -> None:
    calls = 0
    seen: list[str] = []

    def handler(_request: httpx.Request) -> httpx.Response:
        nonlocal calls
        calls += 1
        if calls == 1:
            return httpx.Response(200, stream=SequenceStream([b"data: not-json\n\n"]))
        return httpx.Response(
            200,
            stream=SequenceStream([
                b'data: {"action":"UPDATE","table":"posts","record":{"id":"2"}}\n\n'
            ]),
        )

    async def fast_sleep(_delay: float) -> None:
        return None

    options = RealtimeOptions(max_reconnect_attempts=2, reconnect_delays=[0.0], sleep_fn=fast_sleep)
    client = AYBClient(
        "https://api.example.com",
        http_client=httpx.AsyncClient(transport=httpx.MockTransport(handler)),
    )
    realtime = RealtimeClient(client, options=options)

    done = asyncio.Event()

    def cb(evt: object) -> None:
        seen.append(getattr(evt, "action"))
        done.set()

    unsubscribe = await realtime.subscribe(["posts"], cb)
    await asyncio.wait_for(done.wait(), timeout=1)
    unsubscribe()
    await client.close()

    assert calls >= 2
    assert "UPDATE" in seen


@pytest.mark.asyncio
async def test_max_reconnect_attempts_exhausted() -> None:
    calls = 0

    def handler(_request: httpx.Request) -> httpx.Response:
        nonlocal calls
        calls += 1
        raise httpx.ConnectError("boom")

    async def fast_sleep(_delay: float) -> None:
        return None

    options = RealtimeOptions(max_reconnect_attempts=2, reconnect_delays=[0.0], sleep_fn=fast_sleep)
    client = AYBClient(
        "https://api.example.com",
        http_client=httpx.AsyncClient(transport=httpx.MockTransport(handler)),
    )
    realtime = RealtimeClient(client, options=options)

    unsubscribe = await realtime.subscribe(["posts"], lambda _evt: None)
    await asyncio.sleep(0.1)
    unsubscribe()
    await client.close()

    assert calls == 3
