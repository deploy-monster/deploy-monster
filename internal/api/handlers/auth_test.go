package handlers

import (
	"testing"
)

func TestGenerateSlug(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"John Doe", "john-doe"},
		{"My Team", "my-team"},
		{"UPPER", "upper"},
		{"with_underscores", "with-underscores"},
		{"special!@#chars", "specialchars"},
		{"Mixed 123 Case", "mixed-123-case"},
	}

	for _, tt := range tests {
		got := generateSlug(tt.input)
		if got != tt.want {
			t.Errorf("generateSlug(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}
