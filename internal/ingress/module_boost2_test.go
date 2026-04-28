package ingress

import (
	"testing"
)

func TestValidateHostname_Valid(t *testing.T) {
	cases := []string{
		"localhost",
		"example.com",
		"sub.example.com",
		"a-b.example.com",
		"127.0.0.1",
		"10.0.0.1",
		"192.168.1.1",
		"0.0.0.0",
	}
	for _, host := range cases {
		t.Run(host, func(t *testing.T) {
			if err := validateHostname(host); err != nil {
				t.Errorf("validateHostname(%q) = %v, want nil", host, err)
			}
		})
	}
}

func TestValidateHostname_Invalid(t *testing.T) {
	cases := []struct {
		host string
		want string
	}{
		{"", "invalid hostname label"},
		{"-invalid.com", "hostname label must start and end with alphanumeric"},
		{"invalid-.com", "hostname label must start and end with alphanumeric"},
		{"a..b.com", "invalid hostname label"},
		{"a_b.com", "invalid character in hostname"},
		{"8.8.8.8", "public IP not allowed in redirect"},
		{"1.1.1.1", "public IP not allowed in redirect"},
	}
	for _, tc := range cases {
		t.Run(tc.host, func(t *testing.T) {
			err := validateHostname(tc.host)
			if err == nil {
				t.Fatalf("validateHostname(%q) = nil, want error", tc.host)
			}
			if err.Error() != tc.want {
				t.Errorf("validateHostname(%q) = %q, want %q", tc.host, err.Error(), tc.want)
			}
		})
	}
}

func TestValidateHostname_TooLong(t *testing.T) {
	// 254 'a's — one over the 253 limit
	host := make([]byte, 254)
	for i := range host {
		host[i] = 'a'
	}
	err := validateHostname(string(host))
	if err == nil || err.Error() != "hostname too long" {
		t.Errorf("expected 'hostname too long', got %v", err)
	}
}

func TestValidateHostname_LabelTooLong(t *testing.T) {
	// 64 'a's in a single label
	label := make([]byte, 64)
	for i := range label {
		label[i] = 'a'
	}
	err := validateHostname(string(label) + ".com")
	if err == nil || err.Error() != "invalid hostname label" {
		t.Errorf("expected 'invalid hostname label', got %v", err)
	}
}

func TestIsValidRedirectHost(t *testing.T) {
	cases := []struct {
		host string
		want bool
	}{
		{"example.com", true},
		{"localhost", true},
		{"127.0.0.1", true},
		{"10.0.0.1:8080", true},
		{"", false},
		{"example.com:8080", true},
		{"bad\r\nhost", false},
		{"http://evil.com", false},
		{"https://evil.com", false},
		{"user:pass@host.com", false},
		{"8.8.8.8", false},
	}
	for _, tc := range cases {
		t.Run(tc.host, func(t *testing.T) {
			got := isValidRedirectHost(tc.host)
			if got != tc.want {
				t.Errorf("isValidRedirectHost(%q) = %v, want %v", tc.host, got, tc.want)
			}
		})
	}
}
