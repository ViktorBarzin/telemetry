from __future__ import annotations

import asyncio
import ipaddress

import pytest
from hypothesis import given
from hypothesis import strategies as st

from viktorbarzin_telemetry import (
    VISITOR_HEADER,
    RequestContext,
    RequestContextMiddleware,
    current_context,
    current_request,
    request_context,
    request_from_headers,
)
from viktorbarzin_telemetry._asgi import Message, Scope

BROWSER = "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 Chrome/130.0 Safari/537.36"

ipv4 = st.ip_addresses(v=4).map(str)
ips = st.ip_addresses().map(str)


@pytest.mark.parametrize(
    ("headers", "peer", "expected"),
    [
        ({"X-Real-Ip": "81.2.69.142", "X-Forwarded-For": "1.1.1.1"}, "10.0.0.1", "81.2.69.142"),
        ({"X-Forwarded-For": "81.2.69.142, 10.0.0.2"}, "10.0.0.1", "81.2.69.142"),
        ({"x-forwarded-for": " 2001:db8::1 ,10.0.0.2"}, None, "2001:db8::1"),
        ({}, "10.0.0.1", "10.0.0.1"),
        ({"X-Real-Ip": "not-an-ip", "X-Forwarded-For": "81.2.69.142"}, None, "81.2.69.142"),
        ({"X-Real-Ip": "", "X-Forwarded-For": "junk"}, "10.0.0.1", "10.0.0.1"),
        ({}, None, None),
        ({}, "testclient", None),
    ],
)
def test_client_ip_precedence(
    headers: dict[str, str], peer: str | None, expected: str | None
) -> None:
    assert request_from_headers(headers, peer=peer).client_ip == expected


@given(real=ips, forwarded=st.lists(ipv4, max_size=4), peer=st.none() | ipv4)
def test_x_real_ip_always_wins(real: str, forwarded: list[str], peer: str | None) -> None:
    headers = {"X-Real-Ip": real, "X-Forwarded-For": ", ".join(forwarded)}
    assert request_from_headers(headers, peer=peer).client_ip == real


@given(forwarded=st.lists(ips, min_size=1, max_size=4), peer=st.none() | ipv4)
def test_first_forwarded_entry_wins(forwarded: list[str], peer: str | None) -> None:
    got = request_from_headers({"X-Forwarded-For": ",".join(forwarded)}, peer=peer).client_ip
    assert got == forwarded[0]


@given(value=st.text(max_size=40))
def test_client_ip_is_an_ip_or_none(value: str) -> None:
    got = request_from_headers({"X-Real-Ip": value, "X-Forwarded-For": value}).client_ip
    if got is not None:
        ipaddress.ip_address(got)


@pytest.mark.parametrize(
    ("agent", "expected"),
    [
        (BROWSER, BROWSER),
        ("analytics-python/2.4.0", None),
        ("analytics-node-next/1.3.0", None),
        ("analytics-go (version: 3.4.0)", None),
        ("", None),
    ],
)
def test_user_agent_keeps_people_and_drops_sdks(agent: str, expected: str | None) -> None:
    assert request_from_headers({"User-Agent": agent}).user_agent == expected


def test_user_agent_is_truncated() -> None:
    assert len(request_from_headers({"user-agent": "M" * 2000}).user_agent or "") == 512


@pytest.mark.parametrize(
    ("value", "expected"),
    [
        ("2a1c7b9e-6a51-4f4a-9a0e-9e0b7c1c2d3f", "2a1c7b9e-6a51-4f4a-9a0e-9e0b7c1c2d3f"),
        ("lib-v020-check", "lib-v020-check"),
        ("  lib-v020-check ", "lib-v020-check"),
        ("", None),
        ("has space", None),
        ("<script>", None),
        ("x" * 129, None),
    ],
)
def test_visitor_id_header(value: str, expected: str | None) -> None:
    assert request_from_headers({VISITOR_HEADER: value}).visitor_id == expected


@given(name=st.sampled_from(["X-Visitor-Id", "x-visitor-id", "X-VISITOR-ID"]))
def test_header_names_are_case_insensitive(name: str) -> None:
    pairs = [(name, "v-1"), ("User-Agent", BROWSER)]
    assert request_from_headers(pairs) == RequestContext("v-1", None, BROWSER)


def test_request_context_sets_and_unwinds() -> None:
    assert current_request() == RequestContext()
    with request_context(
        {VISITOR_HEADER: "v-1", "X-Real-Ip": "81.2.69.142", "User-Agent": BROWSER},
        enduser_id="sub-1",
    ) as req:
        assert current_request() == req == RequestContext("v-1", "81.2.69.142", BROWSER)
        assert current_context().enduser_id == "sub-1"
    assert current_request() == RequestContext()
    assert current_context().enduser_id is None


async def _receive() -> Message:
    return {"type": "http.request"}


async def _send(message: Message) -> None:
    return None


def _app(seen: list[tuple[RequestContext, str | None]]) -> RequestContextMiddleware.App:
    async def app(scope: Scope, receive: object, send: object) -> None:
        seen.append((current_request(), current_context().enduser_id))

    return app


def _user_from_header(scope: Scope) -> str | None:
    headers = scope.get("headers")
    if isinstance(headers, list):
        for name, value in headers:
            if name == b"x-user":
                return bytes(value).decode()
    return None


async def _async_user(scope: Scope) -> str | None:
    return _user_from_header(scope)


@pytest.mark.parametrize("resolver", [None, _user_from_header, _async_user])
@pytest.mark.parametrize("kind", ["http", "websocket"])
def test_middleware_captures_the_request(
    resolver: RequestContextMiddleware.Resolver | None, kind: str
) -> None:
    seen: list[tuple[RequestContext, str | None]] = []
    scope: Scope = {
        "type": kind,
        "headers": [
            (b"x-visitor-id", b"v-1"),
            (b"x-forwarded-for", b"81.2.69.142, 10.0.0.9"),
            (b"user-agent", BROWSER.encode()),
            (b"x-user", b"sub-1"),
        ],
        "client": ("10.0.0.5", 4321),
    }
    asyncio.run(RequestContextMiddleware(_app(seen), resolve_user=resolver)(scope, _receive, _send))
    expected_user = None if resolver is None else "sub-1"
    assert seen == [(RequestContext("v-1", "81.2.69.142", BROWSER), expected_user)]
    assert current_request() == RequestContext()


def test_middleware_falls_back_to_the_peer() -> None:
    seen: list[tuple[RequestContext, str | None]] = []
    scope: Scope = {"type": "http", "headers": [], "client": ("81.2.69.142", 1)}
    asyncio.run(RequestContextMiddleware(_app(seen))(scope, _receive, _send))
    assert seen == [(RequestContext(None, "81.2.69.142", None), None)]


def test_middleware_skips_lifespan() -> None:
    seen: list[tuple[RequestContext, str | None]] = []

    def boom(scope: Scope) -> str | None:
        raise AssertionError("resolver called for lifespan")

    asyncio.run(RequestContextMiddleware(_app(seen), boom)({"type": "lifespan"}, _receive, _send))
    assert seen == [(RequestContext(), None)]
