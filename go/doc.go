// Package telemetry is the Go client for the viktorbarzin.me telemetry
// standard.
//
// Product events go to the Event pipeline through Segment's analytics-go,
// errors to GlitchTip through sentry-go, and logs to stdout as one JSON
// object per line with OpenTelemetry field names, through a [log/slog]
// handler.
//
// Events leave the process only when the environment is "production",
// set in [Config] or through $TELEMETRY_ENVIRONMENT. Everywhere else the
// client validates and sends nothing.
//
// Wrap HTTP handlers in [RequestMiddleware] so events sent while handling a
// request carry the browser's visitor id (the X-Visitor-Id header) as their
// anonymousId, and the person's IP and User-Agent as context.ip and
// context.userAgent. An event with neither a visitor id nor an explicit
// anonymousId sends none when it has a userId, and otherwise the stable
// [Config.DefaultAnonymousID], never a random id per event.
package telemetry
