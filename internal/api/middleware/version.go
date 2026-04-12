package middleware

import "net/http"

// APIVersion adds API version headers to all responses.
func APIVersion(version string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("X-DeployMonster-Version", version)
			w.Header().Set("X-API-Version", "v1")
			next.ServeHTTP(w, r)
		})
	}
}
