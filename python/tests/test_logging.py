from __future__ import annotations

import datetime as dt
import io
import json
import logging
import re
import sys
from collections.abc import Iterator

import pytest
from hypothesis import given
from hypothesis import strategies as st
from opentelemetry import trace
from opentelemetry.sdk.trace import TracerProvider

from viktorbarzin_telemetry import (
    JsonFormatter,
    log_context,
    setup_logging,
)
from viktorbarzin_telemetry._logging import format_timestamp, severity, snake_case_key

TIMESTAMP_RE = re.compile(r"^\d{4}-\d\d-\d\dT\d\d:\d\d:\d\d\.\d{6}Z$")
LOG_KEY_RE = re.compile(r"^[a-z0-9_]+(\.[a-z0-9_]+)*$")


def _formatter(secrets: tuple[str, ...] = ()) -> JsonFormatter:
    return JsonFormatter(
        service_name="tv", service_version="1.2.3", environment="production", secrets=secrets
    )


def _record(
    msg: str = "hello",
    level: int = logging.INFO,
    extra: dict[str, object] | None = None,
    exc: bool = False,
) -> logging.LogRecord:
    logger = logging.getLogger("app.module")
    exc_info = None
    if exc:
        try:
            raise ValueError("boom")
        except ValueError:
            exc_info = sys.exc_info()
    return logger.makeRecord("app.module", level, "f.py", 1, msg, (), exc_info, extra=extra)


def _line(record: logging.LogRecord, fmt: JsonFormatter | None = None) -> dict[str, object]:
    out = (fmt or _formatter()).format(record)
    assert "\n" not in out
    parsed = json.loads(out)
    assert isinstance(parsed, dict)
    return parsed


def test_standard_fields() -> None:
    line = _line(_record())
    assert line["body"] == "hello"
    assert line["severity_text"] == "INFO"
    assert (line["body"], line["severity_number"]) == ("hello", 9)
    assert line["logger.name"] == "app.module"
    assert line["service.name"] == "tv"
    assert line["service.version"] == "1.2.3"
    assert line["deployment.environment.name"] == "production"
    assert isinstance(line["timestamp"], str)
    assert TIMESTAMP_RE.match(line["timestamp"])
    assert "trace_id" not in line
    assert "enduser.id" not in line


@pytest.mark.parametrize(
    ("level", "text", "number"),
    [
        (logging.DEBUG, "DEBUG", 5),
        (logging.INFO, "INFO", 9),
        (logging.WARNING, "WARN", 13),
        (logging.ERROR, "ERROR", 17),
        (logging.CRITICAL, "FATAL", 21),
    ],
)
def test_severity_mapping(level: int, text: str, number: int) -> None:
    assert severity(level) == (text, number)
    line = _line(_record(level=level))
    assert (line["severity_text"], line["severity_number"]) == (text, number)


@given(a=st.integers(min_value=0, max_value=100), b=st.integers(min_value=0, max_value=100))
def test_severity_is_monotonic(a: int, b: int) -> None:
    if a <= b:
        assert severity(a)[1] <= severity(b)[1]


@given(
    moment=st.datetimes(
        min_value=dt.datetime(1971, 1, 1),
        max_value=dt.datetime(2200, 1, 1),
        timezones=st.just(dt.timezone.utc),
    )
)
def test_timestamp_round_trips(moment: dt.datetime) -> None:
    text = format_timestamp(moment.timestamp())
    assert TIMESTAMP_RE.match(text)
    back = dt.datetime.fromisoformat(text.replace("Z", "+00:00"))
    assert abs((back - moment).total_seconds()) < 1e-5


def test_exception_fields() -> None:
    line = _line(_record(level=logging.ERROR, exc=True))
    assert line["exception.type"] == "ValueError"
    assert line["exception.message"] == "boom"
    stack = line["exception.stacktrace"]
    assert isinstance(stack, str)
    assert "Traceback" in stack
    assert "ValueError: boom" in stack


def test_extras_become_snake_case_attributes() -> None:
    line = _line(_record(extra={"channelId": 7, "duration_ms": 1.5, "http.request.method": "GET"}))
    assert line["channel_id"] == 7
    assert line["duration_ms"] == 1.5
    assert line["http.request.method"] == "GET"


def test_extras_never_overwrite_standard_fields() -> None:
    line = _line(_record(extra={"body": "x", "severity_number": 99}))
    assert (line["body"], line["severity_number"]) == ("hello", 9)


def test_unserialisable_extras_are_stringified() -> None:
    class Thing:
        def __str__(self) -> str:
            return "thing"

    line = _line(_record(extra={"thing": Thing()}))
    assert line["thing"] == "thing"


@given(key=st.text(max_size=30))
def test_snake_case_key_output_shape(key: str) -> None:
    out = snake_case_key(key)
    assert LOG_KEY_RE.match(out)
    assert snake_case_key(out) == out


@pytest.mark.parametrize(
    ("raw", "expected"),
    [
        ("channel_id", "channel_id"),
        ("channelId", "channel_id"),
        ("HTTPStatus", "http_status"),
        ("duration-ms", "duration_ms"),
        ("http.request.method", "http.request.method"),
        ("", "attribute"),
    ],
)
def test_snake_case_key_examples(raw: str, expected: str) -> None:
    assert snake_case_key(raw) == expected


def test_log_context_adds_enduser_and_attributes() -> None:
    with log_context(enduser_id="sub-123", request_id="r1"):
        line = _line(_record())
    assert line["enduser.id"] == "sub-123"
    assert line["request_id"] == "r1"
    after = _line(_record())
    assert "enduser.id" not in after
    assert "request_id" not in after


def test_log_context_nests_and_restores() -> None:
    with log_context(enduser_id="outer", a=1):
        with log_context(b=2):
            inner = _line(_record())
        outer = _line(_record())
    assert inner["enduser.id"] == "outer"
    assert (inner["a"], inner["b"]) == (1, 2)
    assert "b" not in outer


def test_trace_ids_from_context_when_no_span() -> None:
    with log_context(trace_id="a" * 32, span_id="b" * 16):
        line = _line(_record())
    assert line["trace_id"] == "a" * 32
    assert line["span_id"] == "b" * 16


@pytest.fixture
def tracer() -> Iterator[trace.Tracer]:
    provider = TracerProvider()
    yield provider.get_tracer("test")
    provider.shutdown()


def test_trace_ids_from_active_otel_span(tracer: trace.Tracer) -> None:
    with tracer.start_as_current_span("work") as span, log_context(trace_id="f" * 32):
        ctx = span.get_span_context()
        line = _line(_record())
    assert line["trace_id"] == format(ctx.trace_id, "032x")
    assert line["span_id"] == format(ctx.span_id, "016x")


def test_secrets_and_credentials_are_scrubbed() -> None:
    fmt = _formatter(secrets=("s3cr3t-value-123", "short"))
    # Joined at runtime, so the source holds no token-shaped literal.
    jwt = ".".join(["eyJhbGciOiJIUzI1NiJ9", "eyJzdWIiOiIxMjM0NSJ9", "c2lnbmF0dXJl"])
    line = fmt.format(
        _record(msg=f"key s3cr3t-value-123 auth Bearer abc.def {jwt} short", extra={"tok": jwt})
    )
    assert "s3cr3t-value-123" not in line
    assert "abc.def" not in line
    assert jwt not in line
    assert "[REDACTED]" in line
    # Values under 8 characters are not treated as secrets.
    assert "short" in line


def test_setup_logging_writes_one_json_line_per_record_to_the_stream() -> None:
    stream = io.StringIO()
    root = logging.getLogger()
    saved = (list(root.handlers), root.level)
    try:
        setup_logging(service_name="tv", service_version="1.0.0", environment="dev", stream=stream)
        logging.getLogger("x").info("one")
        logging.getLogger("x").debug("hidden")
        logging.getLogger("x").warning("two")
    finally:
        root.handlers[:] = saved[0]
        root.setLevel(saved[1])
    lines = stream.getvalue().splitlines()
    assert [json.loads(raw)["body"] for raw in lines] == ["one", "two"]
    assert json.loads(lines[0])["deployment.environment.name"] == "dev"
