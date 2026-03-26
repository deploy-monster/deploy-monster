package middleware

import (
	"crypto/subtle"
	"log/slog"
	"net/http"
	"runtime/debug"
	"strings"
	"time"

	"github.com/deploy-monster/deploy-monster/internal/auth"
	"github.com/deploy-monster/deploy-monster/internal/core"
)

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
					http.Error(w, `{"error":"internal server error"}`, http.StatusInternalServerError)
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

			logger.Info("request",
				"method", r.Method,
				"path", r.URL.Path,
				"status", sw.status,
				"duration", time.Since(start).String(),
				"ip", realIP(r),
			)
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
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-API-Key, X-Request-ID")
			w.Header().Set("Access-Control-Expose-Headers", "X-Request-ID, X-DeployMonster-Version, X-API-Version")
			w.Header().Set("Access-Control-Max-Age", "86400")

			if r.Method == http.MethodOptions {
				w.WriteHeader(http.StatusNoContent)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// RequireAuth returns middleware that validates JWT from the Authorization header.
// It also validates API keys using BoltStorer for lookup.
func RequireAuth(jwtSvc *auth.JWTService, bolt core.BoltStorer) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Try JWT from Authorization header
			header := r.Header.Get("Authorization")
			if strings.HasPrefix(header, "Bearer ") {
				tokenStr := strings.TrimPrefix(header, "Bearer ")
				claims, err := jwtSvc.ValidateAccessToken(tokenStr)
				if err != nil {
					http.Error(w, `{"error":"invalid token"}`, http.StatusUnauthorized)
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
					http.Error(w, `{"error":"invalid api key format"}`, http.StatusUnauthorized)
					return
				}

				// Extract prefix (first 8 chars) for lookup
				if len(apiKey) < 12 {
					http.Error(w, `{"error":"invalid api key"}`, http.StatusUnauthorized)
					return
				}

				// Check if bolt store is available for API key lookup
				if bolt == nil {
					http.Error(w, `{"error":"api key authentication not available"}`, http.StatusUnauthorized)
					return
				}

				keyPrefix := apiKey[:8]

				// Lookup API key by prefix using BoltStorer
				storedKey, err := bolt.GetAPIKeyByPrefix(r.Context(), keyPrefix)
				if err != nil {
					http.Error(w, `{"error":"invalid api key"}`, http.StatusUnauthorized)
					return
				}

				// Verify the full key using constant-time comparison to prevent timing attacks
				if subtle.ConstantTimeCompare([]byte(apiKey), []byte(storedKey.KeyHash)) != 1 {
					http.Error(w, `{"error":"invalid api key"}`, http.StatusUnauthorized)
					return
				}

				// Check if key is expired
				if storedKey.ExpiresAt != nil && time.Now().After(*storedKey.ExpiresAt) {
					http.Error(w, `{"error":"api key expired"}`, http.StatusUnauthorized)
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

			http.Error(w, `{"error":"missing authorization — use Bearer token or X-API-Key"}`, http.StatusUnauthorized)
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
				http.Error(w, `{"error":"api key required"}`, http.StatusUnauthorized)
				return
			}

			// Validate API key format
			if !strings.HasPrefix(apiKey, "dm_") {
				http.Error(w, `{"error":"invalid api key format"}`, http.StatusUnauthorized)
				return
			}

			// Extract prefix (first 8 chars) for lookup
			if len(apiKey) < 12 {
				http.Error(w, `{"error":"invalid api key"}`, http.StatusUnauthorized)
				return
			}

			// Check if bolt store is available
			if bolt == nil {
				http.Error(w, `{"error":"api key authentication not available"}`, http.StatusUnauthorized)
				return
			}

			keyPrefix := apiKey[:8]

			// Lookup API key by prefix using BoltStorer
			storedKey, err := bolt.GetAPIKeyByPrefix(r.Context(), keyPrefix)
			if err != nil {
				http.Error(w, `{"error":"invalid api key"}`, http.StatusUnauthorized)
				return
			}

			// Verify the full key using constant-time comparison
			if subtle.ConstantTimeCompare([]byte(apiKey), []byte(storedKey.KeyHash)) != 1 {
				http.Error(w, `{"error":"invalid api key"}`, http.StatusUnauthorized)
				return
			}

			// Check if key is expired
			if storedKey.ExpiresAt != nil && time.Now().After(*storedKey.ExpiresAt) {
				http.Error(w, `{"error":"api key expired"}`, http.StatusUnauthorized)
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
