package auth

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestNewGoogleOAuth(t *testing.T) {
	p := NewGoogleOAuth("google-client-id", "google-client-secret")

	if p.Name != "google" {
		t.Errorf("expected name %q, got %q", "google", p.Name)
	}
	if p.ClientID != "google-client-id" {
		t.Errorf("expected ClientID %q, got %q", "google-client-id", p.ClientID)
	}
	if p.ClientSecret != "google-client-secret" {
		t.Errorf("expected ClientSecret %q, got %q", "google-client-secret", p.ClientSecret)
	}
	if p.AuthURL != "https://accounts.google.com/o/oauth2/v2/auth" {
		t.Errorf("unexpected AuthURL: %q", p.AuthURL)
	}
	if p.TokenURL != "https://oauth2.googleapis.com/token" {
		t.Errorf("unexpected TokenURL: %q", p.TokenURL)
	}
	if p.UserInfoURL != "https://www.googleapis.com/oauth2/v2/userinfo" {
		t.Errorf("unexpected UserInfoURL: %q", p.UserInfoURL)
	}
	if len(p.Scopes) != 2 || p.Scopes[0] != "email" || p.Scopes[1] != "profile" {
		t.Errorf("unexpected scopes: %v", p.Scopes)
	}
	if p.client == nil {
		t.Error("HTTP client should be initialized")
	}
}

func TestNewGitHubOAuth(t *testing.T) {
	p := NewGitHubOAuth("gh-client-id", "gh-client-secret")

	if p.Name != "github" {
		t.Errorf("expected name %q, got %q", "github", p.Name)
	}
	if p.ClientID != "gh-client-id" {
		t.Errorf("expected ClientID %q, got %q", "gh-client-id", p.ClientID)
	}
	if p.ClientSecret != "gh-client-secret" {
		t.Errorf("expected ClientSecret %q, got %q", "gh-client-secret", p.ClientSecret)
	}
	if p.AuthURL != "https://github.com/login/oauth/authorize" {
		t.Errorf("unexpected AuthURL: %q", p.AuthURL)
	}
	if p.TokenURL != "https://github.com/login/oauth/access_token" {
		t.Errorf("unexpected TokenURL: %q", p.TokenURL)
	}
	if p.UserInfoURL != "https://api.github.com/user" {
		t.Errorf("unexpected UserInfoURL: %q", p.UserInfoURL)
	}
	if len(p.Scopes) != 2 || p.Scopes[0] != "read:user" || p.Scopes[1] != "user:email" {
		t.Errorf("unexpected scopes: %v", p.Scopes)
	}
	if p.client == nil {
		t.Error("HTTP client should be initialized")
	}
}

func TestAuthorizationURL(t *testing.T) {
	tests := []struct {
		name        string
		provider    *OAuthProvider
		redirectURI string
		state       string
		wantBase    string
	}{
		{
			name:        "google",
			provider:    NewGoogleOAuth("gid", "gsecret"),
			redirectURI: "http://localhost:3000/callback",
			state:       "random-state-123",
			wantBase:    "https://accounts.google.com/o/oauth2/v2/auth",
		},
		{
			name:        "github",
			provider:    NewGitHubOAuth("ghid", "ghsecret"),
			redirectURI: "http://localhost:3000/callback",
			state:       "state-456",
			wantBase:    "https://github.com/login/oauth/authorize",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.provider.AuthorizationURL(tt.redirectURI, tt.state)

			// Should start with the provider's auth URL
			if !strings.HasPrefix(result, tt.wantBase+"?") {
				t.Errorf("URL should start with %q, got %q", tt.wantBase+"?", result)
			}

			// Parse the URL and check query params
			u, err := url.Parse(result)
			if err != nil {
				t.Fatalf("failed to parse URL: %v", err)
			}

			params := u.Query()

			if got := params.Get("client_id"); got != tt.provider.ClientID {
				t.Errorf("client_id = %q, want %q", got, tt.provider.ClientID)
			}
			if got := params.Get("redirect_uri"); got != tt.redirectURI {
				t.Errorf("redirect_uri = %q, want %q", got, tt.redirectURI)
			}
			if got := params.Get("response_type"); got != "code" {
				t.Errorf("response_type = %q, want %q", got, "code")
			}
			if got := params.Get("state"); got != tt.state {
				t.Errorf("state = %q, want %q", got, tt.state)
			}
			if got := params.Get("scope"); got == "" {
				t.Error("scope should not be empty")
			}
		})
	}
}

func TestAuthorizationURL_EncodesParams(t *testing.T) {
	p := NewGoogleOAuth("client id with spaces", "secret")

	redirectURI := "http://localhost:3000/auth/callback?foo=bar&baz=qux"
	state := "state=with&special chars"

	result := p.AuthorizationURL(redirectURI, state)

	u, err := url.Parse(result)
	if err != nil {
		t.Fatalf("failed to parse URL: %v", err)
	}

	params := u.Query()

	// url.Values.Encode() should handle encoding; verify round-trip
	if got := params.Get("client_id"); got != "client id with spaces" {
		t.Errorf("client_id not properly encoded/decoded: %q", got)
	}
	if got := params.Get("redirect_uri"); got != redirectURI {
		t.Errorf("redirect_uri not properly encoded/decoded: %q", got)
	}
	if got := params.Get("state"); got != state {
		t.Errorf("state not properly encoded/decoded: %q", got)
	}
}

func TestAuthorizationURL_ScopeJoining(t *testing.T) {
	tests := []struct {
		name      string
		provider  *OAuthProvider
		wantScope string
	}{
		{
			name:      "google scopes joined with space",
			provider:  NewGoogleOAuth("cid", "cs"),
			wantScope: "email profile",
		},
		{
			name:      "github scopes joined with space",
			provider:  NewGitHubOAuth("cid", "cs"),
			wantScope: "read:user user:email",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.provider.AuthorizationURL("http://localhost/cb", "s")
			u, err := url.Parse(result)
			if err != nil {
				t.Fatalf("failed to parse URL: %v", err)
			}
			if got := u.Query().Get("scope"); got != tt.wantScope {
				t.Errorf("scope = %q, want %q", got, tt.wantScope)
			}
		})
	}
}

func TestJoinScopes(t *testing.T) {
	tests := []struct {
		name   string
		scopes []string
		want   string
	}{
		{"empty", nil, ""},
		{"single", []string{"email"}, "email"},
		{"multiple", []string{"email", "profile", "openid"}, "email profile openid"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := joinScopes(tt.scopes)
			if got != tt.want {
				t.Errorf("joinScopes(%v) = %q, want %q", tt.scopes, got, tt.want)
			}
		})
	}
}

func TestExchangeCode_Success(t *testing.T) {
	tokenServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if got := r.Header.Get("Accept"); got != "application/json" {
			t.Errorf("Accept header = %q, want %q", got, "application/json")
		}

		q := r.URL.Query()
		if got := q.Get("client_id"); got != "test-client" {
			t.Errorf("client_id = %q, want %q", got, "test-client")
		}
		if got := q.Get("client_secret"); got != "test-secret" {
			t.Errorf("client_secret = %q, want %q", got, "test-secret")
		}
		if got := q.Get("code"); got != "auth-code-123" {
			t.Errorf("code = %q, want %q", got, "auth-code-123")
		}
		if got := q.Get("redirect_uri"); got != "http://localhost/cb" {
			t.Errorf("redirect_uri = %q, want %q", got, "http://localhost/cb")
		}
		if got := q.Get("grant_type"); got != "authorization_code" {
			t.Errorf("grant_type = %q, want %q", got, "authorization_code")
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{
			"access_token": "mock-access-token-xyz",
		})
	}))
	defer tokenServer.Close()

	p := &OAuthProvider{
		Name:         "test",
		ClientID:     "test-client",
		ClientSecret: "test-secret",
		TokenURL:     tokenServer.URL,
		client:       &http.Client{Timeout: 5 * time.Second},
	}

	token, err := p.ExchangeCode(context.Background(), "auth-code-123", "http://localhost/cb")
	if err != nil {
		t.Fatalf("ExchangeCode: %v", err)
	}
	if token != "mock-access-token-xyz" {
		t.Errorf("token = %q, want %q", token, "mock-access-token-xyz")
	}
}

func TestExchangeCode_OAuthError(t *testing.T) {
	tokenServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{
			"error": "invalid_grant",
		})
	}))
	defer tokenServer.Close()

	p := &OAuthProvider{
		Name:         "test",
		ClientID:     "cid",
		ClientSecret: "cs",
		TokenURL:     tokenServer.URL,
		client:       &http.Client{Timeout: 5 * time.Second},
	}

	_, err := p.ExchangeCode(context.Background(), "bad-code", "http://localhost/cb")
	if err == nil {
		t.Fatal("expected error for invalid_grant response")
	}
	if !strings.Contains(err.Error(), "invalid_grant") {
		t.Errorf("error = %q, want it to contain %q", err.Error(), "invalid_grant")
	}
}

func TestExchangeCode_NetworkError(t *testing.T) {
	p := &OAuthProvider{
		Name:         "test",
		ClientID:     "cid",
		ClientSecret: "cs",
		TokenURL:     "http://127.0.0.1:1",
		client:       &http.Client{Timeout: 1 * time.Second},
	}

	_, err := p.ExchangeCode(context.Background(), "code", "http://localhost/cb")
	if err == nil {
		t.Fatal("expected error for unreachable server")
	}
}

func TestGetUser_Google(t *testing.T) {
	userServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer google-token" {
			t.Errorf("Authorization = %q, want %q", got, "Bearer google-token")
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{
			"id":      "g-123",
			"email":   "user@gmail.com",
			"name":    "Google User",
			"picture": "https://example.com/avatar.png",
		})
	}))
	defer userServer.Close()

	p := &OAuthProvider{
		Name:        "google",
		UserInfoURL: userServer.URL,
		client:      &http.Client{Timeout: 5 * time.Second},
	}

	user, err := p.GetUser(context.Background(), "google-token")
	if err != nil {
		t.Fatalf("GetUser: %v", err)
	}
	if user.ID != "g-123" {
		t.Errorf("ID = %q, want %q", user.ID, "g-123")
	}
	if user.Email != "user@gmail.com" {
		t.Errorf("Email = %q, want %q", user.Email, "user@gmail.com")
	}
	if user.Name != "Google User" {
		t.Errorf("Name = %q, want %q", user.Name, "Google User")
	}
	if user.Avatar != "https://example.com/avatar.png" {
		t.Errorf("Avatar = %q, want %q", user.Avatar, "https://example.com/avatar.png")
	}
	if user.Provider != "google" {
		t.Errorf("Provider = %q, want %q", user.Provider, "google")
	}
}

func TestGetUser_GitHub(t *testing.T) {
	userServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"id":         42,
			"email":      "dev@github.com",
			"name":       "GitHub User",
			"login":      "ghuser",
			"avatar_url": "https://github.com/avatar.png",
		})
	}))
	defer userServer.Close()

	p := &OAuthProvider{
		Name:        "github",
		UserInfoURL: userServer.URL,
		client:      &http.Client{Timeout: 5 * time.Second},
	}

	user, err := p.GetUser(context.Background(), "gh-token")
	if err != nil {
		t.Fatalf("GetUser: %v", err)
	}
	if user.ID != "42" {
		t.Errorf("ID = %q, want %q", user.ID, "42")
	}
	if user.Email != "dev@github.com" {
		t.Errorf("Email = %q, want %q", user.Email, "dev@github.com")
	}
	if user.Name != "GitHub User" {
		t.Errorf("Name = %q, want %q", user.Name, "GitHub User")
	}
	if user.Avatar != "https://github.com/avatar.png" {
		t.Errorf("Avatar = %q, want %q", user.Avatar, "https://github.com/avatar.png")
	}
	if user.Provider != "github" {
		t.Errorf("Provider = %q, want %q", user.Provider, "github")
	}
}

func TestGetUser_GitHub_FallbackToLogin(t *testing.T) {
	userServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"id":    99,
			"email": "noname@github.com",
			"name":  "",
			"login": "loginuser",
		})
	}))
	defer userServer.Close()

	p := &OAuthProvider{
		Name:        "github",
		UserInfoURL: userServer.URL,
		client:      &http.Client{Timeout: 5 * time.Second},
	}

	user, err := p.GetUser(context.Background(), "gh-token")
	if err != nil {
		t.Fatalf("GetUser: %v", err)
	}
	if user.Name != "loginuser" {
		t.Errorf("Name = %q, want %q (should fall back to login)", user.Name, "loginuser")
	}
}

func TestGetUser_NetworkError(t *testing.T) {
	p := &OAuthProvider{
		Name:        "google",
		UserInfoURL: "http://127.0.0.1:1",
		client:      &http.Client{Timeout: 1 * time.Second},
	}

	_, err := p.GetUser(context.Background(), "token")
	if err == nil {
		t.Fatal("expected error for unreachable server")
	}
}
