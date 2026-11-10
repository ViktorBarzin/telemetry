package telemetry

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"net"
	"strings"
	"sync"
	"testing"
	"testing/quick"

	"github.com/segmentio/analytics-go/v3"
)

type fakeSegment struct {
	mu     sync.Mutex
	msgs   []analytics.Message
	closed int
}

func (f *fakeSegment) Enqueue(m analytics.Message) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.msgs = append(f.msgs, m)
	return nil
}

func (f *fakeSegment) Close() error {
	f.closed++
	return nil
}

func prodConfig() Config {
	return Config{App: "tv", WriteKey: "wk", Release: "1.2.3", Environment: "production"}
}

func newProd(t *testing.T) (*Client, *fakeSegment, *bytes.Buffer) {
	t.Helper()
	fake := &fakeSegment{}
	var warnings bytes.Buffer
	c, err := New(prodConfig(), WithSegmentClient(fake), WithLogger(slog.New(slog.NewTextHandler(&warnings, nil))))
	if err != nil {
		t.Fatal(err)
	}
	return c, fake, &warnings
}

func TestTrackSendsAValidEvent(t *testing.T) {
	c, fake, _ := newProd(t)
	ctx := WithAnonymousID(WithEnduser(context.Background(), "sub-1"), "anon-1")
	if err := c.Track(ctx, "recording.started", map[string]any{"channel_id": 4}); err != nil {
		t.Fatal(err)
	}
	if len(fake.msgs) != 1 {
		t.Fatalf("sent %d messages", len(fake.msgs))
	}
	m, ok := fake.msgs[0].(analytics.Track)
	if !ok {
		t.Fatalf("sent %T", fake.msgs[0])
	}
	if m.Event != "recording.started" || m.UserId != "sub-1" || m.AnonymousId != "anon-1" {
		t.Errorf("message = %+v", m)
	}
	if m.Properties["channel_id"] != 4 {
		t.Errorf("properties = %v", m.Properties)
	}
	if m.Context == nil || m.Context.App.Name != "tv" || m.Context.App.Version != "1.2.3" || m.Context.Library.Name != "analytics-go" {
		t.Errorf("context = %+v", m.Context)
	}
}

func TestAnonymousEventsShareOneStableID(t *testing.T) {
	prop := func(n uint8) bool {
		c, fake, _ := newProd(t)
		for i := 0; i < int(n%20)+2; i++ {
			if err := c.Track(context.Background(), "trip.created", nil); err != nil {
				t.Fatal(err)
			}
		}
		for _, m := range fake.msgs {
			tr := m.(analytics.Track)
			if tr.AnonymousId != "server:tv" || tr.UserId != "" {
				return false
			}
		}
		return true
	}
	if err := quick.Check(prop, nil); err != nil {
		t.Fatal(err)
	}
}

func TestACallerSuppliedDefaultAnonymousIDIsUsed(t *testing.T) {
	fake := &fakeSegment{}
	cfg := prodConfig()
	cfg.DefaultAnonymousID = "worker-7"
	c, err := New(cfg, WithSegmentClient(fake))
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		_ = c.Track(context.Background(), "trip.created", nil)
	}
	for _, m := range fake.msgs {
		if id := m.(analytics.Track).AnonymousId; id != "worker-7" {
			t.Fatalf("anonymous id %q", id)
		}
	}
}

func TestAKnownPersonWithoutAVisitorSendsNoAnonymousID(t *testing.T) {
	c, fake, _ := newProd(t)
	ctx := WithEnduser(context.Background(), "sub-1")
	if err := c.Track(ctx, "trip.created", nil); err != nil {
		t.Fatal(err)
	}
	if err := c.Identify(context.Background(), "sub-1", nil); err != nil {
		t.Fatal(err)
	}
	tr, id := fake.msgs[0].(analytics.Track), fake.msgs[1].(analytics.Identify)
	if tr.UserId != "sub-1" || tr.AnonymousId != "" || id.AnonymousId != "" {
		t.Fatalf("track %+v identify %+v", tr, id)
	}
}

func TestAnonymousIDPrecedenceInARequest(t *testing.T) {
	tests := []struct {
		name     string
		explicit string
		user     string
		want     string
	}{
		{"visitor", "", "", "v-1"},
		{"visitor with person", "", "sub-1", "v-1"},
		{"explicit beats visitor", "anon-x", "", "anon-x"},
		{"explicit with person", "anon-x", "sub-1", "anon-x"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, fake, _ := newProd(t)
			ctx := WithRequest(context.Background(), Request{VisitorID: "v-1"})
			if tt.explicit != "" {
				ctx = WithAnonymousID(ctx, tt.explicit)
			}
			if tt.user != "" {
				ctx = WithEnduser(ctx, tt.user)
			}
			_ = c.Track(ctx, "trip.created", nil)
			_ = c.Identify(ctx, "sub-1", nil)
			tr, id := fake.msgs[0].(analytics.Track), fake.msgs[1].(analytics.Identify)
			if tr.AnonymousId != tt.want || id.AnonymousId != tt.want {
				t.Fatalf("track %q identify %q, want %q", tr.AnonymousId, id.AnonymousId, tt.want)
			}
		})
	}
}

func TestRequestIPAndAgentGoInTheEventContext(t *testing.T) {
	prop := func(a, b, c, d byte) bool {
		ip := net.IPv4(a, b, c, d).String()
		client, fake, _ := newProd(t)
		ctx := WithRequest(context.Background(), Request{ClientIP: ip, UserAgent: browserUA})
		_ = client.Track(ctx, "trip.created", nil)
		_ = client.Identify(ctx, "sub-1", nil)
		_ = client.Track(context.Background(), "trip.created", nil)
		tr, id, bare := fake.msgs[0].(analytics.Track), fake.msgs[1].(analytics.Identify), fake.msgs[2].(analytics.Track)
		return tr.Context.IP.String() == ip && tr.Context.UserAgent == browserUA &&
			id.Context.IP.String() == ip && id.Context.UserAgent == browserUA &&
			tr.Context.App.Name == "tv" &&
			bare.Context.IP == nil && bare.Context.UserAgent == ""
	}
	if err := quick.Check(prop, nil); err != nil {
		t.Fatal(err)
	}
}

func TestInvalidEventsAreDroppedWithAWarning(t *testing.T) {
	tests := []struct {
		name  string
		event string
		props map[string]any
	}{
		{"bad name", "Trip Created", nil},
		{"bad key", "trip.created", map[string]any{"tripId": 1}},
		{"too long", "trip.created", map[string]any{"blob": strings.Repeat("x", 3000)}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, fake, warnings := newProd(t)
			err := c.Track(context.Background(), tt.event, tt.props)
			if !errors.Is(err, ErrInvalid) {
				t.Fatalf("err = %v, want ErrInvalid", err)
			}
			if len(fake.msgs) != 0 {
				t.Fatal("invalid event sent")
			}
			if !strings.Contains(warnings.String(), "level=WARN") || !strings.Contains(warnings.String(), "dropped") {
				t.Fatalf("no warning logged: %q", warnings.String())
			}
		})
	}
}

func TestNothingIsSentOutsideProduction(t *testing.T) {
	for _, env := range []string{"", "dev", "staging", "Production", "production "} {
		t.Run(env, func(t *testing.T) {
			fake := &fakeSegment{}
			cfg := prodConfig()
			cfg.Environment = env
			c, err := New(cfg, WithSegmentClient(fake))
			if err != nil {
				t.Fatal(err)
			}
			if c.Enabled() {
				t.Fatal("enabled outside production")
			}
			if err := c.Track(context.Background(), "trip.created", nil); err != nil {
				t.Fatal(err)
			}
			if err := c.Identify(context.Background(), "sub-1", nil); err != nil {
				t.Fatal(err)
			}
			if err := c.Close(); err != nil {
				t.Fatal(err)
			}
			if len(fake.msgs) != 0 || fake.closed != 0 {
				t.Fatalf("fake used outside production: %d msgs, %d closes", len(fake.msgs), fake.closed)
			}
			if err := c.Track(context.Background(), "BadName", nil); !errors.Is(err, ErrInvalid) {
				t.Fatalf("validation skipped outside production: %v", err)
			}
		})
	}
}

func TestIdentify(t *testing.T) {
	tests := []struct {
		name   string
		userID string
		traits map[string]any
		ok     bool
	}{
		{"valid", "sub-1", map[string]any{"plan": "free"}, true},
		{"no user", "", nil, false},
		{"bad trait key", "sub-1", map[string]any{"Plan": "x"}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, fake, _ := newProd(t)
			ctx := WithAnonymousID(context.Background(), "anon-1")
			err := c.Identify(ctx, tt.userID, tt.traits)
			if (err == nil) != tt.ok {
				t.Fatalf("err = %v, want ok=%v", err, tt.ok)
			}
			if !tt.ok {
				if len(fake.msgs) != 0 {
					t.Fatal("invalid identify sent")
				}
				return
			}
			m := fake.msgs[0].(analytics.Identify)
			if m.UserId != "sub-1" || m.AnonymousId != "anon-1" || m.Traits["plan"] != "free" {
				t.Fatalf("message = %+v", m)
			}
		})
	}
}

func TestCloseFlushesTheSegmentClient(t *testing.T) {
	c, fake, _ := newProd(t)
	if err := c.Close(); err != nil {
		t.Fatal(err)
	}
	if fake.closed != 1 {
		t.Fatalf("closed %d times", fake.closed)
	}
}

func TestSegmentConfig(t *testing.T) {
	tests := []struct {
		name     string
		endpoint string
		env      string
		want     string
	}{
		{"explicit", PublicEndpoint("tv.viktorbarzin.me"), "", "https://tv.viktorbarzin.me/_t/e"},
		{"from the environment", "", "https://tv.viktorbarzin.me/_t/e", "https://tv.viktorbarzin.me/_t/e"},
		{"explicit wins", "https://a.viktorbarzin.me/_t/e", "https://b.viktorbarzin.me/_t/e", "https://a.viktorbarzin.me/_t/e"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv(HostVariable, tt.env)
			cfg := prodConfig()
			cfg.Endpoint = tt.endpoint
			sc := SegmentConfig(cfg)
			if sc.Endpoint != tt.want {
				t.Fatalf("Endpoint = %q, want %q", sc.Endpoint, tt.want)
			}
			if sc.DefaultContext == nil || sc.DefaultContext.App.Name != "tv" {
				t.Fatalf("DefaultContext = %+v", sc.DefaultContext)
			}
			if sc.Logger == nil {
				t.Fatal("no logger")
			}
		})
	}
}

func TestProductionNeedsAnEndpoint(t *testing.T) {
	t.Setenv(HostVariable, "")
	if _, err := New(prodConfig()); err == nil || !strings.Contains(err.Error(), HostVariable) {
		t.Fatalf("err = %v, want one naming %s", err, HostVariable)
	}
}

func TestDevNeedsNoEndpoint(t *testing.T) {
	t.Setenv(HostVariable, "")
	cfg := prodConfig()
	cfg.Environment = "dev"
	if _, err := New(cfg); err != nil {
		t.Fatal(err)
	}
}

func TestNewBuildsARealClientInProduction(t *testing.T) {
	cfg := prodConfig()
	cfg.Endpoint = PublicEndpoint("tv.viktorbarzin.me")
	c, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if !c.Enabled() {
		t.Fatal("not enabled in production")
	}
	if err := c.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestResolveEnvironment(t *testing.T) {
	tests := []struct {
		explicit, env, want string
	}{
		{"production", "", "production"},
		{"", "production", "production"},
		{"", "", "dev"},
		{"dev", "production", "dev"},
	}
	for _, tt := range tests {
		t.Run(tt.explicit+"/"+tt.env, func(t *testing.T) {
			t.Setenv(EnvironmentVariable, tt.env)
			if got := ResolveEnvironment(tt.explicit); got != tt.want {
				t.Fatalf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestSentryOptions(t *testing.T) {
	cfg := prodConfig()
	cfg.DSN = "https://k@tv.viktorbarzin.me/_t/x/1"
	opts := SentryOptions(cfg)
	if opts.Dsn != cfg.DSN || opts.Release != "1.2.3" || opts.Environment != "production" {
		t.Fatalf("options = %+v", opts)
	}
}

func TestInitSentryOnlyInProductionWithADSN(t *testing.T) {
	tests := []struct {
		name, env, dsn string
		want           bool
	}{
		{"dev", "dev", "https://k@tv.viktorbarzin.me/_t/x/1", false},
		{"no dsn", "production", "", false},
		{"production", "production", "https://k@tv.viktorbarzin.me/_t/x/1", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := prodConfig()
			cfg.Environment, cfg.DSN = tt.env, tt.dsn
			got, err := InitSentry(cfg)
			if err != nil {
				t.Fatal(err)
			}
			if got != tt.want {
				t.Fatalf("InitSentry = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestNewRegistersTheWriteKeyForScrubbing(t *testing.T) {
	cfg := prodConfig()
	cfg.WriteKey = "write-key-0123456789abcdef"
	if _, err := New(cfg, WithSegmentClient(&fakeSegment{})); err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	slog.New(NewHandler(&buf, HandlerOptions{ServiceName: "tv"})).Info("sent " + cfg.WriteKey)
	if strings.Contains(buf.String(), cfg.WriteKey) {
		t.Fatalf("write key leaked: %s", buf.String())
	}
}
