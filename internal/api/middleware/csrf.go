package middleware

import (
	"crypto/rand"
	"encoding/hex"
	"net/http"
)

const (
	csrfCookieName = "dm_csrf"
	csrfHeaderName = "X-CSRF-Token"
)

// CSRFProtect implements double-submit cookie CSRF protection.
// It only enforces CSRF checks on requests authenticated via cookies (not
// Bearer tokens or API keys, which are not auto-sent by the browser).
func CSRFProtect(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Safe methods are exempt
		if r.Method == http.MethodGet || r.Method == http.MethodHead || r.Method == http.MethodOptions {
			next.ServeHTTP(w, r)
			return
		}

		// Only enforce CSRF for cookie-authenticated requests.
		// If the request has an Authorization header or X-API-Key, it's
		// programmatic and not vulnerable to CSRF.
		if r.Header.Get("Authorization") != "" || r.Header.Get("X-API-Key") != "" {
			next.ServeHTTP(w, r)
			return
		}

		// Check if there's a cookie (meaning browser-based request)
		cookie, err := r.Cookie(csrfCookieName)
		if err != nil || cookie.Value == "" {
			// No CSRF cookie — could be first request or non-browser client
			// For cookie-auth endpoints that mutate (login, register), we don't
			// enforce CSRF since they set the cookie. For protected endpoints,
			// RequireAuth will reject if there's no auth at all.
			next.ServeHTTP(w, r)
			return
		}

		// Validate: X-CSRF-Token header must match the cookie value
		headerToken := r.Header.Get(csrfHeaderName)
		if headerToken == "" || headerToken != cookie.Value {
			writeErrorJSON(w, http.StatusForbidden, "CSRF token mismatch")
			return
		}

		next.ServeHTTP(w, r)
	})
}

// SetCSRFCookie sets the CSRF double-submit cookie. Call this after successful
// authentication (login, register, refresh) so the frontend can read it.
// The Secure flag is gated on the request transport so the cookie is actually
// stored by Chromium when the server listens on plain HTTP (dev / CI E2E).
func SetCSRFCookie(w http.ResponseWriter, r *http.Request) {
	token := generateCSRFToken()
	secure := r != nil && (r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https")
	http.SetCookie(w, &http.Cookie{
		Name:     csrfCookieName,
		Value:    token,
		Path:     "/",
		MaxAge:   86400, // 24 hours
		HttpOnly: false, // JS must read this to send as header
		Secure:   secure,
		SameSite: http.SameSiteLaxMode,
	})
}

func generateCSRFToken() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
