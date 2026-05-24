package core

import "testing"

func TestShortID(t *testing.T) {
	tests := []struct {
		name string
		id   string
		max  int
		want string
	}{
		{name: "shorter", id: "abc", max: 8, want: "abc"},
		{name: "exact", id: "abcdefgh", max: 8, want: "abcdefgh"},
		{name: "longer", id: "abcdefghi", max: 8, want: "abcdefgh"},
		{name: "zero", id: "abcdefghi", max: 0, want: "abcdefghi"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ShortID(tt.id, tt.max); got != tt.want {
				t.Fatalf("ShortID(%q, %d) = %q, want %q", tt.id, tt.max, got, tt.want)
			}
		})
	}
}
