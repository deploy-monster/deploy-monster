package middleware

import (
	"log/slog"
	"net"
	"net/http"
	"strings"
)

// IPAllowlist enforces an allowlist of CIDR ranges. Requests from IPs
// outside the configured ranges receive a 403 Forbidden response
// before they reach authentication or business logic.
//
// When AllowedCIDRs is nil or empty, all requests pass through — this
// preserves backward compatibility for deployments that do not need
// IP-based access control.
//
// Security properties:
//   - Only RemoteAddr is used for source IP detection. XFF headers are
//     NOT respected (attackers can forge them), matching the approach
//     used by GlobalRateLimiter.
//   - The check happens before GlobalRateLimiter so blocked IPs do not
//     consume rate limit tokens.
type IPAllowlist struct {
	allowedCIDRs []*net.IPNet
	logger      *slog.Logger
}

// NewIPAllowlist creates an allowlist from a list of CIDR notation
// strings (e.g. "10.0.0.0/8", "192.168.0.0/16"). Nil or empty input
// means allow all (backward-compatible default). Invalid CIDRs are
// logged and skipped — only correctly parsed ranges are enforced.
func NewIPAllowlist(cidrs []string, logger *slog.Logger) *IPAllowlist {
	if logger == nil {
		logger = slog.Default()
	}
	al := &IPAllowlist{logger: logger}
	if len(cidrs) == 0 {
		return al
	}
	for _, cidr := range cidrs {
		cidr = strings.TrimSpace(cidr)
		if cidr == "" {
			continue
		}
		_, ipNet, err := net.ParseCIDR(cidr)
		if err != nil {
			logger.Warn("skipping invalid CIDR in ip_allowlist config", "cidr", cidr, "error", err)
			continue
		}
		al.allowedCIDRs = append(al.allowedCIDRs, ipNet)
	}
	return al
}

// Middleware returns an HTTP handler that blocks non-allowed IPs.
func (al *IPAllowlist) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if len(al.allowedCIDRs) == 0 {
			next.ServeHTTP(w, r)
			return
		}
		ip := safeClientIP(r, false)
		clientIP := net.ParseIP(ip)
		if clientIP == nil {
			al.logger.Warn("could not parse client IP, blocking", "ip", ip)
			writeErrorJSON(w, http.StatusForbidden, "access denied")
			return
		}
		for _, cidr := range al.allowedCIDRs {
			if cidr.Contains(clientIP) {
				next.ServeHTTP(w, r)
				return
			}
		}
		al.logger.Info("request blocked by IP allowlist", "ip", ip)
		writeErrorJSON(w, http.StatusForbidden, "access denied")
	})
}
