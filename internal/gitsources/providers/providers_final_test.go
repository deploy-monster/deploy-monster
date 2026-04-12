package providers

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

// =============================================================================
// GitHub.do — HTTP 400+ error response branch (line 154)
// =============================================================================

func TestGitHub_Do_HTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		w.Write([]byte(`{"message":"rate limit"}`))
	}))
	defer srv.Close()

	gh := &GitHub{
		token:  "test-token",
		client: srv.Client(),
	}
	// Patch the do method indirectly by using the get method and intercepting URL
	_, _ = gh.do(context.Background(), http.MethodGet, srv.URL+"/test", nil)
	// Since do prepends "https://api.github.com", we can't easily redirect.
	// Instead test via a transport rewrite approach.

	// Use the simpler approach: test network error path
	gh2 := &GitHub{
		token: "test-token",
		client: &http.Client{Transport: &testTransport{handler: func(req *http.Request) (*http.Response, error) {
			rr := httptest.NewRecorder()
			rr.WriteHeader(http.StatusForbidden)
			rr.WriteString(`{"message":"forbidden"}`)
			return rr.Result(), nil
		}}},
	}

	_, err := gh2.do(context.Background(), http.MethodGet, "/user/repos", nil)
	if err == nil {
		t.Error("expected error for HTTP 403")
	}
}

// =============================================================================
// GitLab.do — HTTP error response branch (line 142)
// =============================================================================

func TestGitLab_Do_HTTPError(t *testing.T) {
	gl := &GitLab{
		token:   "test-token",
		baseURL: "https://gitlab.com/api/v4",
		client: &http.Client{Transport: &testTransport{handler: func(req *http.Request) (*http.Response, error) {
			rr := httptest.NewRecorder()
			rr.WriteHeader(http.StatusUnauthorized)
			return rr.Result(), nil
		}}},
	}

	_, err := gl.do(context.Background(), http.MethodGet, "/projects", nil)
	if err == nil {
		t.Error("expected error for HTTP 401")
	}
}

// =============================================================================
// Bitbucket.do — HTTP error response branch (line 171)
// =============================================================================

func TestBitbucket_Do_HTTPError(t *testing.T) {
	bb := &Bitbucket{
		token: "test-token",
		client: &http.Client{Transport: &testTransport{handler: func(req *http.Request) (*http.Response, error) {
			rr := httptest.NewRecorder()
			rr.WriteHeader(http.StatusNotFound)
			return rr.Result(), nil
		}}},
	}

	_, err := bb.do(context.Background(), http.MethodGet, "/repositories/foo/bar", nil)
	if err == nil {
		t.Error("expected error for HTTP 404")
	}
}

// =============================================================================
// Gitea.do — HTTP error response branch (line 145)
// =============================================================================

func TestGitea_Do_HTTPError(t *testing.T) {
	g := &Gitea{
		token:   "test-token",
		baseURL: "https://gitea.com/api/v1",
		client: &http.Client{Transport: &testTransport{handler: func(req *http.Request) (*http.Response, error) {
			rr := httptest.NewRecorder()
			rr.WriteHeader(http.StatusInternalServerError)
			return rr.Result(), nil
		}}},
	}

	_, err := g.do(context.Background(), http.MethodGet, "/user/repos", nil)
	if err == nil {
		t.Error("expected error for HTTP 500")
	}
}

// testTransport allows intercepting HTTP requests in tests.
type testTransport struct {
	handler func(*http.Request) (*http.Response, error)
}

func (t *testTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	return t.handler(req)
}
