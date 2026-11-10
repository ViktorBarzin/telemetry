package telemetry

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/quick"
)

const browserUA = "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 Chrome/130.0 Safari/537.36"

func newRequest(headers map[string]string, remote string) *http.Request {
	r := httptest.NewRequest(http.MethodGet, "/api/x", nil)
	r.RemoteAddr = remote
	for k, v := range headers {
		r.Header.Set(k, v)
	}
	return r
}

func TestRequestFromClientIP(t *testing.T) {
	tests := []struct {
		name    string
		headers map[string]string
		remote  string
		want    string
	}{
		{"x-real-ip wins", map[string]string{"X-Real-Ip": "81.2.69.142", "X-Forwarded-For": "1.1.1.1"}, "10.0.0.1:80", "81.2.69.142"},
		{"first forwarded", map[string]string{"X-Forwarded-For": "81.2.69.142, 10.0.0.2"}, "10.0.0.1:80", "81.2.69.142"},
		{"forwarded v6", map[string]string{"X-Forwarded-For": " 2001:db8::1 ,10.0.0.2"}, "", "2001:db8::1"},
		{"peer", nil, "10.0.0.1:4321", "10.0.0.1"},
		{"peer v6", nil, "[2001:db8::2]:443", "2001:db8::2"},
		{"bad real ip skipped", map[string]string{"X-Real-Ip": "nope", "X-Forwarded-For": "81.2.69.142"}, "", "81.2.69.142"},
		{"all junk", map[string]string{"X-Real-Ip": "", "X-Forwarded-For": "junk"}, "pipe", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := RequestFrom(newRequest(tt.headers, tt.remote)).ClientIP; got != tt.want {
				t.Fatalf("ClientIP = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestXRealIPAlwaysWins(t *testing.T) {
	prop := func(real, fwd [4]byte, peer [4]byte) bool {
		ip := net.IP(real[:]).String()
		r := newRequest(map[string]string{"X-Real-Ip": ip, "X-Forwarded-For": net.IP(fwd[:]).String()}, net.IP(peer[:]).String()+":1")
		return RequestFrom(r).ClientIP == ip
	}
	if err := quick.Check(prop, nil); err != nil {
		t.Fatal(err)
	}
}

func TestClientIPIsAnIPOrEmpty(t *testing.T) {
	prop := func(value string) bool {
		got := RequestFrom(newRequest(map[string]string{"X-Real-Ip": value, "X-Forwarded-For": value}, value)).ClientIP
		return got == "" || net.ParseIP(got) != nil
	}
	if err := quick.Check(prop, nil); err != nil {
		t.Fatal(err)
	}
}

func TestRequestFromUserAgent(t *testing.T) {
	tests := []struct{ agent, want string }{
		{browserUA, browserUA},
		{"analytics-python/2.4.0", ""},
		{"analytics-node-next/1.3.0", ""},
		{"analytics-go (version: 3.4.0)", ""},
		{"", ""},
		{strings.Repeat("M", 2000), strings.Repeat("M", 512)},
	}
	for _, tt := range tests {
		if got := RequestFrom(newRequest(map[string]string{"User-Agent": tt.agent}, "")).UserAgent; got != tt.want {
			t.Errorf("UserAgent(%.30q) = %.30q, want %.30q", tt.agent, got, tt.want)
		}
	}
}

func TestRequestFromVisitorID(t *testing.T) {
	tests := []struct{ value, want string }{
		{"2a1c7b9e-6a51-4f4a-9a0e-9e0b7c1c2d3f", "2a1c7b9e-6a51-4f4a-9a0e-9e0b7c1c2d3f"},
		{"lib-v020-check", "lib-v020-check"},
		{"  lib-v020-check ", "lib-v020-check"},
		{"", ""},
		{"has space", ""},
		{"<script>", ""},
		{strings.Repeat("x", 129), ""},
	}
	for _, tt := range tests {
		if got := RequestFrom(newRequest(map[string]string{VisitorHeader: tt.value}, "")).VisitorID; got != tt.want {
			t.Errorf("VisitorID(%q) = %q, want %q", tt.value, got, tt.want)
		}
	}
}

func TestRequestMiddleware(t *testing.T) {
	tests := []struct {
		name     string
		resolve  func(*http.Request) string
		wantUser string
	}{
		{"no resolver", nil, ""},
		{"resolver", func(r *http.Request) string { return r.Header.Get("X-User") }, "sub-1"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var gotReq Request
			var gotUser string
			h := RequestMiddleware(tt.resolve)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				gotReq, gotUser = CurrentRequest(r.Context()), Enduser(r.Context())
			}))
			r := newRequest(map[string]string{
				"X-Visitor-Id":    "v-1",
				"X-Forwarded-For": "81.2.69.142, 10.0.0.9",
				"User-Agent":      browserUA,
				"X-User":          "sub-1",
			}, "10.0.0.5:4321")
			h.ServeHTTP(httptest.NewRecorder(), r)
			want := Request{VisitorID: "v-1", ClientIP: "81.2.69.142", UserAgent: browserUA}
			if gotReq != want || gotUser != tt.wantUser {
				t.Fatalf("request %+v user %q", gotReq, gotUser)
			}
		})
	}
}

func TestCurrentRequestIsEmptyOutsideARequest(t *testing.T) {
	if got := CurrentRequest(context.Background()); got != (Request{}) {
		t.Fatalf("got %+v", got)
	}
}
