package middleware

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/deploy-monster/deploy-monster/internal/auth"
	"github.com/deploy-monster/deploy-monster/internal/core"
)

// errorCodeMap maps HTTP status codes to machine-readable error codes.
var errorCodeMap = map[int]string{
	http.StatusBadRequest:          "bad_request",
	http.StatusUnauthorized:        "unauthorized",
	http.StatusForbidden:           "forbidden",
	http.StatusNotFound:            "not_found",
	http.StatusConflict:            "conflict",
	http.StatusTooManyRequests:     "rate_limited",
	http.StatusInternalServerError: "internal_error",
	http.StatusServiceUnavailable:  "unavailable",
}

// writeErrorJSON writes a structured JSON error response matching the handler error format.
func writeErrorJSON(w http.ResponseWriter, status int, message string) {
	code := errorCodeMap[status]
	if code == "" {
		code = "error"
	}
	resp := map[string]any{
		"success": false,
		"error": map[string]string{
			"code":    code,
			"message": message,
		},
	}
	if rid := w.Header().Get("X-Request-ID"); rid != "" {
		resp["request_id"] = rid
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(resp)
}

// Chain applies middlewares in order (last applied runs first).
func Chain(handler http.Handler, middlewares ...func(http.Handler) http.Handler) http.Handler {
	for i := len(middlewares) - 1; i >= 0; i-- {
		handler = middlewares[i](handler)
	}
	return handler
}

// Recovery returns middleware that recovers from panics and logs the error.
func Recovery(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				if rec := recover(); rec != nil {
					logger.Error("panic recovered",
						"error", fmt.Sprintf("%v", rec),

						"path", r.URL.Path,
					)
					writeErrorJSON(w, http.StatusInternalServerError, "internal server error")
				}
			}()
			next.ServeHTTP(w, r)
		})
	}
}

// RequestLogger returns middleware that logs each request.
func RequestLogger(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			sw := &statusWriter{ResponseWriter: w, status: http.StatusOK}
			next.ServeHTTP(sw, r)

			elapsed := time.Since(start)
			attrs := []any{
				"method", r.Method,
				"path", r.URL.Path,
				"status", sw.status,
				"bytes", sw.bytesWritten,
				"duration", elapsed.String(),
				"ip", realIP(r),
			}
			if traceID := GetTraceID(r.Context()); traceID != "" {
				attrs = append(attrs, "trace_id", traceID)
			}
			if reqID := GetRequestID(r.Context()); reqID != "" {
				attrs = append(attrs, "request_id", reqID)
			}
			// Warn on slow requests (>5s) so ops can spot bottlenecks
			if elapsed >= 5*time.Second {
				logger.Warn("slow request", attrs...)
			} else {
				logger.Info("request", attrs...)
			}
		})
	}
}

// CORS returns middleware that sets CORS headers.
//
// Two modes, chosen by allowedOrigins:
//
//  1. Public mode (allowedOrigins == "" or "*"): emit
//     Access-Control-Allow-Origin: * and omit Allow-Credentials. This
//     matches the fetch-spec ban on wildcard+credentials — credentialed
//     cross-origin fetches are rejected by browsers in this mode.
//
//  2. Allowlist mode (comma-separated origin list): echo the request
//     Origin only if it is in the list, and emit Allow-Credentials:
//     true. Origins not in the list receive no Allow-Origin header,
//     which the browser interprets as CORS denial.
//
// Vary: Origin is always set so caches can key on Origin. The previous
// "always wildcard" behavior (introduced in commit a72550d) was unsafe
// for any deployment that configured cors_origins — this restores the
// intended allowlist semantics while keeping the wildcard fallback for
// self-hosted defaults.
func CORS(allowedOrigins string, enforceHTTPS bool) func(http.Handler) http.Handler {
	// Parse the allowlist once per middleware instance.
	origins := parseCORSOrigins(allowedOrigins)
	publicMode := len(origins) == 0 || (len(origins) == 1 && origins[0] == "*")

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Vary", "Origin")
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-API-Key, X-Request-ID")
			w.Header().Set("Access-Control-Expose-Headers", "X-Request-ID, X-DeployMonster-Version, X-API-Version")
			w.Header().Set("Access-Control-Max-Age", "86400")

			origin := r.Header.Get("Origin")
			if publicMode {
				// Wildcard. Never combined with Allow-Credentials — browsers
				// reject credentialed fetches against a wildcard origin.
				w.Header().Set("Access-Control-Allow-Origin", "*")
			} else if origin != "" && originAllowed(origin, origins) {
				// Strict allowlist. Safe to set Allow-Credentials because
				// we echo a specific origin, not "*".
				w.Header().Set("Access-Control-Allow-Origin", origin)
				w.Header().Set("Access-Control-Allow-Credentials", "true")
			}
			// else: no Allow-Origin header → browser denies the response.

			if r.Method == http.MethodOptions {
				w.WriteHeader(http.StatusNoContent)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// parseCORSOrigins splits the config value into trimmed, non-empty origins.
func parseCORSOrigins(s string) []string {
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if v := strings.TrimSpace(p); v != "" {
			out = append(out, v)
		}
	}
	return out
}

// originAllowed reports whether origin appears in the allowlist.
func originAllowed(origin string, allowlist []string) bool {
	for _, a := range allowlist {
		if a == origin {
			return true
		}
	}
	return false
}

// RequireAuth returns middleware that validates JWT from the Authorization header,
// dm_access cookie, or X-API-Key header (in that priority order).
func RequireAuth(jwtSvc *auth.JWTService, bolt core.BoltStorer, store core.Store) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Try JWT from Authorization header
			header := r.Header.Get("Authorization")
			if strings.HasPrefix(header, "Bearer ") {
				tokenStr := strings.TrimPrefix(header, "Bearer ")
				claims, err := jwtSvc.ValidateAccessToken(tokenStr)
				if err != nil {
					writeErrorJSON(w, http.StatusUnauthorized, "invalid token")
					return
				}
				// SECURITY FIX: Check if access token is revoked
				if jwtSvc.IsAccessTokenRevoked(bolt, claims.ID) {
					writeErrorJSON(w, http.StatusUnauthorized, "token has been revoked")
					return
				}
				ctx := auth.ContextWithClaims(r.Context(), claims)
				next.ServeHTTP(w, r.WithContext(ctx))
				return
			}

			// Try JWT from httpOnly cookie
			if c, err := r.Cookie("dm_access"); err == nil && c.Value != "" {
				claims, err := jwtSvc.ValidateAccessToken(c.Value)
				if err != nil {
					writeErrorJSON(w, http.StatusUnauthorized, "invalid token")
					return
				}
				// SECURITY FIX: Check if access token is revoked
				if jwtSvc.IsAccessTokenRevoked(bolt, claims.ID) {
					writeErrorJSON(w, http.StatusUnauthorized, "token has been revoked")
					return
				}
				ctx := auth.ContextWithClaims(r.Context(), claims)
				next.ServeHTTP(w, r.WithContext(ctx))
				return
			}

			// Try API key from X-API-Key header
			apiKey := r.Header.Get("X-API-Key")
			if apiKey != "" {
				// Validate API key format
				if !strings.HasPrefix(apiKey, "dm_") {
					writeErrorJSON(w, http.StatusUnauthorized, "invalid api key format")
					return
				}

				// Extract the stable prefix for lookup.
				if len(apiKey) < auth.APIKeyPrefixLength {
					writeErrorJSON(w, http.StatusUnauthorized, "invalid api key")
					return
				}

				// Check if bolt store is available for API key lookup
				if bolt == nil {
					writeErrorJSON(w, http.StatusUnauthorized, "api key authentication not available")
					return
				}

				keyPrefix := apiKey[:auth.APIKeyPrefixLength]

				// Lookup API key by prefix using BoltStorer
				storedKey, err := bolt.GetAPIKeyByPrefix(r.Context(), keyPrefix)
				if err != nil {
					writeErrorJSON(w, http.StatusUnauthorized, "invalid api key")
					return
				}

				// Verify: compare the incoming plaintext key against the stored bcrypt hash.
				// SECURITY FIX (CRYPTO-001): Use bcrypt VerifyAPIKey instead of SHA-256 + ConstantTimeCompare
				// bcrypt's adaptive cost factor provides protection against rainbow table attacks.
				if !auth.VerifyAPIKey(apiKey, storedKey.KeyHash) {
					writeErrorJSON(w, http.StatusUnauthorized, "invalid api key")
					return
				}

				// Check if key is expired
				if storedKey.ExpiresAt != nil && time.Now().After(*storedKey.ExpiresAt) {
					writeErrorJSON(w, http.StatusUnauthorized, "api key expired")
					return
				}

				// Create claims from the API key's associated user
				// SECURITY FIX: Look up RoleID from user membership so RBAC works with API keys
				claims := &auth.Claims{
					UserID:   storedKey.UserID,
					TenantID: storedKey.TenantID,
				}
				if store != nil {
					if membership, err := store.GetUserMembership(r.Context(), storedKey.UserID); err == nil && membership != nil {
						claims.RoleID = membership.RoleID
						if claims.TenantID == "" {
							claims.TenantID = membership.TenantID
						}
					}
				}

				ctx := auth.ContextWithClaims(r.Context(), claims)
				next.ServeHTTP(w, r.WithContext(ctx))
				return
			}

			writeErrorJSON(w, http.StatusUnauthorized, "missing authorization — use Bearer token or X-API-Key")
		})
	}
}

// statusWriter wraps ResponseWriter to capture the status code and bytes written.
type statusWriter struct {
	http.ResponseWriter
	status       int
	bytesWritten int
}

func (sw *statusWriter) WriteHeader(code int) {
	sw.status = code
	sw.ResponseWriter.WriteHeader(code)
}

func (sw *statusWriter) Write(b []byte) (int, error) {
	n, err := sw.ResponseWriter.Write(b)
	sw.bytesWritten += n
	return n, err
}

func realIP(r *http.Request) string {
	if ip := r.Header.Get("X-Real-IP"); ip != "" {
		return ip
	}
	if ip := r.Header.Get("X-Forwarded-For"); ip != "" {
		return ip
	}
	return r.RemoteAddr
}
