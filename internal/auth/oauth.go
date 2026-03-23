package auth

import (
	"context"
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

// AuthorizationURL returns the URL to redirect the user to for OAuth consent.
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

// ExchangeCode exchanges an authorization code for an access token.
func (p *OAuthProvider) ExchangeCode(ctx context.Context, code, redirectURI string) (string, error) {
	params := url.Values{
		"client_id":     {p.ClientID},
		"client_secret": {p.ClientSecret},
		"code":          {code},
		"redirect_uri":  {redirectURI},
		"grant_type":    {"authorization_code"},
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

	body, _ := io.ReadAll(resp.Body)
	var tokenResp struct {
		AccessToken string `json:"access_token"`
		Error       string `json:"error"`
	}
	json.Unmarshal(body, &tokenResp)

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
		json.Unmarshal(body, &g)
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
		json.Unmarshal(body, &g)
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
