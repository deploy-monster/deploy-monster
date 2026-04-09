package middleware

import (
	"crypto/subtle"
	"encoding/json"
	"log/slog"
	"net/http"
	"runtime/debug"
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
	json.NewEncoder(w).Encode(resp)
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
						"error", rec,
						"stack", string(debug.Stack()),
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

			attrs := []any{
				"method", r.Method,
				"path", r.URL.Path,
				"status", sw.status,
				"duration", time.Since(start).String(),
				"ip", realIP(r),
			}
			if traceID := GetTraceID(r.Context()); traceID != "" {
				attrs = append(attrs, "trace_id", traceID)
			}
			if reqID := GetRequestID(r.Context()); reqID != "" {
				attrs = append(attrs, "request_id", reqID)
			}
			logger.Info("request", attrs...)
		})
	}
}

// CORS returns middleware that sets CORS headers.
// Pass "*" to allow all origins, or comma-separated list for specific origins.
func CORS(allowedOrigins string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			origin := r.Header.Get("Origin")

			if allowedOrigins == "*" {
				w.Header().Set("Access-Control-Allow-Origin", "*")
			} else if origin != "" {
				// Check if origin is in allowed list
				for _, allowed := range strings.Split(allowedOrigins, ",") {
					if strings.TrimSpace(allowed) == origin {
						w.Header().Set("Access-Control-Allow-Origin", origin)
						w.Header().Set("Vary", "Origin")
						break
					}
				}
			}

			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-API-Key, X-Request-ID, X-CSRF-Token")
			w.Header().Set("Access-Control-Expose-Headers", "X-Request-ID, X-DeployMonster-Version, X-API-Version")
			w.Header().Set("Access-Control-Allow-Credentials", "true")
			w.Header().Set("Access-Control-Max-Age", "86400")

			if r.Method == http.MethodOptions {
				w.WriteHeader(http.StatusNoContent)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// RequireAuth returns middleware that validates JWT from the Authorization header,
// dm_access cookie, or X-API-Key header (in that priority order).
func RequireAuth(jwtSvc *auth.JWTService, bolt core.BoltStorer) func(http.Handler) http.Handler {
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

				// Extract prefix (first 8 chars) for lookup
				if len(apiKey) < 12 {
					writeErrorJSON(w, http.StatusUnauthorized, "invalid api key")
					return
				}

				// Check if bolt store is available for API key lookup
				if bolt == nil {
					writeErrorJSON(w, http.StatusUnauthorized, "api key authentication not available")
					return
				}

				keyPrefix := apiKey[:8]

				// Lookup API key by prefix using BoltStorer
				storedKey, err := bolt.GetAPIKeyByPrefix(r.Context(), keyPrefix)
				if err != nil {
					writeErrorJSON(w, http.StatusUnauthorized, "invalid api key")
					return
				}

				// Verify the full key using constant-time comparison to prevent timing attacks
				if subtle.ConstantTimeCompare([]byte(apiKey), []byte(storedKey.KeyHash)) != 1 {
					writeErrorJSON(w, http.StatusUnauthorized, "invalid api key")
					return
				}

				// Check if key is expired
				if storedKey.ExpiresAt != nil && time.Now().After(*storedKey.ExpiresAt) {
					writeErrorJSON(w, http.StatusUnauthorized, "api key expired")
					return
				}

				// Create claims from the API key's associated user
				// Note: RoleID and Email would need to be looked up from user if needed
				claims := &auth.Claims{
					UserID:   storedKey.UserID,
					TenantID: storedKey.TenantID,
				}

				ctx := auth.ContextWithClaims(r.Context(), claims)
				next.ServeHTTP(w, r.WithContext(ctx))
				return
			}

			writeErrorJSON(w, http.StatusUnauthorized, "missing authorization — use Bearer token or X-API-Key")
		})
	}
}

// RequireAPIKey returns middleware that validates API key from X-API-Key header.
// This is a stricter version that only accepts API keys, not JWT tokens.
func RequireAPIKey(bolt core.BoltStorer) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			apiKey := r.Header.Get("X-API-Key")
			if apiKey == "" {
				writeErrorJSON(w, http.StatusUnauthorized, "api key required")
				return
			}

			// Validate API key format
			if !strings.HasPrefix(apiKey, "dm_") {
				writeErrorJSON(w, http.StatusUnauthorized, "invalid api key format")
				return
			}

			// Extract prefix (first 8 chars) for lookup
			if len(apiKey) < 12 {
				writeErrorJSON(w, http.StatusUnauthorized, "invalid api key")
				return
			}

			// Check if bolt store is available
			if bolt == nil {
				writeErrorJSON(w, http.StatusUnauthorized, "api key authentication not available")
				return
			}

			keyPrefix := apiKey[:8]

			// Lookup API key by prefix using BoltStorer
			storedKey, err := bolt.GetAPIKeyByPrefix(r.Context(), keyPrefix)
			if err != nil {
				writeErrorJSON(w, http.StatusUnauthorized, "invalid api key")
				return
			}

			// Verify the full key using constant-time comparison
			if subtle.ConstantTimeCompare([]byte(apiKey), []byte(storedKey.KeyHash)) != 1 {
				writeErrorJSON(w, http.StatusUnauthorized, "invalid api key")
				return
			}

			// Check if key is expired
			if storedKey.ExpiresAt != nil && time.Now().After(*storedKey.ExpiresAt) {
				writeErrorJSON(w, http.StatusUnauthorized, "api key expired")
				return
			}

			// Create claims from the API key's associated user
			// Note: RoleID and Email would need to be looked up from user if needed
			claims := &auth.Claims{
				UserID:   storedKey.UserID,
				TenantID: storedKey.TenantID,
			}

			ctx := auth.ContextWithClaims(r.Context(), claims)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// statusWriter wraps ResponseWriter to capture the status code.
type statusWriter struct {
	http.ResponseWriter
	status int
}

func (sw *statusWriter) WriteHeader(code int) {
	sw.status = code
	sw.ResponseWriter.WriteHeader(code)
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
