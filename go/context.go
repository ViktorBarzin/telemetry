package telemetry

import (
	"context"
	"log/slog"
)

type contextKey int

const (
	enduserKey contextKey = iota
	anonymousKey
	traceKey
	attrsKey
	requestKey
)

type traceIDs struct{ traceID, spanID string }

// WithEnduser sets the Person's Authentik sub. Log lines get enduser.id and
// events get userId.
func WithEnduser(ctx context.Context, sub string) context.Context {
	return context.WithValue(ctx, enduserKey, sub)
}

// Enduser returns the sub set by WithEnduser, or "".
func Enduser(ctx context.Context) string {
	s, _ := ctx.Value(enduserKey).(string)
	return s
}

// WithAnonymousID sets the anonymousId sent with events. It overrides the
// request's visitor id (see Request).
func WithAnonymousID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, anonymousKey, id)
}

// AnonymousID returns the id set by WithAnonymousID, or "".
func AnonymousID(ctx context.Context) string {
	s, _ := ctx.Value(anonymousKey).(string)
	return s
}

// WithTraceIDs sets trace_id and span_id for log lines when no OpenTelemetry
// span is active in ctx, e.g. ids taken from an incoming traceparent header.
func WithTraceIDs(ctx context.Context, traceID, spanID string) context.Context {
	return context.WithValue(ctx, traceKey, traceIDs{traceID, spanID})
}

// WithAttrs adds attributes to every log line written with ctx.
func WithAttrs(ctx context.Context, attrs ...slog.Attr) context.Context {
	prev, _ := ctx.Value(attrsKey).([]slog.Attr)
	merged := make([]slog.Attr, 0, len(prev)+len(attrs))
	merged = append(append(merged, prev...), attrs...)
	return context.WithValue(ctx, attrsKey, merged)
}

func contextAttrs(ctx context.Context) []slog.Attr {
	a, _ := ctx.Value(attrsKey).([]slog.Attr)
	return a
}

func contextTraceIDs(ctx context.Context) traceIDs {
	t, _ := ctx.Value(traceKey).(traceIDs)
	return t
}
