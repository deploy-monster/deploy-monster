package middleware

import "net/http"

// SecurityHeaders adds common security headers to responses.
func SecurityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("X-XSS-Protection", "1; mode=block")
		w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
		next.ServeHTTP(w, r)
	})
}

// CustomHeaders applies custom header modifications.
type CustomHeaders struct {
	Add    map[string]string
	Remove []string
}

// Middleware returns an HTTP middleware that applies custom header modifications.
func (ch *CustomHeaders) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		for k, v := range ch.Add {
			w.Header().Set(k, v)
		}
		for _, k := range ch.Remove {
			w.Header().Del(k)
		}
		next.ServeHTTP(w, r)
	})
}
