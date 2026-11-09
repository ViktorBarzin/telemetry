"""ASGI middleware that puts the signed-in Person on the request's log context."""

from __future__ import annotations

import contextlib
from collections.abc import Awaitable, Callable, MutableMapping

from ._context import log_context

Scope = MutableMapping[str, object]
Message = MutableMapping[str, object]
Receive = Callable[[], Awaitable[Message]]
Send = Callable[[Message], Awaitable[None]]


class EnduserMiddleware:
    """Sets ``enduser.id`` for each HTTP or WebSocket request.

    ``resolve_user`` gets the ASGI scope and returns the Person's Authentik
    ``sub`` (or None), directly or as an awaitable. With Starlette or FastAPI:
    ``app.add_middleware(EnduserMiddleware, resolve_user=...)``.
    """

    App = Callable[[Scope, Receive, Send], Awaitable[None]]
    Resolver = Callable[[Scope], "str | Awaitable[str | None] | None"]

    def __init__(self, app: App, resolve_user: Resolver) -> None:
        self.app = app
        self.resolve_user = resolve_user

    async def __call__(self, scope: Scope, receive: Receive, send: Send) -> None:
        if scope.get("type") not in ("http", "websocket"):
            await self.app(scope, receive, send)
            return
        result = self.resolve_user(scope)
        user = result if result is None or isinstance(result, str) else await result
        block = log_context(enduser_id=user) if user else contextlib.nullcontext()
        with block:
            await self.app(scope, receive, send)
