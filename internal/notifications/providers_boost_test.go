package notifications

import (
	"strings"
	"testing"
)

func TestValidateWebhookURL(t *testing.T) {
	cases := []struct {
		name    string
		url     string
		wantErr string
	}{
		{"empty", "", "webhook URL is required"},
		{"invalid URL", "://bad", "invalid webhook URL"},
		{"http scheme", "http://example.com/hook", "webhook URL must use HTTPS scheme"},
		{"missing hostname", "https:///path", "webhook URL must have a hostname"},
		{"localhost", "https://localhost/hook", "webhook URL cannot point to localhost"},
		{"127.0.0.1", "https://127.0.0.1/hook", "webhook URL cannot point to localhost"},
		{"::1", "https://[::1]/hook", "webhook URL cannot point to localhost"},
		{"0.0.0.0", "https://0.0.0.0/hook", "webhook URL cannot point to localhost"},
		{"private IP", "https://10.0.0.1/hook", "webhook URL cannot point to internal IP addresses"},
		{"link-local", "https://169.254.1.1/hook", "webhook URL cannot point to internal IP addresses"},
		{"cloud metadata", "https://169.254.169.254/hook", "webhook URL cannot point to internal IP addresses"},
		{"internal hostname", "https://metadata.google.internal/hook", "webhook URL cannot point to internal hostnames"},
		{"internal suffix", "https://sub.metadata.ec2.internal/hook", "webhook URL cannot point to internal hostnames"},
		{"valid slack", "https://hooks.slack.com/services/T00/B00/xxx", ""},
		{"valid discord", "https://discord.com/api/webhooks/123/abc", ""},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := validateWebhookURL(tc.url)
			if tc.wantErr == "" {
				if err != nil {
					t.Errorf("validateWebhookURL(%q) = %v, want nil", tc.url, err)
				}
				return
			}
			if err == nil {
				t.Fatalf("validateWebhookURL(%q) = nil, want error", tc.url)
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Errorf("validateWebhookURL(%q) = %q, want containing %q", tc.url, err.Error(), tc.wantErr)
			}
		})
	}
}
