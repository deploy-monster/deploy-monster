package build

import (
	"strings"
	"testing"
)

func TestValidateDockerImageTag(t *testing.T) {
	cases := []struct {
		name    string
		tag     string
		wantErr bool
	}{
		{"empty", "", true},
		{"simple", "nginx", false},
		{"with tag", "nginx:latest", false},
		{"registry path", "registry.example.com/app:v1", false},
		{"with digest", "nginx@sha256:abc123", false},
		{"complex", "my-registry.io:5000/user/app:v1.0.0-beta", true},
		{"invalid start char", ":latest", true},
		{"invalid slash start", "/nginx", true},
		{"invalid char space", "nginx latest", true},
		{"invalid char", "nginx|latest", true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := validateDockerImageTag(tc.tag)
			if (err != nil) != tc.wantErr {
				t.Errorf("validateDockerImageTag(%q) error = %v, wantErr %v", tc.tag, err, tc.wantErr)
			}
		})
	}
}

func TestValidateBuildArg(t *testing.T) {
	cases := []struct {
		name    string
		key     string
		value   string
		wantErr bool
	}{
		{"valid", "KEY", "value", false},
		{"valid underscore", "_KEY", "value", false},
		{"valid mixed", "MY_KEY", "some_value", false},
		{"invalid key number start", "123KEY", "value", true},
		{"invalid key hyphen", "MY-KEY", "value", true},
		{"control char null", "KEY", "val\x00ue", true},
		{"control char newline", "KEY", "val\nue", true},
		{"control char carriage return", "KEY", "val\rue", true},
		{"flag injection", "KEY", "--flag", true},
		{"flag injection single", "KEY", "-f", true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := validateBuildArg(tc.key, tc.value)
			if (err != nil) != tc.wantErr {
				t.Errorf("validateBuildArg(%q, %q) error = %v, wantErr %v", tc.key, tc.value, err, tc.wantErr)
			}
		})
	}
}

func TestIsPrivateOrBlockedIP(t *testing.T) {
	cases := []struct {
		host string
		want bool
	}{
		{"example.com", false},
		{"8.8.8.8", false},
		{"1.1.1.1", false},
		{"10.0.0.1", true},
		{"172.16.0.1", true},
		{"192.168.1.1", true},
		{"127.0.0.1", true},
		{"169.254.1.1", true},
		{"0.0.0.0", true},
		{"", false},
	}

	for _, tc := range cases {
		t.Run(tc.host, func(t *testing.T) {
			got := isPrivateOrBlockedIP(tc.host)
			if got != tc.want {
				t.Errorf("isPrivateOrBlockedIP(%q) = %v, want %v", tc.host, got, tc.want)
			}
		})
	}
}

func TestValidateResolvedHost(t *testing.T) {
	tests := []struct {
		name    string
		url     string
		wantErr bool
		errSub  string
	}{
		{"invalid_url", "://not-a-url", true, ""},
		{"file_scheme", "file:///etc/passwd", false, ""},
		{"ftp_scheme", "ftp://example.com/file", false, ""},
		{"ssh_empty_host", "ssh:///path/to/repo", false, ""},
		{"https_empty_host", "https:///no-host", false, ""},
		{"http_empty_host", "http:///no-host", false, ""},
		{"unresolvable_host", "https://this-host-definitely-does-not-exist-xyz.invalid/owner/repo", true, "DNS lookup failed"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateResolvedHost(tt.url)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				if tt.errSub != "" && !strings.Contains(err.Error(), tt.errSub) {
					t.Errorf("error = %q, want containing %q", err.Error(), tt.errSub)
				}
				return
			}
			if err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}
