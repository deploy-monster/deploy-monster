package middleware

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"net/http"
)

const (
	csrfCookieName    = "__Host-dm_csrf" // __Host- prefix enforced by browsers
	csrfHeaderName    = "X-CSRF-Token"
	accessCookieName  = "dm_access"
	refreshCookieName = "dm_refresh"
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

		// Check if there's a CSRF cookie. If token cookies are present, this is
		// a cookie-authenticated browser request and must not bypass CSRF just
		// because the CSRF cookie is missing or expired.
		cookie, err := r.Cookie(csrfCookieName)
		if err != nil || cookie.Value == "" {
			if hasCookie(r, accessCookieName) || hasCookie(r, refreshCookieName) {
				writeErrorJSON(w, http.StatusForbidden, "CSRF token required")
				return
			}
			// No CSRF or token cookies — could be first login/register or a
			// non-browser client. Protected endpoints will still be rejected by
			// RequireAuth if no bearer/API key auth is supplied.
			next.ServeHTTP(w, r)
			return
		}

		// Validate: X-CSRF-Token header must match the cookie value.
		// Use a constant-time compare so the match doesn't leak timing on a
		// security token (lengths are pre-checked to keep the compare safe).
		headerToken := r.Header.Get(csrfHeaderName)
		if headerToken == "" || len(headerToken) != len(cookie.Value) ||
			subtle.ConstantTimeCompare([]byte(headerToken), []byte(cookie.Value)) != 1 {
			writeErrorJSON(w, http.StatusForbidden, "CSRF token mismatch")
			return
		}

		next.ServeHTTP(w, r)
	})
}

func hasCookie(r *http.Request, name string) bool {
	cookie, err := r.Cookie(name)
	return err == nil && cookie.Value != ""
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
	if _, err := rand.Read(b); err != nil {
		panic("crypto/rand unavailable: " + err.Error())
	}
	return hex.EncodeToString(b)
}
