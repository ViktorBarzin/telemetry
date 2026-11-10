package telemetry

import (
	"context"
	"net"
	"net/http"
	"regexp"
	"strings"
)

// VisitorHeader is the header a browser sets on same-origin API calls to its
// Rybbit visitor id (localStorage "rybbit-visitor-id").
const VisitorHeader = "X-Visitor-Id"

// MaxUserAgentLength is how much of a User-Agent is kept.
const MaxUserAgentLength = 512

var (
	visitorIDPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:-]{0,127}$`)
	// Segment SDK agents. Rybbit flags them as bots, so they never stand in
	// for a person's agent.
	sdkAgentPattern = regexp.MustCompile(`(?i)^analytics[-_]`)
)

// Request is who sent the request being handled. Events sent with a ctx
// carrying it default their anonymousId to VisitorID and carry ClientIP and
// UserAgent as context.ip and context.userAgent.
type Request struct {
	VisitorID string
	ClientIP  string
	UserAgent string
}

// RequestFrom reads the visitor id, client IP and user agent from r. The IP
// is X-Real-Ip (set by Traefik's real-ip middleware), else the first
// X-Forwarded-For entry, else the peer address; values that are not IP
// addresses are skipped. A Segment SDK user agent counts as none.
func RequestFrom(r *http.Request) Request {
	visitor := strings.TrimSpace(r.Header.Get(VisitorHeader))
	if !visitorIDPattern.MatchString(visitor) {
		visitor = ""
	}
	forwarded, _, _ := strings.Cut(r.Header.Get("X-Forwarded-For"), ",")
	peer := r.RemoteAddr
	if host, _, err := net.SplitHostPort(peer); err == nil {
		peer = host
	}
	ip := firstIP(r.Header.Get("X-Real-Ip"), forwarded, peer)
	agent := strings.TrimSpace(r.UserAgent())
	if sdkAgentPattern.MatchString(agent) {
		agent = ""
	}
	if len(agent) > MaxUserAgentLength {
		agent = agent[:MaxUserAgentLength]
	}
	return Request{VisitorID: visitor, ClientIP: ip, UserAgent: agent}
}

func firstIP(values ...string) string {
	for _, v := range values {
		if ip := net.ParseIP(strings.TrimSpace(v)); ip != nil {
			return ip.String()
		}
	}
	return ""
}

// WithRequest stores req in ctx, for code that does not use RequestMiddleware.
func WithRequest(ctx context.Context, req Request) context.Context {
	return context.WithValue(ctx, requestKey, req)
}

// CurrentRequest returns the Request stored in ctx, or the zero Request.
func CurrentRequest(ctx context.Context) Request {
	r, _ := ctx.Value(requestKey).(Request)
	return r
}

// RequestMiddleware stores each request's sender (RequestFrom) in its
// context, and with a non-nil resolveUser also the Person's Authentik sub
// (WithEnduser) when resolveUser returns one.
func RequestMiddleware(resolveUser func(*http.Request) string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := WithRequest(r.Context(), RequestFrom(r))
			if resolveUser != nil {
				if sub := resolveUser(r); sub != "" {
					ctx = WithEnduser(ctx, sub)
				}
			}
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}
