package httpmiddleware

import (
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
)

func mustCIDRs(t *testing.T, cidrs ...string) []*net.IPNet {
	t.Helper()
	var out []*net.IPNet
	for _, c := range cidrs {
		_, n, err := net.ParseCIDR(c)
		if err != nil {
			t.Fatalf("bad cidr %q: %v", c, err)
		}
		out = append(out, n)
	}
	return out
}

func resolvedRemoteAddr(t *testing.T, trusted []*net.IPNet, remoteAddr string, headers map[string]string) string {
	t.Helper()
	var seen string
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = r.RemoteAddr
	})
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = remoteAddr
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	TrustedRealIP(trusted)(next).ServeHTTP(httptest.NewRecorder(), req)
	return seen
}

func TestTrustedRealIP_NoTrustedProxies_IgnoresHeaders(t *testing.T) {
	got := resolvedRemoteAddr(t, nil, "203.0.113.5:1234", map[string]string{
		"X-Forwarded-For": "10.0.0.1",
		"X-Real-IP":       "10.0.0.2",
	})
	if got != "203.0.113.5:1234" {
		t.Fatalf("RemoteAddr = %q, want untouched peer address", got)
	}
}

func TestTrustedRealIP_UntrustedPeer_IgnoresHeaders(t *testing.T) {
	trusted := mustCIDRs(t, "10.0.0.0/8")
	got := resolvedRemoteAddr(t, trusted, "203.0.113.5:1234", map[string]string{
		"X-Forwarded-For": "198.51.100.7",
	})
	if got != "203.0.113.5:1234" {
		t.Fatalf("RemoteAddr = %q, want untouched peer address", got)
	}
}

func TestTrustedRealIP_TrustedPeer_UsesXRealIP(t *testing.T) {
	trusted := mustCIDRs(t, "10.0.0.0/8")
	got := resolvedRemoteAddr(t, trusted, "10.1.2.3:4321", map[string]string{
		"X-Real-IP":       "198.51.100.7",
		"X-Forwarded-For": "192.0.2.9",
	})
	if got != "198.51.100.7" {
		t.Fatalf("RemoteAddr = %q, want X-Real-IP value", got)
	}
}

func TestTrustedRealIP_TrustedPeer_UsesRightmostUntrustedForwardedFor(t *testing.T) {
	trusted := mustCIDRs(t, "10.0.0.0/8")
	// Client spoofed 1.1.1.1, real client 198.51.100.7, then two trusted proxies appended.
	got := resolvedRemoteAddr(t, trusted, "10.1.2.3:4321", map[string]string{
		"X-Forwarded-For": "1.1.1.1, 198.51.100.7, 10.0.0.9, 10.0.0.10",
	})
	if got != "198.51.100.7" {
		t.Fatalf("RemoteAddr = %q, want rightmost untrusted hop", got)
	}
}

func TestTrustedRealIP_MalformedForwardedFor_KeepsPeer(t *testing.T) {
	trusted := mustCIDRs(t, "10.0.0.0/8")
	got := resolvedRemoteAddr(t, trusted, "10.1.2.3:4321", map[string]string{
		"X-Forwarded-For": "not-an-ip",
	})
	if got != "10.1.2.3:4321" {
		t.Fatalf("RemoteAddr = %q, want untouched peer address", got)
	}
}

func TestRequestLogger_PassesThrough(t *testing.T) {
	called := false
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusTeapot)
	})
	req := httptest.NewRequest(http.MethodGet, "/api/auth/callback?code=secret&state=abc", nil)
	rec := httptest.NewRecorder()
	RequestLogger(next).ServeHTTP(rec, req)
	if !called || rec.Code != http.StatusTeapot {
		t.Fatalf("logger must pass the request through; called=%v status=%d", called, rec.Code)
	}
}

func TestStripBasePath(t *testing.T) {
	tests := []struct{ name, base, target, want string }{
		{"strips the prefix", "/snorlx", "/snorlx/api/runs", "/api/runs"},
		{"unprefixed path passes through", "/snorlx", "/health", "/health"},
		{"similar prefix is not stripped", "/snorlx", "/snorlxy/api", "/snorlxy/api"},
		{"nested base path", "/tools/snorlx", "/tools/snorlx/ws", "/ws"},
		{"empty base path is a no-op", "", "/snorlx/api", "/snorlx/api"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got string
			next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { got = r.URL.Path })
			req := httptest.NewRequest(http.MethodGet, tt.target, nil)
			StripBasePath(tt.base, next).ServeHTTP(httptest.NewRecorder(), req)
			if got != tt.want {
				t.Errorf("path = %q, want %q", got, tt.want)
			}
		})
	}
}
