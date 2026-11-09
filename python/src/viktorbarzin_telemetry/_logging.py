"""JSON log lines in the OpenTelemetry log data model field names."""

from __future__ import annotations

import datetime as _dt
import functools
import json
import logging
import re
import sys
from collections.abc import Callable, Iterable
from typing import TextIO

from ._config import resolve_environment
from ._context import current_context
from ._scrub import Scrubber

# LogRecord's own attributes, never echoed as extra fields.
_RECORD_KEYS = frozenset(vars(logging.LogRecord("", 0, "", 0, "", (), None))) | {
    "message",
    "asctime",
    "color_message",
}

_STANDARD_KEYS = frozenset(
    {
        "timestamp",
        "severity_text",
        "severity_number",
        "body",
        "logger.name",
        "service.name",
        "service.version",
        "deployment.environment.name",
        "trace_id",
        "span_id",
        "enduser.id",
        "exception.type",
        "exception.message",
        "exception.stacktrace",
        "code.stacktrace",
    }
)

_CAMEL_WORD = re.compile(r"(.)([A-Z][a-z]+)")
_CAMEL_TAIL = re.compile(r"([a-z0-9])([A-Z])")
_NOT_SNAKE = re.compile(r"[^a-z0-9]+")


def severity(levelno: int) -> tuple[str, int]:
    """OpenTelemetry severity text and number for a ``logging`` level."""
    if levelno < logging.INFO:
        return "DEBUG", 5
    if levelno < logging.WARNING:
        return "INFO", 9
    if levelno < logging.ERROR:
        return "WARN", 13
    if levelno < logging.CRITICAL:
        return "ERROR", 17
    return "FATAL", 21


def format_timestamp(created: float) -> str:
    """RFC 3339 in UTC with a ``Z``, to the microsecond ``logging`` records."""
    moment = _dt.datetime.fromtimestamp(created, tz=_dt.timezone.utc)
    return moment.strftime("%Y-%m-%dT%H:%M:%S.%fZ")


def snake_case_key(key: str) -> str:
    """``channelId`` to ``channel_id``. Dots stay, for OTel names like ``http.request.method``."""
    parts: list[str] = []
    for segment in key.split("."):
        words = _CAMEL_TAIL.sub(r"\1_\2", _CAMEL_WORD.sub(r"\1_\2", segment)).lower()
        cleaned = _NOT_SNAKE.sub("_", words).strip("_")
        if cleaned:
            parts.append(cleaned)
    return ".".join(parts) or "attribute"


SpanIds = Callable[[], "tuple[str, str] | None"]


@functools.cache
def _otel_span_ids() -> SpanIds | None:
    """A reader for the active OTel span's ids, or None without opentelemetry-api."""
    try:
        from opentelemetry import trace
    except ImportError:
        return None

    def read() -> tuple[str, str] | None:
        ctx = trace.get_current_span().get_span_context()
        if not ctx.is_valid:
            return None
        return format(ctx.trace_id, "032x"), format(ctx.span_id, "016x")

    return read


def _exception_type(exc_type: type[BaseException]) -> str:
    if exc_type.__module__ == "builtins":
        return exc_type.__qualname__
    return f"{exc_type.__module__}.{exc_type.__qualname__}"


class JsonFormatter(logging.Formatter):
    """One JSON object per record, scrubbed of secrets."""

    def __init__(
        self,
        *,
        service_name: str,
        service_version: str,
        environment: str | None = None,
        secrets: Iterable[str] = (),
    ) -> None:
        super().__init__()
        self._service = {
            "service.name": service_name,
            "service.version": service_version,
            "deployment.environment.name": resolve_environment(environment),
        }
        self._scrubber = Scrubber(secrets)

    def format(self, record: logging.LogRecord) -> str:
        text, number = severity(record.levelno)
        payload: dict[str, object] = {
            "timestamp": format_timestamp(record.created),
            "severity_text": text,
            "severity_number": number,
            "body": record.getMessage(),
            "logger.name": record.name,
            **self._service,
        }
        ctx = current_context()
        reader = _otel_span_ids()
        span = reader() if reader is not None else None
        trace_id, span_id = span if span is not None else (ctx.trace_id, ctx.span_id)
        if trace_id:
            payload["trace_id"] = trace_id
        if span_id:
            payload["span_id"] = span_id
        if ctx.enduser_id:
            payload["enduser.id"] = ctx.enduser_id

        attributes: dict[str, object] = {}
        for key, value in ctx.attributes.items():
            attributes[snake_case_key(key)] = value
        for key, value in vars(record).items():
            if key not in _RECORD_KEYS and not key.startswith("_"):
                attributes[snake_case_key(key)] = value
        for key, value in attributes.items():
            if key not in _STANDARD_KEYS:
                payload[key] = value

        if record.exc_info and record.exc_info[0] is not None:
            exc_type, exc, _ = record.exc_info
            payload["exception.type"] = _exception_type(exc_type)
            payload["exception.message"] = str(exc)
            payload["exception.stacktrace"] = self.formatException(record.exc_info)
        if record.stack_info:
            payload["code.stacktrace"] = record.stack_info
        return self._scrubber.scrub(_dumps(payload))


def _dumps(payload: dict[str, object]) -> str:
    try:
        return json.dumps(payload, ensure_ascii=False, default=str)
    except ValueError:
        # A circular structure: keep the line, as text.
        flat = {
            k: v if isinstance(v, (str, int, float, bool)) else str(v) for k, v in payload.items()
        }
        return json.dumps(flat, ensure_ascii=False)


def setup_logging(
    *,
    service_name: str,
    service_version: str,
    environment: str | None = None,
    level: int | str = logging.INFO,
    stream: TextIO | None = None,
    secrets: Iterable[str] = (),
) -> logging.Handler:
    """Make the root logger write JSON lines to stdout (or ``stream``), replacing its handlers."""
    handler = logging.StreamHandler(stream if stream is not None else sys.stdout)
    handler.setFormatter(
        JsonFormatter(
            service_name=service_name,
            service_version=service_version,
            environment=environment,
            secrets=secrets,
        )
    )
    root = logging.getLogger()
    for existing in list(root.handlers):
        root.removeHandler(existing)
    root.addHandler(handler)
    root.setLevel(level)
    return handler
