package webhooks

import (
	"fmt"
	"net/http"
	"testing"
)

func BenchmarkVerifyGitHubSignature(b *testing.B) {
	body := []byte(`{"ref":"refs/heads/main","head_commit":{"id":"abc123def456","message":"feat: add new feature"},"repository":{"clone_url":"https://github.com/test/repo.git","full_name":"test/repo"}}`)
	secret := "webhook-secret-key-123"
	mac := computeHMACSHA256(body, secret)
	sig := fmt.Sprintf("sha256=%s", mac)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		VerifyGitHubSignature(body, secret, sig)
	}
}

func BenchmarkParseGitHubPayload(b *testing.B) {
	body := []byte(`{
		"ref": "refs/heads/main",
		"head_commit": {
			"id": "abc123def456789",
			"message": "feat: add deployment pipeline",
			"author": {"name": "developer", "email": "dev@example.com"}
		},
		"repository": {
			"clone_url": "https://github.com/org/repo.git",
			"full_name": "org/repo"
		}
	}`)

	req, _ := http.NewRequest("POST", "/hooks/v1/test", nil)
	req.Header.Set("X-GitHub-Event", "push")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		parseGitHub(body, req)
	}
}
