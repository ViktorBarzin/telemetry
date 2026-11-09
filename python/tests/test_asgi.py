from __future__ import annotations

import asyncio

import pytest

from viktorbarzin_telemetry import EnduserMiddleware, current_context
from viktorbarzin_telemetry._asgi import Message, Scope


async def _receive() -> Message:
    return {"type": "http.request"}


async def _send(message: Message) -> None:
    return None


def _run(middleware: EnduserMiddleware, scope: Scope) -> None:
    asyncio.run(middleware(scope, _receive, _send))


def _recording_app(seen: list[str | None]) -> EnduserMiddleware.App:
    async def app(scope: Scope, receive: object, send: object) -> None:
        seen.append(current_context().enduser_id)

    return app


def _sync_resolver(scope: Scope) -> str | None:
    headers = scope.get("headers")
    if isinstance(headers, list):
        for name, value in headers:
            if name == b"x-user":
                return bytes(value).decode()
    return None


async def _async_resolver(scope: Scope) -> str | None:
    return _sync_resolver(scope)


@pytest.mark.parametrize("resolver", [_sync_resolver, _async_resolver])
@pytest.mark.parametrize(
    ("headers", "expected"),
    [([(b"x-user", b"sub-1")], "sub-1"), ([], None)],
)
def test_sets_enduser_for_the_request(
    resolver: EnduserMiddleware.Resolver,
    headers: list[tuple[bytes, bytes]],
    expected: str | None,
) -> None:
    seen: list[str | None] = []
    _run(EnduserMiddleware(_recording_app(seen), resolver), {"type": "http", "headers": headers})
    assert seen == [expected]
    assert current_context().enduser_id is None


def test_lifespan_skips_the_resolver() -> None:
    seen: list[str | None] = []

    def boom(scope: Scope) -> str | None:
        raise AssertionError("resolver called for lifespan")

    _run(EnduserMiddleware(_recording_app(seen), boom), {"type": "lifespan"})
    assert seen == [None]
