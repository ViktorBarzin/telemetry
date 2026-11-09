from __future__ import annotations

import logging

_log = logging.getLogger("viktorbarzin_telemetry")


def init_sentry(dsn: str, *, release: str, environment: str) -> bool:
    """Start the Sentry SDK against GlitchTip. False when sentry-sdk is not installed.

    The DSN has the form ``https://<key>@<app host>/_t/x/<project id>``.
    """
    try:
        import sentry_sdk
    except ImportError:
        _log.warning(
            "a Sentry DSN is set but sentry-sdk is not installed; "
            "install viktorbarzin-telemetry[sentry]"
        )
        return False
    sentry_sdk.init(dsn=dsn, release=release, environment=environment)
    return True
