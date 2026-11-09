from __future__ import annotations

import dataclasses
import os

PRODUCTION = "production"
ENVIRONMENT_VARIABLE = "TELEMETRY_ENVIRONMENT"
HOST_VARIABLE = "TELEMETRY_HOST"


def resolve_environment(explicit: str | None = None) -> str:
    """The explicit value, else ``$TELEMETRY_ENVIRONMENT``, else ``dev``."""
    if explicit is not None:
        return explicit
    return os.environ.get(ENVIRONMENT_VARIABLE) or "dev"


def resolve_host(explicit: str | None = None) -> str | None:
    """The explicit Event pipeline URL, else ``$TELEMETRY_HOST``, else None."""
    if explicit:
        return explicit
    return os.environ.get(HOST_VARIABLE) or None


def public_host(app_host: str) -> str:
    """The Event pipeline on an app's public host, for code outside the cluster."""
    return f"https://{app_host}/_t/e"


@dataclasses.dataclass(frozen=True)
class Config:
    """One app's telemetry settings.

    ``host`` is the Event pipeline's base URL; the Segment client appends
    ``/v1/batch``. Production needs one. Inside our cluster it is the
    pipeline's Service; elsewhere it is ``public_host("<app host>")``.
    """

    app: str
    write_key: str
    release: str
    environment: str
    host: str | None = None
    dsn: str | None = None

    @property
    def production(self) -> bool:
        return self.environment == PRODUCTION
