# viktorbarzin-telemetry

The Python client for the viktorbarzin.me telemetry standard: Product events
through Segment's `segment-analytics-python` to our Event pipeline, errors
through `sentry-sdk` to GlitchTip, and logs as one OpenTelemetry-shaped JSON
object per line on stdout.

```sh
pip install viktorbarzin-telemetry            # events and logs
pip install "viktorbarzin-telemetry[sentry]"  # plus Sentry
```

```python
import logging
import viktorbarzin_telemetry as telemetry

telemetry.setup_logging(service_name="tv", service_version="1.4.0")
telemetry.configure(
    app="tv",
    write_key=settings.segment_write_key,
    release="1.4.0",
    environment="production",  # or set TELEMETRY_ENVIRONMENT=production
    host=settings.event_pipeline_url,  # or set TELEMETRY_HOST
    dsn=settings.sentry_dsn,  # https://<key>@tv.viktorbarzin.me/_t/x/<project id>
)

with telemetry.log_context(enduser_id=user.sub):
    logging.getLogger(__name__).info("recording started", extra={"channel_id": 4})
    telemetry.track("recording.started", {"channel_id": 4})

telemetry.flush()  # before exit
```

- `host` is the Event pipeline's base URL, from the argument or
  `TELEMETRY_HOST`. Inside our cluster it is the event-pipeline Service's URL;
  elsewhere use `telemetry.public_host("tv.viktorbarzin.me")`. Production
  without a host raises `ValueError`.
- Nothing is sent unless the environment is `production`. Invalid events
  are validated in every environment, dropped and logged as a warning.
- `RequestContextMiddleware(app, resolve_user=...)` captures, per HTTP or
  WebSocket request, the `X-Visitor-Id` header (the browser's Rybbit visitor
  id), the client IP (`X-Real-Ip`, else the first `X-Forwarded-For` entry,
  else the peer) and the User-Agent. Events sent during the request use the
  visitor id as their default `anonymousId` and carry `context.ip` and
  `context.userAgent`. `resolve_user` is optional and works as in
  `EnduserMiddleware`. Outside ASGI, wrap the handler in
  `with telemetry.request_context(request.headers, peer=remote_addr):`.
- `track` takes `userId` from `log_context(enduser_id=...)`. `anonymousId` is
  the explicit argument, else `log_context(anonymous_id=...)`, else the
  request's visitor id. Without any of those it is omitted when there is a
  `userId`, and otherwise `server:<app>`, or `configure(...,
  default_anonymous_id=...)`. It is never a new random id per event.
- `EnduserMiddleware(app, resolve_user=...)` sets `enduser.id` per request in
  any ASGI app. `resolve_user` gets the ASGI scope and returns the Person's
  Authentik `sub` or None, directly or as an awaitable.
- `trace_id` and `span_id` come from the active OpenTelemetry span when
  `opentelemetry-api` is installed, else from `log_context`.
- The JSON formatter masks the write key, values passed as `secrets=`, and
  bearer tokens, Basic credentials, JWTs and PEM blocks.

```python
app.add_middleware(RequestContextMiddleware, resolve_user=person_from_scope)
```

Upgrading from 0.1.0: call sites keep working. A test double passed as
`client=` must now accept `anonymous_id: str | None`, because an event with a
Person and no visitor id sends none.

The standard itself is in the repository README.
