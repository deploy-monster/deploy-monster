package auth

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestNewPKCEChallenge_S256(t *testing.T) {
	c, err := NewPKCEChallenge()
	if err != nil {
		t.Fatalf("NewPKCEChallenge: %v", err)
	}

	if c.Method != "S256" {
		t.Errorf("Method = %q, want S256", c.Method)
	}
	// Verifier must be long enough per RFC 7636 §4.1 (43-128 chars).
	if len(c.Verifier) < 43 || len(c.Verifier) > 128 {
		t.Errorf("Verifier length %d out of RFC 7636 range [43,128]", len(c.Verifier))
	}
	// Challenge must equal base64url(SHA256(verifier)).
	sum := sha256.Sum256([]byte(c.Verifier))
	want := base64.RawURLEncoding.EncodeToString(sum[:])
	if c.Challenge != want {
		t.Errorf("Challenge = %q, want %q", c.Challenge, want)
	}
}

func TestNewPKCEChallenge_Unique(t *testing.T) {
	// Two successive calls must produce distinct verifiers. Any
	// duplication would indicate a broken crypto/rand source.
	seen := make(map[string]bool, 16)
	for i := 0; i < 16; i++ {
		c, err := NewPKCEChallenge()
		if err != nil {
			t.Fatalf("NewPKCEChallenge: %v", err)
		}
		if seen[c.Verifier] {
			t.Fatalf("verifier collision at iteration %d: %s", i, c.Verifier)
		}
		seen[c.Verifier] = true
	}
}

func TestAuthorizationURLPKCE_IncludesChallenge(t *testing.T) {
	p := NewGoogleOAuth("cid", "csecret")
	c, _ := NewPKCEChallenge()

	raw := p.AuthorizationURLPKCE("https://example.com/cb", "state-xyz", c)
	u, err := url.Parse(raw)
	if err != nil {
		t.Fatalf("url.Parse: %v", err)
	}
	q := u.Query()

	if q.Get("code_challenge") != c.Challenge {
		t.Errorf("code_challenge = %q, want %q", q.Get("code_challenge"), c.Challenge)
	}
	if q.Get("code_challenge_method") != "S256" {
		t.Errorf("code_challenge_method = %q, want S256", q.Get("code_challenge_method"))
	}
	if q.Get("state") != "state-xyz" {
		t.Errorf("state = %q, want state-xyz", q.Get("state"))
	}
	if q.Get("client_id") != "cid" {
		t.Errorf("client_id = %q, want cid", q.Get("client_id"))
	}
}

func TestAuthorizationURLPKCE_NilChallengeFallsBack(t *testing.T) {
	p := NewGoogleOAuth("cid", "csecret")
	raw := p.AuthorizationURLPKCE("https://example.com/cb", "state", nil)
	if strings.Contains(raw, "code_challenge") {
		t.Errorf("nil PKCE should fall back to plain AuthorizationURL, got %q", raw)
	}
}

func TestExchangeCodePKCE_SendsVerifier(t *testing.T) {
	// Capture the token-exchange request and assert the verifier is
	// forwarded to the identity provider.
	var gotVerifier string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			// The oauth client posts params in the raw query — cover both.
			_ = r.ParseForm()
		}
		gotVerifier = r.URL.Query().Get("code_verifier")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"access_token":"tok-xyz"}`))
	}))
	defer srv.Close()

	p := &OAuthProvider{
		Name:         "fake",
		ClientID:     "cid",
		ClientSecret: "csecret",
		TokenURL:     srv.URL,
		client:       srv.Client(),
	}

	tok, err := p.ExchangeCodePKCE(context.Background(), "auth-code", "https://example.com/cb", "verifier-abc")
	if err != nil {
		t.Fatalf("ExchangeCodePKCE: %v", err)
	}
	if tok != "tok-xyz" {
		t.Errorf("access token = %q, want tok-xyz", tok)
	}
	if gotVerifier != "verifier-abc" {
		t.Errorf("IdP received code_verifier = %q, want verifier-abc", gotVerifier)
	}
}

func TestExchangeCode_OmitsVerifier(t *testing.T) {
	// The pre-PKCE ExchangeCode path must not send a code_verifier
	// parameter, otherwise providers that validate PKCE strictly
	// (Google, Okta) will reject the exchange with a
	// "code_verifier required" error.
	var seenVerifier string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seenVerifier = r.URL.Query().Get("code_verifier")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"access_token":"tok"}`))
	}))
	defer srv.Close()

	p := &OAuthProvider{
		ClientID:     "cid",
		ClientSecret: "csecret",
		TokenURL:     srv.URL,
		client:       srv.Client(),
	}

	if _, err := p.ExchangeCode(context.Background(), "auth-code", "https://example.com/cb"); err != nil {
		t.Fatalf("ExchangeCode: %v", err)
	}
	if seenVerifier != "" {
		t.Errorf("ExchangeCode sent code_verifier=%q, want empty", seenVerifier)
	}
}
