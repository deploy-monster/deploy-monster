package strategies

import "testing"

func TestNew(t *testing.T) {
	tests := []struct {
		name string
		want string
	}{
		{"recreate", "recreate"},
		{"rolling", "rolling"},
		{"", "recreate"},        // default
		{"unknown", "recreate"}, // fallback
	}

	for _, tt := range tests {
		s := New(tt.name)
		if s.Name() != tt.want {
			t.Errorf("New(%q).Name() = %q, want %q", tt.name, s.Name(), tt.want)
		}
	}
}
