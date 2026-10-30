package auth

import (
	"net"
	"net/http"
	"strings"
)

// ClientIP returns the best-effort client IP for rate limiting.
// When behindProxy is true, the first entry of X-Forwarded-For
// (set by the proxy) is trusted; otherwise RemoteAddr is used.
func ClientIP(r *http.Request, behindProxy bool) string {
	if behindProxy {
		if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
			first := strings.TrimSpace(strings.Split(xff, ",")[0])
			if first != "" {
				return first
			}
		}
	}

	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}

	return host
}
