package middleware

import (
	"net"
	"net/http"
	"strings"
)

// TrustMode controls which proxy headers are trusted for client IP extraction.
type TrustMode string

const (
	TrustNone       TrustMode = "none"
	TrustAll        TrustMode = "all"
	TrustCloudflare TrustMode = "cloudflare"
)

// ParseTrustMode converts a string to a TrustMode, defaulting to TrustNone
// for unrecognised values.
func ParseTrustMode(s string) TrustMode {
	switch strings.ToLower(s) {
	case "all":
		return TrustAll
	case "cloudflare":
		return TrustCloudflare
	default:
		return TrustNone
	}
}

// ClientIP extracts the real client IP from the request, respecting the
// configured trust mode. The extraction priority (when trusted) is:
//
//  1. CF-Connecting-IP  (Cloudflare — most reliable)
//  2. X-Real-IP         (reverse proxy standard)
//  3. X-Forwarded-For   (first IP in chain)
//  4. RemoteAddr        (direct connection fallback)
func ClientIP(r *http.Request, mode TrustMode) string {
	if mode != TrustNone && shouldTrustHeaders(r, mode) {
		if ip := parseValidIP(r.Header.Get("CF-Connecting-IP")); ip != "" {
			return ip
		}
		if ip := parseValidIP(r.Header.Get("X-Real-IP")); ip != "" {
			return ip
		}
		if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
			if ip := parseValidIP(strings.SplitN(xff, ",", 2)[0]); ip != "" {
				return ip
			}
		}
	}
	return remoteIP(r)
}

// parseValidIP trims whitespace and validates that s is a valid IP address.
// Returns the canonical IP string, or empty string if invalid.
func parseValidIP(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	ip := net.ParseIP(s)
	if ip == nil {
		return ""
	}
	return ip.String()
}

// shouldTrustHeaders decides whether proxy headers can be trusted based on
// the trust mode and the connecting address.
//
// TrustCloudflare assumes Cloudflare Tunnel (cloudflared) is in use, where
// the tunnel connects from loopback. Headers are only trusted when RemoteAddr
// is a loopback address to prevent spoofing from direct connections.
func shouldTrustHeaders(r *http.Request, mode TrustMode) bool {
	switch mode {
	case TrustAll:
		return true
	case TrustCloudflare:
		ip := net.ParseIP(remoteIP(r))
		return ip != nil && ip.IsLoopback()
	default:
		return false
	}
}

// remoteIP extracts the IP portion of RemoteAddr (strips the port).
func remoteIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}
