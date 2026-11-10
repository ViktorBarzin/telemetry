"""Who sent the current request: the browser's visitor id, client IP and user agent.

Events sent while handling a request use these as the default anonymousId
and as ``context.ip`` / ``context.userAgent``, so a server event joins the
browser's person in Rybbit and gets a country and a device.
"""

from __future__ import annotations

import contextlib
import contextvars
import dataclasses
import ipaddress
import re
from collections.abc import Iterable, Iterator, Mapping

from ._context import log_context

VISITOR_HEADER = "X-Visitor-Id"
"""Header the browser sets on same-origin API calls to its Rybbit visitor id."""

MAX_USER_AGENT_LENGTH = 512

Headers = Mapping[str, str] | Iterable[tuple[str, str]]

_VISITOR_ID = re.compile(r"[A-Za-z0-9][A-Za-z0-9._:-]{0,127}")
# Segment SDKs (analytics-python, analytics-node-next, analytics-go). Rybbit
# flags these as bots, so they never stand in for a person's agent.
_SDK_AGENT = re.compile(r"analytics[-_]", re.IGNORECASE)


@dataclasses.dataclass(frozen=True)
class RequestContext:
    visitor_id: str | None = None
    client_ip: str | None = None
    user_agent: str | None = None


_EMPTY = RequestContext()
_current: contextvars.ContextVar[RequestContext] = contextvars.ContextVar(
    "viktorbarzin_telemetry_request", default=_EMPTY
)


def current_request() -> RequestContext:
    return _current.get()


def _ip(value: str | None) -> str | None:
    if not value:
        return None
    try:
        return str(ipaddress.ip_address(value.strip()))
    except ValueError:
        return None


def _lower(headers: Headers) -> dict[str, str]:
    pairs = headers.items() if isinstance(headers, Mapping) else headers
    out: dict[str, str] = {}
    for name, value in pairs:
        out.setdefault(name.lower(), value)
    return out


def request_from_headers(headers: Headers, *, peer: str | None = None) -> RequestContext:
    """Read the visitor id, client IP and user agent from request headers.

    The IP is ``X-Real-Ip`` (set by Traefik's real-ip middleware), else the
    first ``X-Forwarded-For`` entry, else ``peer``; values that are not IP
    addresses are skipped. A Segment SDK user agent counts as none.
    """
    h = _lower(headers)
    visitor = h.get(VISITOR_HEADER.lower(), "").strip()
    forwarded = h.get("x-forwarded-for", "").split(",")[0]
    agent = h.get("user-agent", "").strip()
    return RequestContext(
        visitor_id=visitor if _VISITOR_ID.fullmatch(visitor) else None,
        client_ip=_ip(h.get("x-real-ip")) or _ip(forwarded) or _ip(peer),
        user_agent=agent[:MAX_USER_AGENT_LENGTH] if agent and not _SDK_AGENT.match(agent) else None,
    )


@contextlib.contextmanager
def use_request(request: RequestContext) -> Iterator[RequestContext]:
    """Make ``request`` the current request inside the block."""
    token = _current.set(request)
    try:
        yield request
    finally:
        _current.reset(token)


@contextlib.contextmanager
def request_context(
    headers: Headers = (), *, peer: str | None = None, enduser_id: str | None = None
) -> Iterator[RequestContext]:
    """Set the current request from its headers, for frameworks without ASGI.

    ``enduser_id`` (the Person's Authentik ``sub``) also goes on the log
    context, as ``log_context(enduser_id=...)`` would.
    """
    with use_request(request_from_headers(headers, peer=peer)) as req:
        block = log_context(enduser_id=enduser_id) if enduser_id else contextlib.nullcontext()
        with block:
            yield req
