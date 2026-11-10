"""Product events through Segment's Python SDK, pointed at the Event pipeline."""

from __future__ import annotations

import logging
from typing import Protocol

from segment.analytics.client import Client

from ._config import HOST_VARIABLE, Config, resolve_environment, resolve_host
from ._context import current_context
from ._request import current_request
from ._scrub import register_secret
from ._sentry import init_sentry
from ._validation import JsonValue, Properties, event_name_problem, properties_problem

_log = logging.getLogger("viktorbarzin_telemetry")


class EventClient(Protocol):
    """The part of ``segment.analytics.Client`` this package calls."""

    def track(
        self,
        *,
        user_id: str | None,
        event: str,
        properties: dict[str, JsonValue],
        context: dict[str, JsonValue],
        anonymous_id: str | None,
    ) -> object: ...

    def identify(
        self,
        *,
        user_id: str,
        traits: dict[str, JsonValue],
        context: dict[str, JsonValue],
        anonymous_id: str | None,
    ) -> object: ...

    def flush(self) -> None: ...


def _on_error(err: Exception, batch: list[dict[str, object]]) -> None:
    _log.warning("sending %d events failed: %s", len(batch), err)


def segment_client(config: Config) -> Client:
    if not config.host:
        raise ValueError(
            f"production telemetry needs the Event pipeline URL: pass host or set ${HOST_VARIABLE}"
        )
    return Client(config.write_key, host=config.host, on_error=_on_error)


class Telemetry:
    """One app's event sender. Outside production it validates and sends nothing."""

    def __init__(self, config: Config, client: EventClient | None = None) -> None:
        self.config = config
        self._client: EventClient | None = None
        if config.production:
            self._client = client if client is not None else segment_client(config)
        self._context: dict[str, JsonValue] = {
            "app": {"name": config.app, "version": config.release}
        }

    @property
    def enabled(self) -> bool:
        """True when events leave the process."""
        return self._client is not None

    @property
    def segment_client(self) -> Client | None:
        return self._client if isinstance(self._client, Client) else None

    def _ids(self, user_id: str | None, anonymous_id: str | None) -> tuple[str | None, str | None]:
        """The Person and anonymous ids for one message.

        anonymousId is the explicit one, else ``log_context``'s, else the
        request's visitor id. Without any of those, a message with a Person
        needs none, and one without uses the config's stable fallback; never
        a fresh random id, which Rybbit would count as a new person.
        """
        ctx = current_context()
        user = user_id if user_id is not None else ctx.enduser_id
        anon = anonymous_id or ctx.anonymous_id or current_request().visitor_id
        if anon is None and not user:
            anon = self.config.fallback_anonymous_id
        return user, anon

    def _message_context(self) -> dict[str, JsonValue]:
        context = dict(self._context)
        req = current_request()
        if req.client_ip:
            context["ip"] = req.client_ip
        if req.user_agent:
            context["userAgent"] = req.user_agent
        return context

    def track(
        self,
        event: str,
        properties: Properties | None = None,
        *,
        user_id: str | None = None,
        anonymous_id: str | None = None,
    ) -> bool:
        """Queue a Product event. False when it was invalid and dropped."""
        props = dict(properties or {})
        problem = event_name_problem(event) or properties_problem(props)
        if problem is not None:
            _log.warning("event dropped: %s", problem, extra={"event_name": event})
            return False
        if self._client is not None:
            user, anon = self._ids(user_id, anonymous_id)
            self._client.track(
                user_id=user,
                event=event,
                properties=props,
                context=self._message_context(),
                anonymous_id=anon,
            )
        return True

    def identify(
        self,
        user_id: str,
        traits: Properties | None = None,
        *,
        anonymous_id: str | None = None,
    ) -> bool:
        """Tie ``anonymous_id`` to the Person ``user_id`` (their Authentik ``sub``)."""
        values = dict(traits or {})
        problem = "identify without a user id" if not user_id else properties_problem(values)
        if problem is not None:
            _log.warning("identify dropped: %s", problem)
            return False
        if self._client is not None:
            _, anon = self._ids(user_id, anonymous_id)
            self._client.identify(
                user_id=user_id,
                traits=values,
                context=self._message_context(),
                anonymous_id=anon,
            )
        return True

    def flush(self) -> None:
        """Block until queued events are sent."""
        if self._client is not None:
            self._client.flush()


_default = Telemetry(Config(app="unconfigured", write_key="", release="0", environment="unset"))


def configure(
    app: str,
    write_key: str,
    release: str,
    environment: str | None = None,
    dsn: str | None = None,
    host: str | None = None,
    *,
    client: EventClient | None = None,
    default_anonymous_id: str | None = None,
) -> Telemetry:
    """Set up events (and Sentry when ``dsn`` is given) for this process.

    Sending happens only when the environment, explicit or from
    ``$TELEMETRY_ENVIRONMENT``, is ``production``. ``host`` is the Event
    pipeline's base URL, explicit or from ``$TELEMETRY_HOST``; production
    without one raises ValueError. ``default_anonymous_id`` is the
    anonymousId of events with neither a Person nor a visitor id; it
    defaults to ``server:<app>``.
    """
    global _default
    config = Config(
        app=app,
        write_key=write_key,
        release=release,
        environment=resolve_environment(environment),
        host=resolve_host(host),
        dsn=dsn,
        default_anonymous_id=default_anonymous_id,
    )
    register_secret(write_key)
    _default = Telemetry(config, client=client)
    if dsn and config.production:
        init_sentry(dsn, release=release, environment=config.environment)
    return _default


def track(
    event: str,
    properties: Properties | None = None,
    *,
    user_id: str | None = None,
    anonymous_id: str | None = None,
) -> bool:
    return _default.track(event, properties, user_id=user_id, anonymous_id=anonymous_id)


def identify(
    user_id: str, traits: Properties | None = None, *, anonymous_id: str | None = None
) -> bool:
    return _default.identify(user_id, traits, anonymous_id=anonymous_id)


def flush() -> None:
    _default.flush()
