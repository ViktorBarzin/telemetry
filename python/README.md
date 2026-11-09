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
- `track` takes `userId` from `log_context(enduser_id=...)` and `anonymousId`
  from `log_context(anonymous_id=...)`, or a fresh random id.
- `EnduserMiddleware(app, resolve_user=...)` sets `enduser.id` per request in
  any ASGI app. `resolve_user` gets the ASGI scope and returns the Person's
  Authentik `sub` or None, directly or as an awaitable.
- `trace_id` and `span_id` come from the active OpenTelemetry span when
  `opentelemetry-api` is installed, else from `log_context`.
- The JSON formatter masks the write key, values passed as `secrets=`, and
  bearer tokens, Basic credentials, JWTs and PEM blocks.

The standard itself is in the repository README.
