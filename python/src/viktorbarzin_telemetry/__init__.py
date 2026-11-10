"""Client library for the viktorbarzin.me telemetry standard.

Product events go to the Event pipeline through Segment's SDK, errors to
GlitchTip through Sentry's, and logs to stdout as OpenTelemetry-shaped JSON.
"""

from ._asgi import EnduserMiddleware, RequestContextMiddleware
from ._config import (
    ENVIRONMENT_VARIABLE,
    HOST_VARIABLE,
    PRODUCTION,
    Config,
    public_host,
    resolve_environment,
    resolve_host,
)
from ._context import LogContext, current_context, log_context
from ._events import EventClient, Telemetry, configure, flush, identify, track
from ._logging import JsonFormatter, setup_logging
from ._request import (
    VISITOR_HEADER,
    RequestContext,
    current_request,
    request_context,
    request_from_headers,
    use_request,
)
from ._scrub import Scrubber, register_secret
from ._sentry import init_sentry
from ._validation import (
    MAX_PROPERTIES_LENGTH,
    JsonValue,
    Properties,
    event_name_problem,
    properties_problem,
)

__all__ = [
    "ENVIRONMENT_VARIABLE",
    "HOST_VARIABLE",
    "MAX_PROPERTIES_LENGTH",
    "PRODUCTION",
    "VISITOR_HEADER",
    "Config",
    "EnduserMiddleware",
    "EventClient",
    "JsonFormatter",
    "JsonValue",
    "LogContext",
    "Properties",
    "RequestContext",
    "RequestContextMiddleware",
    "Scrubber",
    "Telemetry",
    "configure",
    "current_context",
    "current_request",
    "event_name_problem",
    "flush",
    "identify",
    "init_sentry",
    "log_context",
    "properties_problem",
    "public_host",
    "register_secret",
    "request_context",
    "request_from_headers",
    "resolve_environment",
    "resolve_host",
    "setup_logging",
    "track",
    "use_request",
]
