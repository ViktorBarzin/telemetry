package telemetry

import (
	"context"
	"fmt"
	"log/slog"
	"net"

	"github.com/segmentio/analytics-go/v3"
)

// SegmentClient is the part of analytics.Client this package calls.
type SegmentClient interface {
	Enqueue(analytics.Message) error
	Close() error
}

// Client sends one app's Product events. Outside production it validates
// and sends nothing.
type Client struct {
	cfg     Config
	segment SegmentClient
	log     *slog.Logger
	context analytics.Context
}

// Option changes how New builds a Client.
type Option func(*options)

type options struct {
	segment SegmentClient
	log     *slog.Logger
}

// WithSegmentClient uses c instead of building an analytics-go client (tests).
func WithSegmentClient(c SegmentClient) Option { return func(o *options) { o.segment = c } }

// WithLogger is where dropped-event warnings go; the default is slog.Default().
func WithLogger(l *slog.Logger) Option { return func(o *options) { o.log = l } }

// SegmentConfig is the analytics-go config for cfg: our endpoint, the app in
// the default context, and analytics-go's own logging routed through slog.
func SegmentConfig(cfg Config) analytics.Config {
	return analytics.Config{
		Endpoint:       cfg.endpoint(),
		Logger:         slogLogger{},
		DefaultContext: &analytics.Context{App: analytics.AppInfo{Name: cfg.App, Version: cfg.Release}},
	}
}

// New returns a Client for cfg. It registers the write key with the log
// scrubber.
func New(cfg Config, opts ...Option) (*Client, error) {
	o := options{}
	for _, opt := range opts {
		opt(&o)
	}
	if o.log == nil {
		o.log = slog.Default()
	}
	RegisterSecret(cfg.WriteKey)
	c := &Client{cfg: cfg, log: o.log, context: messageContext(cfg)}
	if !cfg.IsProduction() {
		return c, nil
	}
	c.segment = o.segment
	if c.segment == nil {
		if cfg.endpoint() == "" {
			return nil, fmt.Errorf("production telemetry needs the Event pipeline URL: set Config.Endpoint or $%s", HostVariable)
		}
		seg, err := analytics.NewWithConfig(cfg.WriteKey, SegmentConfig(cfg))
		if err != nil {
			return nil, fmt.Errorf("segment client: %w", err)
		}
		c.segment = seg
	}
	return c, nil
}

// Enabled reports whether events leave the process.
func (c *Client) Enabled() bool { return c.segment != nil }

func (c *Client) drop(ctx context.Context, kind string, err error) error {
	c.log.WarnContext(ctx, kind+" dropped", "reason", err.Error())
	return err
}

// Track queues a Product event. userId comes from WithEnduser. anonymousId
// comes from WithAnonymousID, else the request's visitor id (WithRequest or
// RequestMiddleware); without either, an event with a userId sends none and
// one without sends Config's stable fallback. context.ip and
// context.userAgent come from the request. An invalid event is dropped with
// a warning and an error wrapping ErrInvalid.
func (c *Client) Track(ctx context.Context, event string, props map[string]any) error {
	if err := ValidateEventName(event); err != nil {
		return c.drop(ctx, "event", err)
	}
	if err := ValidateProperties(props); err != nil {
		return c.drop(ctx, "event", err)
	}
	if c.segment == nil {
		return nil
	}
	user := Enduser(ctx)
	anon := anonymousID(ctx)
	if anon == "" && user == "" {
		anon = c.cfg.fallbackAnonymousID()
	}
	return c.segment.Enqueue(analytics.Track{
		Event:       event,
		UserId:      user,
		AnonymousId: anon,
		Properties:  analytics.Properties(props),
		Context:     c.messageContext(ctx),
	})
}

// Identify ties the request's anonymousId (WithAnonymousID, else the
// visitor id, else none) to the Person userID (their Authentik sub), with
// optional traits.
func (c *Client) Identify(ctx context.Context, userID string, traits map[string]any) error {
	if userID == "" {
		return c.drop(ctx, "identify", fmt.Errorf("%w: identify without a user id", ErrInvalid))
	}
	if err := ValidateProperties(traits); err != nil {
		return c.drop(ctx, "identify", err)
	}
	if c.segment == nil {
		return nil
	}
	return c.segment.Enqueue(analytics.Identify{
		UserId:      userID,
		AnonymousId: anonymousID(ctx),
		Traits:      analytics.Traits(traits),
		Context:     c.messageContext(ctx),
	})
}

// Close sends queued events and stops the client. Call it before exit.
func (c *Client) Close() error {
	if c.segment == nil {
		return nil
	}
	return c.segment.Close()
}

// messageContext is set on each message, because analytics-go sends its
// DefaultContext on the batch rather than on the messages.
func messageContext(cfg Config) analytics.Context {
	return analytics.Context{
		App:     analytics.AppInfo{Name: cfg.App, Version: cfg.Release},
		Library: analytics.LibraryInfo{Name: "analytics-go", Version: analytics.Version},
	}
}

func (c *Client) messageContext(ctx context.Context) *analytics.Context {
	mc := c.context
	req := CurrentRequest(ctx)
	if ip := net.ParseIP(req.ClientIP); ip != nil {
		mc.IP = ip
	}
	mc.UserAgent = req.UserAgent
	return &mc
}

// anonymousID is the explicit id, else the request's visitor id, else "".
// Never a random one: Rybbit would count each event as a new person.
func anonymousID(ctx context.Context) string {
	if id := AnonymousID(ctx); id != "" {
		return id
	}
	return CurrentRequest(ctx).VisitorID
}

// slogLogger routes analytics-go's messages to slog.Default().
type slogLogger struct{}

func (slogLogger) Logf(format string, args ...any) {
	slog.Debug(fmt.Sprintf(format, args...), "component", "analytics-go")
}

func (slogLogger) Errorf(format string, args ...any) {
	slog.Warn(fmt.Sprintf(format, args...), "component", "analytics-go")
}
