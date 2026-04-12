package handlers

import (
	"crypto/tls"
	"net/http"
	"net/http/httptest"
	"testing"

	internalAuth "github.com/deploy-monster/deploy-monster/internal/auth"
)

func TestGenerateSlug(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"John Doe", "john-doe"},
		{"My Team", "my-team"},
		{"UPPER", "upper"},
		{"with_underscores", "with-underscores"},
		{"special!@#chars", "specialchars"},
		{"Mixed 123 Case", "mixed-123-case"},
	}

	for _, tt := range tests {
		got := generateSlug(tt.input)
		if got != tt.want {
			t.Errorf("generateSlug(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

// TestIsSecureRequest guards the Tier 103 fix: auth cookies now only
// get the Secure flag when the request arrived over TLS (directly or
// behind a reverse proxy that set X-Forwarded-Proto). Setting Secure
// unconditionally broke the E2E Playwright suite in CI because the
// server listens on plain HTTP and Chromium silently drops Secure
// cookies set over http://, so every authenticated test landed on
// /login.
func TestIsSecureRequest(t *testing.T) {
	cases := []struct {
		name  string
		build func() *http.Request
		want  bool
	}{
		{
			name: "nil request",
			build: func() *http.Request {
				return nil
			},
			want: false,
		},
		{
			name: "plain http",
			build: func() *http.Request {
				return httptest.NewRequest(http.MethodPost, "http://localhost:8443/api/v1/auth/login", nil)
			},
			want: false,
		},
		{
			name: "direct TLS",
			build: func() *http.Request {
				r := httptest.NewRequest(http.MethodPost, "https://example.test/api/v1/auth/login", nil)
				r.TLS = &tls.ConnectionState{}
				return r
			},
			want: true,
		},
		{
			name: "forwarded proto https",
			build: func() *http.Request {
				r := httptest.NewRequest(http.MethodPost, "http://localhost/api/v1/auth/login", nil)
				r.Header.Set("X-Forwarded-Proto", "https")
				return r
			},
			want: true,
		},
		{
			name: "forwarded proto http",
			build: func() *http.Request {
				r := httptest.NewRequest(http.MethodPost, "http://localhost/api/v1/auth/login", nil)
				r.Header.Set("X-Forwarded-Proto", "http")
				return r
			},
			want: false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := isSecureRequest(tc.build()); got != tc.want {
				t.Errorf("isSecureRequest = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestSetTokenCookies_SecureFlagGatedOnTransport(t *testing.T) {
	tokens := &internalAuth.TokenPair{
		AccessToken:  "access.jwt.token",
		RefreshToken: "refresh.jwt.token",
		ExpiresIn:    900,
	}

	t.Run("plain http drops secure", func(t *testing.T) {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "http://localhost:8443/api/v1/auth/login", nil)
		setTokenCookies(rec, req, tokens)

		cookies := rec.Result().Cookies()
		if len(cookies) != 2 {
			t.Fatalf("expected 2 cookies, got %d", len(cookies))
		}
		for _, c := range cookies {
			if c.Secure {
				t.Errorf("cookie %q must NOT be Secure over plain HTTP", c.Name)
			}
			if !c.HttpOnly {
				t.Errorf("cookie %q must stay HttpOnly even over plain HTTP", c.Name)
			}
		}
	})

	t.Run("https sets secure", func(t *testing.T) {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "https://example.test/api/v1/auth/login", nil)
		req.TLS = &tls.ConnectionState{}
		setTokenCookies(rec, req, tokens)

		for _, c := range rec.Result().Cookies() {
			if !c.Secure {
				t.Errorf("cookie %q must be Secure over TLS", c.Name)
			}
		}
	})
}

func TestClearTokenCookies_SecureFlagGatedOnTransport(t *testing.T) {
	t.Run("plain http", func(t *testing.T) {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "http://localhost:8443/api/v1/auth/logout", nil)
		clearTokenCookies(rec, req)
		for _, c := range rec.Result().Cookies() {
			if c.Secure {
				t.Errorf("clear cookie %q must not be Secure over plain HTTP", c.Name)
			}
			if c.MaxAge != -1 {
				t.Errorf("clear cookie %q MaxAge = %d, want -1", c.Name, c.MaxAge)
			}
		}
	})

	t.Run("https", func(t *testing.T) {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "https://example.test/api/v1/auth/logout", nil)
		req.TLS = &tls.ConnectionState{}
		clearTokenCookies(rec, req)
		for _, c := range rec.Result().Cookies() {
			if !c.Secure {
				t.Errorf("clear cookie %q must be Secure over TLS", c.Name)
			}
		}
	})
}
