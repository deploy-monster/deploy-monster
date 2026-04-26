package ingress

import "testing"

func TestURLParseSafe(t *testing.T) {
	tests := []struct {
		input string
		want  string
		err   bool
	}{
		{"https://example.com/path", "example.com", false},
		{"http://localhost:8080", "localhost", false},
		{"ftp://files.example.com", "files.example.com", false},
		{"not-a-url", "", true},
		{"https:///no-host", "", true},
		{"opaque:data", "", true},
		{"/just/a/path", "", true},
	}

	for _, tc := range tests {
		t.Run(tc.input, func(t *testing.T) {
			got, err := urlParseSafe(tc.input)
			if tc.err {
				if err == nil {
					t.Errorf("expected error for %q", tc.input)
				}
				return
			}
			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}
			if got != tc.want {
				t.Errorf("urlParseSafe(%q) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}
