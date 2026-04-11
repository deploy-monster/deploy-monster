package webhooks

import (
	"context"
	"fmt"
	"net/http"
	"testing"
)

// FuzzVerifyGitHubSignature generates a valid HMAC for random body+secret pairs,
// then confirms that VerifyGitHubSignature accepts the computed signature.
func FuzzVerifyGitHubSignature(f *testing.F) {
	f.Add([]byte(`{"ref":"refs/heads/main"}`), "my-secret")
	f.Add([]byte(`{}`), "")
	f.Add([]byte{0, 1, 2, 255}, "s3cr3t!")
	f.Add([]byte(`{"commits":[],"head_commit":null}`), "long-webhook-secret-value-here-1234567890")

	f.Fuzz(func(t *testing.T, body []byte, secret string) {
		// Compute the correct HMAC
		mac := computeHMACSHA256(body, secret)
		sig := fmt.Sprintf("sha256=%s", mac)

		if !VerifyGitHubSignature(body, secret, sig) {
			t.Error("VerifyGitHubSignature should accept a correctly computed signature")
		}

		// Signature without prefix must be rejected
		if VerifyGitHubSignature(body, secret, mac) {
			t.Error("VerifyGitHubSignature should reject signature without sha256= prefix")
		}
	})
}

// FuzzParsePayload sends random JSON blobs through parsePayload for every
// supported provider and asserts it never panics.
func FuzzParsePayload(f *testing.F) {
	f.Add([]byte(`{"ref":"refs/heads/main","head_commit":{"id":"abc","message":"test","author":{"name":"dev"}},"repository":{"clone_url":"https://github.com/test/repo.git","full_name":"test/repo"}}`))
	f.Add([]byte(`{}`))
	f.Add([]byte(`{"provider":"generic","event_type":"push","branch":"main"}`))
	f.Add([]byte(`not json at all`))
	f.Add([]byte{})

	providers := []string{"github", "gitlab", "gitea", "gogs", "generic"}

	f.Fuzz(func(t *testing.T, body []byte) {
		for _, provider := range providers {
			// Build a minimal request with appropriate headers
			req, _ := http.NewRequest("POST", "/hooks/v1/test", nil)
			switch provider {
			case "github":
				req.Header.Set("X-GitHub-Event", "push")
			case "gitlab":
				req.Header.Set("X-Gitlab-Event", "Push Hook")
			case "gitea":
				req.Header.Set("X-Gitea-Event", "push")
			case "gogs":
				req.Header.Set("X-Gogs-Event", "push")
			}

			// Must not panic — errors are fine
			_, _ = parsePayload(provider, body, req)
		}
	})
}

// FuzzVerifyBitbucketSignature confirms VerifyBitbucketSignature accepts a
// correctly computed HMAC with or without the "sha256=" prefix, and rejects
// an empty header. Mirrors the GitHub fuzzer's round-trip invariant.
func FuzzVerifyBitbucketSignature(f *testing.F) {
	f.Add([]byte(`{"eventKey":"repo:refs_changed"}`), "bb-secret")
	f.Add([]byte(`{}`), "")
	f.Add([]byte{0xff, 0x00, 0x7f}, "a-long-bitbucket-webhook-secret")

	f.Fuzz(func(t *testing.T, body []byte, secret string) {
		mac := computeHMACSHA256(body, secret)

		// Prefixed form (Bitbucket Server ≥ 5.4)
		if !VerifyBitbucketSignature(body, secret, "sha256="+mac) {
			t.Error("VerifyBitbucketSignature should accept prefixed signature")
		}
		// Raw hex form (older Bitbucket Server)
		if !VerifyBitbucketSignature(body, secret, mac) {
			t.Error("VerifyBitbucketSignature should accept raw hex signature")
		}
		// Empty header is always rejected
		if VerifyBitbucketSignature(body, secret, "") {
			t.Error("VerifyBitbucketSignature must reject empty signature")
		}
	})
}

// FuzzVerifySignature asserts the provider-dispatching verifier never panics
// regardless of what ends up in headers or body. Only the no-panic invariant
// is checked — signatures are deliberately not pre-computed so we also cover
// the rejection paths.
func FuzzVerifySignature(f *testing.F) {
	f.Add("github", []byte(`{"ref":"refs/heads/main"}`), "sec", "sha256=aabb")
	f.Add("gitlab", []byte(`{}`), "tok", "tok")
	f.Add("bitbucket", []byte(`{}`), "sec", "")
	f.Add("generic", []byte(``), "", "")
	f.Add("unknown-provider", []byte(`garbage`), "secret", "sig")

	f.Fuzz(func(t *testing.T, provider string, body []byte, secret, sigHeader string) {
		req, _ := http.NewRequest("POST", "/hooks/v1/test", nil)
		req.Header.Set("X-Hub-Signature-256", sigHeader)
		req.Header.Set("X-Hub-Signature", sigHeader)
		req.Header.Set("X-Gitlab-Token", sigHeader)
		req.Header.Set("X-Gitea-Signature", sigHeader)
		req.Header.Set("X-Gogs-Signature", sigHeader)

		// Must return without panicking.
		_ = VerifySignature(context.Background(), provider, body, secret, req)
	})
}
