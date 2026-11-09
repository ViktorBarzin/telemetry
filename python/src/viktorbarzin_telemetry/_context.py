"""Per-request context: who the Person is and which trace the work belongs to.

Kept in a ContextVar, so each asyncio task and thread sees its own value and
the value unwinds when the ``log_context`` block ends.
"""

from __future__ import annotations

import contextlib
import contextvars
import dataclasses
from collections.abc import Iterator, Mapping
from types import MappingProxyType


@dataclasses.dataclass(frozen=True)
class LogContext:
    enduser_id: str | None = None
    anonymous_id: str | None = None
    trace_id: str | None = None
    span_id: str | None = None
    attributes: Mapping[str, object] = dataclasses.field(
        default_factory=lambda: MappingProxyType({})
    )


_EMPTY = LogContext()
_current: contextvars.ContextVar[LogContext] = contextvars.ContextVar(
    "viktorbarzin_telemetry_context", default=_EMPTY
)


def current_context() -> LogContext:
    return _current.get()


@contextlib.contextmanager
def log_context(
    *,
    enduser_id: str | None = None,
    anonymous_id: str | None = None,
    trace_id: str | None = None,
    span_id: str | None = None,
    **attributes: object,
) -> Iterator[LogContext]:
    """Add fields to every log line, and ids to every event, inside the block.

    Values left as None keep whatever an enclosing block set. Keyword
    ``attributes`` merge over the enclosing ones.
    """
    outer = _current.get()
    merged = LogContext(
        enduser_id=enduser_id if enduser_id is not None else outer.enduser_id,
        anonymous_id=anonymous_id if anonymous_id is not None else outer.anonymous_id,
        trace_id=trace_id if trace_id is not None else outer.trace_id,
        span_id=span_id if span_id is not None else outer.span_id,
        attributes=MappingProxyType({**outer.attributes, **attributes}),
    )
    token = _current.set(merged)
    try:
        yield merged
    finally:
        _current.reset(token)
