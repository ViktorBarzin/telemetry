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
package telemetry
