package telemetry

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"regexp"
	"strings"
	"testing"
	"testing/quick"
	"time"

	"go.opentelemetry.io/otel/trace"
)

var timestampRE = regexp.MustCompile(`^\d{4}-\d\d-\d\dT\d\d:\d\d:\d\d\.\d{9}Z$`)

func newTestLogger(buf *bytes.Buffer, secrets ...string) *slog.Logger {
	return slog.New(NewHandler(buf, HandlerOptions{
		ServiceName:    "tv",
		ServiceVersion: "1.2.3",
		Environment:    "production",
		LoggerName:     "tv.api",
		Level:          slog.LevelDebug,
		Secrets:        secrets,
	}))
}

func lines(t *testing.T, buf *bytes.Buffer) []map[string]any {
	t.Helper()
	var out []map[string]any
	for _, raw := range strings.Split(strings.TrimRight(buf.String(), "\n"), "\n") {
		var m map[string]any
		if err := json.Unmarshal([]byte(raw), &m); err != nil {
			t.Fatalf("line %q is not JSON: %v", raw, err)
		}
		out = append(out, m)
	}
	return out
}

func one(t *testing.T, buf *bytes.Buffer) map[string]any {
	t.Helper()
	all := lines(t, buf)
	if len(all) != 1 {
		t.Fatalf("got %d lines, want 1: %s", len(all), buf.String())
	}
	return all[0]
}

func TestStandardFields(t *testing.T) {
	var buf bytes.Buffer
	newTestLogger(&buf).Info("hello")
	line := one(t, &buf)
	want := map[string]any{
		"body":                        "hello",
		"severity_text":               "INFO",
		"severity_number":             float64(9),
		"logger.name":                 "tv.api",
		"service.name":                "tv",
		"service.version":             "1.2.3",
		"deployment.environment.name": "production",
	}
	for k, v := range want {
		if line[k] != v {
			t.Errorf("%s = %v, want %v", k, line[k], v)
		}
	}
	ts, _ := line["timestamp"].(string)
	if !timestampRE.MatchString(ts) {
		t.Errorf("timestamp %q is not RFC 3339 UTC with nanoseconds", ts)
	}
	for _, k := range []string{"trace_id", "span_id", "enduser.id"} {
		if _, ok := line[k]; ok {
			t.Errorf("%s present without context", k)
		}
	}
}

func TestKeysComeInStandardOrder(t *testing.T) {
	var buf bytes.Buffer
	newTestLogger(&buf).Info("hello", "a", 1)
	if !strings.HasPrefix(buf.String(), `{"timestamp":`) {
		t.Fatalf("line does not start with timestamp: %s", buf.String())
	}
}

func TestSeverity(t *testing.T) {
	tests := []struct {
		level  slog.Level
		text   string
		number int
	}{
		{slog.LevelDebug - 4, "DEBUG", 5},
		{slog.LevelDebug, "DEBUG", 5},
		{slog.LevelInfo, "INFO", 9},
		{slog.LevelWarn, "WARN", 13},
		{slog.LevelError, "ERROR", 17},
		{LevelFatal, "FATAL", 21},
	}
	for _, tt := range tests {
		t.Run(tt.text, func(t *testing.T) {
			text, number := Severity(tt.level)
			if text != tt.text || number != tt.number {
				t.Fatalf("Severity(%v) = %s %d, want %s %d", tt.level, text, number, tt.text, tt.number)
			}
		})
	}
}

func TestSeverityIsMonotonic(t *testing.T) {
	property := func(a, b int8) bool {
		if a > b {
			a, b = b, a
		}
		_, na := Severity(slog.Level(a))
		_, nb := Severity(slog.Level(b))
		return na <= nb
	}
	if err := quick.Check(property, nil); err != nil {
		t.Fatal(err)
	}
}

func TestFormatTimestamp(t *testing.T) {
	tests := []struct {
		in   time.Time
		want string
	}{
		{time.Date(2026, 11, 9, 10, 0, 0, 0, time.UTC), "2026-11-09T10:00:00.000000000Z"},
		{time.Date(2026, 11, 9, 12, 0, 0, 5, time.FixedZone("x", 2*3600)), "2026-11-09T10:00:00.000000005Z"},
	}
	for _, tt := range tests {
		if got := FormatTimestamp(tt.in); got != tt.want {
			t.Errorf("FormatTimestamp(%v) = %s, want %s", tt.in, got, tt.want)
		}
	}
}

func TestAttributesAreSnakeCaseAndGroupsUseDots(t *testing.T) {
	var buf bytes.Buffer
	logger := newTestLogger(&buf).With("requestId", "r1").WithGroup("http")
	logger.Info("req", "statusCode", 200, slog.Group("request", "method", "GET"))
	line := one(t, &buf)
	if line["request_id"] != "r1" {
		t.Errorf("request_id = %v", line["request_id"])
	}
	if line["http.status_code"] != float64(200) {
		t.Errorf("http.status_code = %v", line["http.status_code"])
	}
	if line["http.request.method"] != "GET" {
		t.Errorf("http.request.method = %v", line["http.request.method"])
	}
}

func TestAttributesNeverOverwriteStandardFields(t *testing.T) {
	var buf bytes.Buffer
	newTestLogger(&buf).Info("hello", "body", "x", "severity_number", 99)
	line := one(t, &buf)
	if line["body"] != "hello" || line["severity_number"] != float64(9) {
		t.Fatalf("standard fields overwritten: %v", line)
	}
}

func TestValueKinds(t *testing.T) {
	var buf bytes.Buffer
	newTestLogger(&buf).Info("kinds",
		"duration_ms", 1.5, "ok", true, "n", int64(-3), "u", uint64(7),
		"took", 2*time.Second, "at", time.Date(2026, 1, 2, 3, 4, 5, 6, time.UTC),
		"obj", map[string]int{"a": 1}, "bad", func() {})
	line := one(t, &buf)
	checks := map[string]any{
		"duration_ms": 1.5, "ok": true, "n": float64(-3), "u": float64(7),
		"took": "2s", "at": "2026-01-02T03:04:05.000000006Z",
	}
	for k, v := range checks {
		if line[k] != v {
			t.Errorf("%s = %#v, want %#v", k, line[k], v)
		}
	}
	if obj, _ := line["obj"].(map[string]any); obj["a"] != float64(1) {
		t.Errorf("obj = %v", line["obj"])
	}
	if s, _ := line["bad"].(string); s == "" {
		t.Errorf("unencodable value not stringified: %v", line["bad"])
	}
}

type codedError struct{ code int }

func (e *codedError) Error() string { return "coded failure" }

func TestErrorsBecomeExceptionFields(t *testing.T) {
	var buf bytes.Buffer
	err := errors.Join(&codedError{code: 3})
	newTestLogger(&buf).Error("failed", "err", &codedError{code: 3}, "other", err)
	line := one(t, &buf)
	if line["exception.type"] != "*telemetry.codedError" {
		t.Errorf("exception.type = %v", line["exception.type"])
	}
	if line["exception.message"] != "coded failure" {
		t.Errorf("exception.message = %v", line["exception.message"])
	}
	if s, _ := line["exception.stacktrace"].(string); !strings.Contains(s, "TestErrorsBecomeExceptionFields") {
		t.Errorf("exception.stacktrace does not name the logging call site: %q", s)
	}
	if line["other"] != "coded failure" {
		t.Errorf("second error = %v", line["other"])
	}
}

func TestContextFields(t *testing.T) {
	var buf bytes.Buffer
	ctx := WithEnduser(context.Background(), "sub-123")
	ctx = WithAttrs(ctx, slog.String("channelId", "c1"))
	ctx = WithTraceIDs(ctx, strings.Repeat("a", 32), strings.Repeat("b", 16))
	newTestLogger(&buf).InfoContext(ctx, "hello")
	line := one(t, &buf)
	want := map[string]any{
		"enduser.id": "sub-123", "channel_id": "c1",
		"trace_id": strings.Repeat("a", 32), "span_id": strings.Repeat("b", 16),
	}
	for k, v := range want {
		if line[k] != v {
			t.Errorf("%s = %v, want %v", k, line[k], v)
		}
	}
}

func TestActiveOTelSpanWins(t *testing.T) {
	traceID, _ := trace.TraceIDFromHex("0123456789abcdef0123456789abcdef")
	spanID, _ := trace.SpanIDFromHex("0123456789abcdef")
	sc := trace.NewSpanContext(trace.SpanContextConfig{TraceID: traceID, SpanID: spanID, TraceFlags: trace.FlagsSampled})
	ctx := trace.ContextWithSpanContext(WithTraceIDs(context.Background(), strings.Repeat("f", 32), "x"), sc)
	var buf bytes.Buffer
	newTestLogger(&buf).InfoContext(ctx, "hello")
	line := one(t, &buf)
	if line["trace_id"] != traceID.String() || line["span_id"] != spanID.String() {
		t.Fatalf("ids = %v %v", line["trace_id"], line["span_id"])
	}
}

func TestSecretsAreScrubbed(t *testing.T) {
	var buf bytes.Buffer
	// Joined at runtime, so the source holds no token-shaped literal.
	jwt := strings.Join([]string{"eyJhbGciOiJIUzI1NiJ9", "eyJzdWIiOiIxMjM0NSJ9", "c2lnbmF0dXJl"}, ".")
	newTestLogger(&buf, "s3cr3t-value-123", "short").Info(
		"key s3cr3t-value-123 auth Bearer abc.def short", "tok", jwt)
	out := buf.String()
	for _, leaked := range []string{"s3cr3t-value-123", "abc.def", jwt} {
		if strings.Contains(out, leaked) {
			t.Errorf("%q leaked: %s", leaked, out)
		}
	}
	if !strings.Contains(out, "short") || !strings.Contains(out, "[REDACTED]") {
		t.Errorf("unexpected scrub result: %s", out)
	}
}

func TestLevelFilters(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(NewHandler(&buf, HandlerOptions{ServiceName: "tv"}))
	logger.Debug("hidden")
	logger.Info("shown")
	if got := lines(t, &buf); len(got) != 1 || got[0]["body"] != "shown" {
		t.Fatalf("got %v", got)
	}
}

func TestSnakeCaseKey(t *testing.T) {
	tests := map[string]string{
		"channel_id":          "channel_id",
		"channelId":           "channel_id",
		"HTTPStatus":          "http_status",
		"duration-ms":         "duration_ms",
		"http.request.method": "http.request.method",
		"":                    "attribute",
	}
	for in, want := range tests {
		if got := SnakeCaseKey(in); got != want {
			t.Errorf("SnakeCaseKey(%q) = %q, want %q", in, got, want)
		}
	}
	shape := regexp.MustCompile(`^[a-z0-9_]+(\.[a-z0-9_]+)*$`)
	property := func(key string) bool {
		out := SnakeCaseKey(key)
		return shape.MatchString(out) && SnakeCaseKey(out) == out
	}
	if err := quick.Check(property, &quick.Config{MaxCount: 2000}); err != nil {
		t.Fatal(err)
	}
}
