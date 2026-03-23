package deploy

import "testing"

func TestSanitizeSlug(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"my-app", "my-app"},
		{"My App", "my-app"},
		{"UPPER_CASE", "upper-case"},
		{"app.name.v2", "app-name-v2"},
		{"---leading---", "leading"},
		{"special!@#chars", "specialchars"},
		{"123numbers", "123numbers"},
		{"a", "a"},
	}

	for _, tt := range tests {
		got := sanitizeSlug(tt.input)
		if got != tt.want {
			t.Errorf("sanitizeSlug(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestSanitizeSlug_Empty(t *testing.T) {
	got := sanitizeSlug("!@#$%")
	if got == "" {
		t.Error("empty input should generate an ID-based slug")
	}
	if len(got) < 4 {
		t.Error("generated slug should be at least 4 chars")
	}
}
