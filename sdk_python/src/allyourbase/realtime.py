"""Realtime SSE client for AYB."""

from __future__ import annotations

import asyncio
import json
import random
from dataclasses import dataclass, field
from typing import TYPE_CHECKING, Awaitable, Callable, List, Optional

from allyourbase.types import RealtimeEvent

if TYPE_CHECKING:
    from allyourbase.client import AYBClient


@dataclass
class RealtimeOptions:
    max_reconnect_attempts: int = 5
    reconnect_delays: List[float] = field(
        default_factory=lambda: [0.2, 0.5, 1.0, 2.0, 5.0]
    )
    jitter_max: float = 0.1
    sleep_fn: Optional[Callable[[float], Awaitable[None]]] = None
    random_fn: Optional[Callable[[], float]] = None


@dataclass
class SseMessage:
    event: Optional[str] = None
    data: Optional[str] = None
    id: Optional[str] = None
    retry: Optional[int] = None


def parse_sse_lines(lines: List[str]) -> List[SseMessage]:
    """Parse SSE lines into messages. Follows the W3C EventSource spec."""
    messages: List[SseMessage] = []
    event_type: Optional[str] = None
    data_lines: Optional[List[str]] = None
    last_id: str = ""
    retry: Optional[int] = None

    for line in lines:
        if line == "":
            # Blank line = dispatch event
            if data_lines is not None:
                messages.append(
                    SseMessage(
                        event=event_type,
                        data="\n".join(data_lines),
                        id=last_id if last_id else None,
                        retry=retry,
                    )
                )
            event_type = None
            data_lines = None
            retry = None
            continue

        if line.startswith(":"):
            # Comment — ignore
            continue

        colon_idx = line.find(":")
        if colon_idx == -1:
            field_name = line
            value = ""
        else:
            field_name = line[:colon_idx]
            value = line[colon_idx + 1 :]
            if value.startswith(" "):
                value = value[1:]

        if field_name == "event":
            event_type = value
        elif field_name == "data":
            if data_lines is None:
                data_lines = []
            data_lines.append(value)
        elif field_name == "id":
            if "\0" not in value:
                last_id = value
        elif field_name == "retry":
            try:
                parsed = int(value)
                if parsed >= 0:
                    retry = parsed
            except ValueError:
                pass

    # Flush remaining event
    if data_lines is not None:
        messages.append(
            SseMessage(
                event=event_type,
                data="\n".join(data_lines),
                id=last_id if last_id else None,
                retry=retry,
            )
        )

    return messages


class RealtimeClient:
    """Handles SSE realtime subscriptions."""

    def __init__(
        self,
        client: AYBClient,
        options: Optional[RealtimeOptions] = None,
    ) -> None:
        self._client = client
        self._options = options or RealtimeOptions()

    async def subscribe(
        self,
        tables: List[str],
        callback: Callable[[RealtimeEvent], None],
    ) -> Callable[[], None]:
        """Subscribe to realtime events. Returns an unsubscribe function."""
        cancelled = False
        reconnect_attempt = 0

        def _compute_delay(attempt: int) -> float:
            delays = self._options.reconnect_delays
            if not delays:
                return 0.0
            idx = min(attempt - 1, len(delays) - 1)
            base = delays[idx]
            jitter_max = self._options.jitter_max
            if jitter_max <= 0:
                return base
            rand_fn = self._options.random_fn or random.random
            jitter = jitter_max * min(max(rand_fn(), 0.0), 1.0)
            return base + jitter

        async def _sleep(delay: float) -> None:
            if self._options.sleep_fn is not None:
                await self._options.sleep_fn(delay)
            else:
                await asyncio.sleep(delay)

        async def _connect() -> None:
            nonlocal cancelled, reconnect_attempt

            if cancelled:
                return

            params = f"tables={','.join(tables)}"
            if self._client.token is not None:
                params += f"&token={self._client.token}"
            url = f"{self._client.base_url}/api/realtime?{params}"

            try:
                async with self._client._http.stream("GET", url) as resp:
                    if resp.status_code < 200 or resp.status_code >= 300:
                        await _schedule_reconnect()
                        return
                    buffer = ""
                    async for chunk in resp.aiter_text():
                        if cancelled:
                            return
                        buffer += chunk
                        while "\n" in buffer:
                            line, buffer = buffer.split("\n", 1)
                            _process_line(line.rstrip("\r"), callback)
            except Exception:
                if not cancelled:
                    await _schedule_reconnect()
                return

            if not cancelled:
                await _schedule_reconnect()

        # SSE line parser state
        _data_lines: List[Optional[List[str]]] = [None]

        def _process_line(
            line: str,
            cb: Callable[[RealtimeEvent], None],
        ) -> None:
            nonlocal reconnect_attempt
            if line == "":
                # Dispatch event
                if _data_lines[0] is not None:
                    data = "\n".join(_data_lines[0])
                    try:
                        parsed = json.loads(data)
                        if isinstance(parsed, dict):
                            event = RealtimeEvent.model_validate(parsed)
                            cb(event)
                            reconnect_attempt = 0
                    except (json.JSONDecodeError, ValueError):
                        pass
                _data_lines[0] = None
                return

            if line.startswith(":"):
                return

            colon_idx = line.find(":")
            if colon_idx == -1:
                field_name = line
                value = ""
            else:
                field_name = line[:colon_idx]
                value = line[colon_idx + 1 :]
                if value.startswith(" "):
                    value = value[1:]

            if field_name == "event":
                # Event type is currently not needed by RealtimeEvent payload parsing.
                return
            elif field_name == "data":
                if _data_lines[0] is None:
                    _data_lines[0] = []
                _data_lines[0].append(value)

        async def _schedule_reconnect() -> None:
            nonlocal cancelled, reconnect_attempt

            if cancelled:
                return
            if reconnect_attempt >= self._options.max_reconnect_attempts:
                return

            reconnect_attempt += 1
            delay = _compute_delay(reconnect_attempt)
            await _sleep(delay)
            # Always yield once, even when sleep_fn is a no-op.
            await asyncio.sleep(0)

            if cancelled:
                return
            await _connect()

        task = asyncio.ensure_future(_connect())

        def unsubscribe() -> None:
            nonlocal cancelled
            cancelled = True
            task.cancel()

        return unsubscribe
