package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"
)

// OAuthProvider represents a configured SSO provider.
type OAuthProvider struct {
	Name         string
	ClientID     string
	ClientSecret string
	AuthURL      string
	TokenURL     string
	UserInfoURL  string
	Scopes       []string
	client       *http.Client
}

// OAuthUser holds the normalized user info from an OAuth provider.
type OAuthUser struct {
	ID       string `json:"id"`
	Email    string `json:"email"`
	Name     string `json:"name"`
	Avatar   string `json:"avatar_url"`
	Provider string `json:"provider"`
}

// NewGoogleOAuth creates a Google OAuth provider.
func NewGoogleOAuth(clientID, clientSecret string) *OAuthProvider {
	return &OAuthProvider{
		Name:         "google",
		ClientID:     clientID,
		ClientSecret: clientSecret,
		AuthURL:      "https://accounts.google.com/o/oauth2/v2/auth",
		TokenURL:     "https://oauth2.googleapis.com/token",
		UserInfoURL:  "https://www.googleapis.com/oauth2/v2/userinfo",
		Scopes:       []string{"email", "profile"},
		client:       &http.Client{Timeout: 10 * time.Second},
	}
}

// NewGitHubOAuth creates a GitHub OAuth provider.
func NewGitHubOAuth(clientID, clientSecret string) *OAuthProvider {
	return &OAuthProvider{
		Name:         "github",
		ClientID:     clientID,
		ClientSecret: clientSecret,
		AuthURL:      "https://github.com/login/oauth/authorize",
		TokenURL:     "https://github.com/login/oauth/access_token",
		UserInfoURL:  "https://api.github.com/user",
		Scopes:       []string{"read:user", "user:email"},
		client:       &http.Client{Timeout: 10 * time.Second},
	}
}

// AuthorizationURL returns the URL to redirect the user to for OAuth
// consent. The pre-PKCE signature is kept so existing call sites keep
// working; new call sites should use AuthorizationURLPKCE which also
// returns the code_verifier to persist in the state cookie.
func (p *OAuthProvider) AuthorizationURL(redirectURI, state string) string {
	params := url.Values{
		"client_id":     {p.ClientID},
		"redirect_uri":  {redirectURI},
		"response_type": {"code"},
		"state":         {state},
		"scope":         {joinScopes(p.Scopes)},
	}
	return p.AuthURL + "?" + params.Encode()
}

// PKCEChallenge holds the S256 PKCE pair generated for an authorization
// request. The verifier is the high-entropy secret held by the client
// until the token exchange; the challenge is derived from it and sent
// with the /authorize request. Only the challenge is safe to leak.
type PKCEChallenge struct {
	Verifier  string // random 32-byte base64url; keep server-side in the state cookie
	Challenge string // SHA-256(verifier), base64url; sent in /authorize
	Method    string // always "S256"
}

// NewPKCEChallenge generates a fresh PKCE verifier/challenge pair per
// RFC 7636 §4. The verifier has 32 bytes of crypto/rand entropy — well
// above the 43-character minimum in the spec.
func NewPKCEChallenge() (*PKCEChallenge, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return nil, fmt.Errorf("pkce rand: %w", err)
	}
	verifier := base64.RawURLEncoding.EncodeToString(raw)
	sum := sha256.Sum256([]byte(verifier))
	return &PKCEChallenge{
		Verifier:  verifier,
		Challenge: base64.RawURLEncoding.EncodeToString(sum[:]),
		Method:    "S256",
	}, nil
}

// AuthorizationURLPKCE is AuthorizationURL with RFC 7636 PKCE S256
// parameters appended. Call NewPKCEChallenge() to produce `challenge`,
// persist the verifier alongside the state cookie, and then hand the
// verifier back to ExchangeCodePKCE during the token exchange.
//
// Adding PKCE defends against authorization-code interception attacks
// where an attacker on the redirect path captures the code before the
// client can exchange it. Without a verifier the stolen code is
// useless.
func (p *OAuthProvider) AuthorizationURLPKCE(redirectURI, state string, challenge *PKCEChallenge) string {
	if challenge == nil {
		return p.AuthorizationURL(redirectURI, state)
	}
	params := url.Values{
		"client_id":             {p.ClientID},
		"redirect_uri":          {redirectURI},
		"response_type":         {"code"},
		"state":                 {state},
		"scope":                 {joinScopes(p.Scopes)},
		"code_challenge":        {challenge.Challenge},
		"code_challenge_method": {challenge.Method},
	}
	return p.AuthURL + "?" + params.Encode()
}

// ExchangeCode exchanges an authorization code for an access token.
func (p *OAuthProvider) ExchangeCode(ctx context.Context, code, redirectURI string) (string, error) {
	return p.exchange(ctx, code, redirectURI, "")
}

// ExchangeCodePKCE is ExchangeCode with an RFC 7636 `code_verifier`
// parameter. The verifier must be the same string used to derive the
// `code_challenge` sent on the authorize step.
func (p *OAuthProvider) ExchangeCodePKCE(ctx context.Context, code, redirectURI, codeVerifier string) (string, error) {
	return p.exchange(ctx, code, redirectURI, codeVerifier)
}

func (p *OAuthProvider) exchange(ctx context.Context, code, redirectURI, codeVerifier string) (string, error) {
	params := url.Values{
		"client_id":     {p.ClientID},
		"client_secret": {p.ClientSecret},
		"code":          {code},
		"redirect_uri":  {redirectURI},
		"grant_type":    {"authorization_code"},
	}
	if codeVerifier != "" {
		params.Set("code_verifier", codeVerifier)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.TokenURL, nil)
	if err != nil {
		return "", err
	}
	req.URL.RawQuery = params.Encode()
	req.Header.Set("Accept", "application/json")

	resp, err := p.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("token exchange: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		io.Copy(io.Discard, resp.Body)
		return "", fmt.Errorf("token exchange: HTTP %d", resp.StatusCode)
	}

	body, _ := io.ReadAll(resp.Body)
	var tokenResp struct {
		AccessToken string `json:"access_token"`
		Error       string `json:"error"`
	}
	if err := json.Unmarshal(body, &tokenResp); err != nil {
		return "", fmt.Errorf("token response parse: %w", err)
	}

	if tokenResp.Error != "" {
		return "", fmt.Errorf("oauth error: %s", tokenResp.Error)
	}
	return tokenResp.AccessToken, nil
}

// GetUser fetches the user profile from the OAuth provider.
func (p *OAuthProvider) GetUser(ctx context.Context, accessToken string) (*OAuthUser, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, p.UserInfoURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("get user info: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		io.Copy(io.Discard, resp.Body)
		return nil, fmt.Errorf("get user info: HTTP %d", resp.StatusCode)
	}

	body, _ := io.ReadAll(resp.Body)

	user := &OAuthUser{Provider: p.Name}

	switch p.Name {
	case "google":
		var g struct {
			ID      string `json:"id"`
			Email   string `json:"email"`
			Name    string `json:"name"`
			Picture string `json:"picture"`
		}
		if err := json.Unmarshal(body, &g); err != nil {
			return nil, fmt.Errorf("google user parse: %w", err)
		}
		user.ID = g.ID
		user.Email = g.Email
		user.Name = g.Name
		user.Avatar = g.Picture

	case "github":
		var g struct {
			ID     int    `json:"id"`
			Email  string `json:"email"`
			Name   string `json:"name"`
			Login  string `json:"login"`
			Avatar string `json:"avatar_url"`
		}
		if err := json.Unmarshal(body, &g); err != nil {
			return nil, fmt.Errorf("github user parse: %w", err)
		}
		user.ID = fmt.Sprintf("%d", g.ID)
		user.Email = g.Email
		user.Name = g.Name
		if user.Name == "" {
			user.Name = g.Login
		}
		user.Avatar = g.Avatar
	}

	return user, nil
}

func joinScopes(scopes []string) string {
	result := ""
	for i, s := range scopes {
		if i > 0 {
			result += " "
		}
		result += s
	}
	return result
}
