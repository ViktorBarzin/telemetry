"""ASGI middleware that puts the Person and the request's sender on the context."""

from __future__ import annotations

import contextlib
from collections.abc import Awaitable, Callable, MutableMapping
from typing import TypeAlias

from ._context import log_context
from ._request import request_from_headers, use_request

Scope = MutableMapping[str, object]
Message = MutableMapping[str, object]
Receive = Callable[[], Awaitable[Message]]
Send = Callable[[Message], Awaitable[None]]
App: TypeAlias = Callable[[Scope, Receive, Send], Awaitable[None]]
Resolver: TypeAlias = Callable[[Scope], "str | Awaitable[str | None] | None"]


async def _resolve(resolve_user: Resolver, scope: Scope) -> str | None:
    result = resolve_user(scope)
    return result if result is None or isinstance(result, str) else await result


def _headers(scope: Scope) -> list[tuple[str, str]]:
    raw = scope.get("headers")
    if not isinstance(raw, list | tuple):
        return []
    out: list[tuple[str, str]] = []
    for pair in raw:
        if isinstance(pair, list | tuple) and len(pair) == 2:
            name, value = pair
            if isinstance(name, bytes) and isinstance(value, bytes):
                out.append((name.decode("latin-1"), value.decode("latin-1")))
    return out


def _peer(scope: Scope) -> str | None:
    client = scope.get("client")
    if isinstance(client, list | tuple) and client and isinstance(client[0], str):
        return client[0]
    return None


class EnduserMiddleware:
    """Sets ``enduser.id`` for each HTTP or WebSocket request.

    ``resolve_user`` gets the ASGI scope and returns the Person's Authentik
    ``sub`` (or None), directly or as an awaitable. With Starlette or FastAPI:
    ``app.add_middleware(EnduserMiddleware, resolve_user=...)``.
    """

    App: TypeAlias = Callable[[Scope, Receive, Send], Awaitable[None]]
    Resolver: TypeAlias = Callable[[Scope], "str | Awaitable[str | None] | None"]

    def __init__(self, app: App, resolve_user: Resolver) -> None:
        self.app = app
        self.resolve_user = resolve_user

    async def __call__(self, scope: Scope, receive: Receive, send: Send) -> None:
        if scope.get("type") not in ("http", "websocket"):
            await self.app(scope, receive, send)
            return
        user = await _resolve(self.resolve_user, scope)
        block = log_context(enduser_id=user) if user else contextlib.nullcontext()
        with block:
            await self.app(scope, receive, send)


class RequestContextMiddleware:
    """Captures each HTTP or WebSocket request's sender for events.

    Reads the ``X-Visitor-Id`` header (the browser's Rybbit visitor id), the
    client IP (``X-Real-Ip``, else the first ``X-Forwarded-For`` entry, else
    the peer) and the User-Agent. Events sent during the request default
    their anonymousId to the visitor id and carry ``context.ip`` and
    ``context.userAgent``. With ``resolve_user`` it also sets ``enduser.id``
    as EnduserMiddleware does. With Starlette or FastAPI:
    ``app.add_middleware(RequestContextMiddleware, resolve_user=...)``.
    """

    App: TypeAlias = Callable[[Scope, Receive, Send], Awaitable[None]]
    Resolver: TypeAlias = Callable[[Scope], "str | Awaitable[str | None] | None"]

    def __init__(self, app: App, resolve_user: Resolver | None = None) -> None:
        self.app = app
        self.resolve_user = resolve_user

    async def __call__(self, scope: Scope, receive: Receive, send: Send) -> None:
        if scope.get("type") not in ("http", "websocket"):
            await self.app(scope, receive, send)
            return
        request = request_from_headers(_headers(scope), peer=_peer(scope))
        user = await _resolve(self.resolve_user, scope) if self.resolve_user else None
        block = log_context(enduser_id=user) if user else contextlib.nullcontext()
        with use_request(request), block:
            await self.app(scope, receive, send)
