package telemetry

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"go.opentelemetry.io/otel/trace"
)

// LevelFatal is slog's missing level above Error, written as FATAL 21.
const LevelFatal = slog.Level(12)

// HandlerOptions configure the JSON log handler.
type HandlerOptions struct {
	ServiceName    string
	ServiceVersion string
	// Environment is deployment.environment.name; empty reads $TELEMETRY_ENVIRONMENT.
	Environment string
	// LoggerName is logger.name, omitted when empty. See Handler.Named.
	LoggerName string
	// Level is the minimum level written; nil means Info.
	Level slog.Leveler
	// Secrets are values masked in every line, on top of RegisterSecret's.
	Secrets []string
}

// Handler writes one JSON object per record in the OpenTelemetry log data
// model's field names. It is safe for concurrent use.
type Handler struct {
	w       io.Writer
	mu      *sync.Mutex
	opts    HandlerOptions
	env     string
	secrets []string
	attrs   []slog.Attr // already prefixed with their groups
	prefix  string      // open groups, joined with dots
}

// NewHandler returns a Handler writing to w.
func NewHandler(w io.Writer, opts HandlerOptions) *Handler {
	return &Handler{
		w:       w,
		mu:      &sync.Mutex{},
		opts:    opts,
		env:     ResolveEnvironment(opts.Environment),
		secrets: longestFirst(opts.Secrets),
	}
}

// SetupLogging makes a JSON Handler on stdout the slog default and returns its logger.
func SetupLogging(opts HandlerOptions) *slog.Logger {
	logger := slog.New(NewHandler(os.Stdout, opts))
	slog.SetDefault(logger)
	return logger
}

// Named returns a copy of h that writes logger.name = name.
func (h *Handler) Named(name string) *Handler {
	c := *h
	c.opts.LoggerName = name
	return &c
}

// Enabled implements slog.Handler.
func (h *Handler) Enabled(_ context.Context, level slog.Level) bool {
	threshold := slog.LevelInfo
	if h.opts.Level != nil {
		threshold = h.opts.Level.Level()
	}
	return level >= threshold
}

// WithAttrs implements slog.Handler.
func (h *Handler) WithAttrs(attrs []slog.Attr) slog.Handler {
	c := *h
	c.attrs = append([]slog.Attr{}, h.attrs...)
	for _, a := range attrs {
		c.attrs = append(c.attrs, slog.Attr{Key: joinKey(h.prefix, a.Key), Value: a.Value})
	}
	return &c
}

// WithGroup implements slog.Handler. Group names prefix keys with a dot.
func (h *Handler) WithGroup(name string) slog.Handler {
	if name == "" {
		return h
	}
	c := *h
	c.prefix = joinKey(h.prefix, name)
	return &c
}

// Handle implements slog.Handler.
func (h *Handler) Handle(ctx context.Context, r slog.Record) error {
	moment := r.Time
	if moment.IsZero() {
		moment = time.Now()
	}
	text, number := Severity(r.Level)
	line := &jsonObject{}
	line.add("timestamp", FormatTimestamp(moment))
	line.add("severity_text", text)
	line.add("severity_number", number)
	line.add("body", r.Message)
	if h.opts.LoggerName != "" {
		line.add("logger.name", h.opts.LoggerName)
	}
	if h.opts.ServiceName != "" {
		line.add("service.name", h.opts.ServiceName)
	}
	if h.opts.ServiceVersion != "" {
		line.add("service.version", h.opts.ServiceVersion)
	}
	line.add("deployment.environment.name", h.env)

	ids := contextTraceIDs(ctx)
	if sc := trace.SpanContextFromContext(ctx); sc.IsValid() {
		ids = traceIDs{sc.TraceID().String(), sc.SpanID().String()}
	}
	if ids.traceID != "" {
		line.add("trace_id", ids.traceID)
	}
	if ids.spanID != "" {
		line.add("span_id", ids.spanID)
	}
	if sub := Enduser(ctx); sub != "" {
		line.add("enduser.id", sub)
	}

	attrs := &attributes{}
	for _, a := range contextAttrs(ctx) {
		attrs.flatten("", a)
	}
	for _, a := range h.attrs {
		attrs.flatten("", a)
	}
	r.Attrs(func(a slog.Attr) bool {
		attrs.flatten(h.prefix, a)
		return true
	})
	for _, k := range attrs.keys {
		if !standardKeys[k] {
			line.addRaw(k, attrs.values[k])
		}
	}
	if attrs.err != nil {
		line.add("exception.type", fmt.Sprintf("%T", attrs.err))
		line.add("exception.message", attrs.err.Error())
		line.add("exception.stacktrace", callerStack())
	}

	out := scrub(line.String(), h.secrets) + "\n"
	h.mu.Lock()
	defer h.mu.Unlock()
	_, err := io.WriteString(h.w, out)
	return err
}

// Severity maps a slog level to OpenTelemetry severity text and number.
func Severity(level slog.Level) (string, int) {
	switch {
	case level < slog.LevelInfo:
		return "DEBUG", 5
	case level < slog.LevelWarn:
		return "INFO", 9
	case level < slog.LevelError:
		return "WARN", 13
	case level < LevelFatal:
		return "ERROR", 17
	default:
		return "FATAL", 21
	}
}

// FormatTimestamp is RFC 3339 in UTC with a Z and all nine fractional digits.
func FormatTimestamp(t time.Time) string {
	return t.UTC().Format("2006-01-02T15:04:05.000000000Z")
}

var (
	camelWord = regexp.MustCompile(`(.)([A-Z][a-z]+)`)
	camelTail = regexp.MustCompile(`([a-z0-9])([A-Z])`)
	notSnake  = regexp.MustCompile(`[^a-z0-9]+`)
)

// SnakeCaseKey turns channelId into channel_id. Dots stay, for OpenTelemetry
// names such as http.request.method.
func SnakeCaseKey(key string) string {
	var parts []string
	for _, segment := range strings.Split(key, ".") {
		words := camelTail.ReplaceAllString(camelWord.ReplaceAllString(segment, "${1}_${2}"), "${1}_${2}")
		cleaned := strings.Trim(notSnake.ReplaceAllString(strings.ToLower(words), "_"), "_")
		if cleaned != "" {
			parts = append(parts, cleaned)
		}
	}
	if len(parts) == 0 {
		return "attribute"
	}
	return strings.Join(parts, ".")
}

var standardKeys = map[string]bool{
	"timestamp": true, "severity_text": true, "severity_number": true, "body": true,
	"logger.name": true, "service.name": true, "service.version": true,
	"deployment.environment.name": true, "trace_id": true, "span_id": true,
	"enduser.id": true, "exception.type": true, "exception.message": true,
	"exception.stacktrace": true,
}

func joinKey(prefix, key string) string {
	if prefix == "" {
		return key
	}
	if key == "" {
		return prefix
	}
	return prefix + "." + key
}

// attributes collects flattened, snake_cased attributes; a later key wins.
type attributes struct {
	keys   []string
	values map[string]json.RawMessage
	err    error
}

func (a *attributes) set(key string, raw json.RawMessage) {
	if a.values == nil {
		a.values = map[string]json.RawMessage{}
	}
	if _, ok := a.values[key]; !ok {
		a.keys = append(a.keys, key)
	}
	a.values[key] = raw
}

func (a *attributes) flatten(prefix string, attr slog.Attr) {
	attr.Value = attr.Value.Resolve()
	if attr.Equal(slog.Attr{}) {
		return
	}
	if attr.Value.Kind() == slog.KindGroup {
		for _, member := range attr.Value.Group() {
			a.flatten(joinKey(prefix, attr.Key), member)
		}
		return
	}
	key := SnakeCaseKey(joinKey(prefix, attr.Key))
	if err, ok := attr.Value.Any().(error); ok && attr.Value.Kind() == slog.KindAny {
		if a.err == nil {
			a.err = err
			return
		}
		a.set(key, mustMarshal(err.Error()))
		return
	}
	a.set(key, valueJSON(attr.Value))
}

func valueJSON(v slog.Value) json.RawMessage {
	switch v.Kind() {
	case slog.KindString:
		return mustMarshal(v.String())
	case slog.KindInt64:
		return json.RawMessage(strconv.FormatInt(v.Int64(), 10))
	case slog.KindUint64:
		return json.RawMessage(strconv.FormatUint(v.Uint64(), 10))
	case slog.KindBool:
		return json.RawMessage(strconv.FormatBool(v.Bool()))
	case slog.KindDuration:
		return mustMarshal(v.Duration().String())
	case slog.KindTime:
		return mustMarshal(FormatTimestamp(v.Time()))
	case slog.KindFloat64:
		if raw, err := marshal(v.Float64()); err == nil {
			return raw
		}
		return mustMarshal(v.String())
	default:
		if raw, err := marshal(v.Any()); err == nil {
			return raw
		}
		return mustMarshal(fmt.Sprint(v.Any()))
	}
}

func marshal(v any) (json.RawMessage, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return nil, err
	}
	return bytes.TrimRight(buf.Bytes(), "\n"), nil
}

// mustMarshal encodes a value that always encodes: a string.
func mustMarshal(s string) json.RawMessage {
	raw, err := marshal(s)
	if err != nil {
		return json.RawMessage(`""`)
	}
	return raw
}

// jsonObject writes keys in insertion order.
type jsonObject struct{ buf bytes.Buffer }

func (o *jsonObject) addRaw(key string, raw json.RawMessage) {
	if o.buf.Len() == 0 {
		o.buf.WriteByte('{')
	} else {
		o.buf.WriteByte(',')
	}
	o.buf.Write(mustMarshal(key))
	o.buf.WriteByte(':')
	o.buf.Write(raw)
}

func (o *jsonObject) add(key string, v any) {
	raw, err := marshal(v)
	if err != nil {
		raw = mustMarshal(fmt.Sprint(v))
	}
	o.addRaw(key, raw)
}

func (o *jsonObject) String() string { return o.buf.String() + "}" }

// callerStack is the goroutine's stack from the logging call outwards,
// without the slog and Handler frames.
func callerStack() string {
	pcs := make([]uintptr, 64)
	n := runtime.Callers(3, pcs)
	frames := runtime.CallersFrames(pcs[:n])
	var b strings.Builder
	inLogger := true
	for {
		f, more := frames.Next()
		if inLogger && (strings.HasPrefix(f.Function, "log/slog.") || strings.Contains(f.Function, "telemetry/go.(*Handler)")) {
			if !more {
				break
			}
			continue
		}
		inLogger = false
		fmt.Fprintf(&b, "%s\n\t%s:%d\n", f.Function, f.File, f.Line)
		if !more {
			break
		}
	}
	return b.String()
}
