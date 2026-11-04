// Package httpmiddleware holds the HTTP middleware that is specific to Snorlx: client IP resolution
// that only trusts configured proxies, and a request logger that never records query strings.
package httpmiddleware

import (
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5/middleware"
	"github.com/rs/zerolog/log"
)

// TrustedRealIP rewrites r.RemoteAddr from X-Real-IP or X-Forwarded-For only when the direct peer
// is inside one of the trusted networks. Without this check any client can spoof its address and
// bypass the per-IP rate limits. With no trusted networks the peer address is always used.
func TrustedRealIP(trusted []*net.IPNet) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if len(trusted) > 0 && peerIsTrusted(r.RemoteAddr, trusted) {
				if ip := forwardedClientIP(r, trusted); ip != "" {
					r.RemoteAddr = ip
				}
			}
			next.ServeHTTP(w, r)
		})
	}
}

// StripBasePath removes the base path (such as /snorlx) from request paths, so routes are
// registered without it. Requests without it, like pod probes, pass through unchanged.
func StripBasePath(base string, next http.Handler) http.Handler {
	if base == "" {
		return next
	}
	stripped := http.StripPrefix(base, next)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == base || strings.HasPrefix(r.URL.Path, base+"/") {
			stripped.ServeHTTP(w, r)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func peerIsTrusted(remoteAddr string, trusted []*net.IPNet) bool {
	host, _, err := net.SplitHostPort(remoteAddr)
	if err != nil {
		host = remoteAddr
	}
	ip := net.ParseIP(host)
	if ip == nil {
		return false
	}
	return ipInNetworks(ip, trusted)
}

func ipInNetworks(ip net.IP, networks []*net.IPNet) bool {
	for _, n := range networks {
		if n.Contains(ip) {
			return true
		}
	}
	return false
}

// forwardedClientIP returns the client IP announced by a trusted proxy. X-Real-IP wins when set;
// otherwise the rightmost X-Forwarded-For entry that is not itself a trusted proxy is used, so a
// value injected by the client before the proxy chain cannot override it.
func forwardedClientIP(r *http.Request, trusted []*net.IPNet) string {
	if real := strings.TrimSpace(r.Header.Get("X-Real-IP")); real != "" {
		if ip := net.ParseIP(real); ip != nil {
			return ip.String()
		}
	}
	forwarded := r.Header.Get("X-Forwarded-For")
	if forwarded == "" {
		return ""
	}
	parts := strings.Split(forwarded, ",")
	for i := len(parts) - 1; i >= 0; i-- {
		ip := net.ParseIP(strings.TrimSpace(parts[i]))
		if ip == nil {
			return ""
		}
		if !ipInNetworks(ip, trusted) {
			return ip.String()
		}
	}
	return ""
}

// RequestLogger logs one structured line per request without the query string, so OAuth codes,
// state values and search terms never reach the logs.
func RequestLogger(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)

		next.ServeHTTP(ww, r)

		log.Info().
			Str("request_id", middleware.GetReqID(r.Context())).
			Str("method", r.Method).
			Str("path", r.URL.Path).
			Str("remote_ip", clientIP(r.RemoteAddr)).
			Int("status", ww.Status()).
			Int("bytes", ww.BytesWritten()).
			Dur("duration", time.Since(start)).
			Msg("request")
	})
}

func clientIP(remoteAddr string) string {
	host, _, err := net.SplitHostPort(remoteAddr)
	if err != nil {
		return remoteAddr
	}
	return host
}
