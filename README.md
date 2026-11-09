# telemetry

Client libraries for the viktorbarzin.me telemetry standard, in Python, Go and
Kotlin (Android). The TypeScript library is `@viktorbarzin/telemetry` in
[app-kit](https://github.com/ViktorBarzin/app-kit).

Each library sets up three things for an app:

| Data | App code calls | Path |
|---|---|---|
| Product events | `track`, `identify` (Android also `screen`, `reset`) | Segment's open-source SDK, posting to the Event pipeline |
| Errors and crashes | nothing, captured automatically | Sentry SDK, posting to GlitchTip |
| Logs (backends) | its usual logger | one JSON object per line on stdout |

## Install

| Language | Package | Install |
|---|---|---|
| Python | `viktorbarzin-telemetry` on PyPI | `pip install viktorbarzin-telemetry` (add `[sentry]` for Sentry) |
| Go | `github.com/ViktorBarzin/telemetry/go` | `go get github.com/ViktorBarzin/telemetry/go@latest` |
| Android | `com.github.ViktorBarzin:telemetry` on JitPack | `implementation("com.github.ViktorBarzin:telemetry:android-v0.1.0")` with `maven("https://jitpack.io")` in `repositories` |

Usage is in [`python/README.md`](python/README.md), the Go package docs
([`go/doc.go`](go/doc.go)) and the Android section below.

## The standard

### Product events

Events follow the [Segment spec](https://segment.com/docs/connections/spec/).

| Field | Rule |
|---|---|
| `type` | `track`, `identify`, or `screen` (Android) |
| `event` | `noun.past_verb`, matching `^[a-z][a-z0-9_]*\.[a-z][a-z0-9_]*$`, e.g. `recording.started` |
| `userId` | the Person's Authentik `sub` once signed in |
| `anonymousId` | always sent |
| `properties` | snake_case keys (`^[a-z][a-z0-9_]*$`), at most 2,048 bytes as compact JSON |
| write key | one per app, in Vault `secret/rybbit` under `write_keys` |

The libraries check these rules before sending. An event that breaks one is
dropped and logged as a warning; the Event pipeline rejects such events too.
The libraries count the 2,048 limit in UTF-8 bytes, which is at least the
character count, so an event they accept is never too long for the pipeline.

Endpoints:

| Where the code runs | Event pipeline | Errors (DSN) |
|---|---|---|
| A backend in our cluster | the event-pipeline Service's URL | `https://<key>@<app host>/_t/x/<project id>` |
| Anywhere else, Android | `https://<app host>/_t/e` | the same |

The Event pipeline URL has no default in the code. Python and Go take it as
an argument (`host`, `Config.Endpoint`) or from `TELEMETRY_HOST`; the Segment
SDK appends `/v1/batch`. Production without one is an error at start-up,
and outside production none is needed. Android derives it from `appHost`.

### Production only

Events and errors leave the process only when the environment is
`production`: an explicit setting, or `TELEMETRY_ENVIRONMENT=production` for
Python and Go. On Android the `Config` default is `dev`, so a release build
passes `environment = Config.PRODUCTION`. Everywhere else the libraries still
validate, and send nothing.

### Logs

One JSON object per line on stdout, in the OpenTelemetry log data model's
field names:

| Field | Meaning |
|---|---|
| `timestamp` | RFC 3339 in UTC with `Z`: nanoseconds in Go, microseconds in Python (what `logging` records) |
| `severity_text`, `severity_number` | `DEBUG` 5, `INFO` 9, `WARN` 13, `ERROR` 17, `FATAL` 21 |
| `body` | the message |
| `logger.name` | the logger |
| `service.name`, `service.version` | app and release |
| `deployment.environment.name` | `production` or `dev` |
| `trace_id`, `span_id` | from the active OpenTelemetry span, else from the request context |
| `enduser.id` | the Person's Authentik `sub`, when set for the request |
| `exception.type`, `exception.message`, `exception.stacktrace` | for a logged error |
| other keys | snake_case attributes, e.g. `channel_id`, `duration_ms`; dots for nested groups |

Attributes never overwrite the standard fields. Each line is scrubbed of the
write key, configured secret values, bearer tokens, Basic credentials, JWTs
and PEM blocks before it is written.

Per-request context is `log_context(...)` (contextvars) in Python and
`WithEnduser`, `WithAnonymousID`, `WithTraceIDs`, `WithAttrs` on a
`context.Context` in Go.

## Android

```kotlin
class App : Application() {
    override fun onCreate() {
        super.onCreate()
        Telemetry.init(
            this,
            Config(
                appHost = "tv.viktorbarzin.me",
                writeKey = BuildConfig.SEGMENT_WRITE_KEY,
                release = BuildConfig.VERSION_NAME,
                dsn = BuildConfig.SENTRY_DSN,
                environment = if (BuildConfig.DEBUG) Config.DEV else Config.PRODUCTION,
            ),
        )
    }
}

Telemetry.track("recording.started", buildJsonObject { put("channel_id", 4) })
Telemetry.screen("Live")
Telemetry.identify(sub)
Telemetry.reset() // on sign-out
```

`analytics-kotlin` uploads to `https://<appHost>/_t/e/v1/b` and asks
`https://<appHost>/_t/e/v1/projects/<write key>/settings` for its settings.
The pipeline answers that request with a non-200, so the SDK uses the default
settings this library passes, which point its Segment destination at the same
host. Segment's own usage telemetry is switched off. The library's manifest
sets `io.sentry.auto-init` to false, because `Telemetry.init` starts Sentry
itself.

The library is built with AGP 8.5, Kotlin 1.9 and compileSdk 34, minSdk 23,
the same toolchain as the tv app, so apps on that toolchain can read it.

## Releases

One tag per package and version:

| Tag | Releases |
|---|---|
| `python-vX.Y.Z` | `viktorbarzin-telemetry` to PyPI, by `.github/workflows/release.yml` with trusted publishing. The tag must match `version` in `python/pyproject.toml` |
| `go/vX.Y.Z` | the Go module. Go requires the `go/` prefix for a module in the `go/` directory; nothing else is needed |
| `android-vX.Y.Z` | the Android library. JitPack builds it on the first request for that version (`jitpack.yml`), and the version string is the whole tag |

## Development

Builds and tests run in containers on the devvm and on GitHub-hosted runners
in CI (`.github/workflows/ci.yml`):

```sh
# Python
cd python && pip install -e . --group dev && pytest && mypy && ruff check . && ruff format --check .
# Go
cd go && go vet ./... && go test -race ./...
# Android (JDK 17, Android SDK platform 34)
cd android && ./gradlew testDebugUnitTest lintRelease assembleRelease
```

## Licence

MIT, see [LICENSE](LICENSE).
