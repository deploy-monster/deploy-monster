package compose

import "testing"

func TestIsWeakSecretDefault(t *testing.T) {
	cases := []struct {
		name  string
		value string
		want  bool
	}{
		{"empty", "", false},
		{"whitespace only", "   ", false},
		{"bare changeme", "changeme", true},
		{"changeme with whitespace", "  changeme  ", true},
		{"uppercase admin", "ADMIN", true},
		{"mixed-case minioadmin", "MinioAdmin", true},
		{"strong unique secret", "x9f4-q8k2-rT7v-jLm3", false},
		{"prefix of weak word but not equal", "changemenever", false},
		{"another stack default", "keycloak", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := isWeakSecretDefault(tc.value)
			if got != tc.want {
				t.Fatalf("isWeakSecretDefault(%q) = %v, want %v", tc.value, got, tc.want)
			}
		})
	}
}

// TestParseMemory_EdgeCases extends the basic coverage in parser_extra_test
// with the Sscanf-failure and fractional-GB branches.
func TestParseMemory_EdgeCases(t *testing.T) {
	cases := []struct {
		name  string
		input string
		want  int64
	}{
		{"fractional gb rounds down", "1.5g", 1536},
		{"trims whitespace", "  768m  ", 768},
		{"non-numeric mb falls through Sscanf", "abcm", 0},
		{"non-numeric gb falls through Sscanf", "abcg", 0},
		{"kb suffix is unknown unit", "100kb", 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := parseMemory(tc.input)
			if got != tc.want {
				t.Fatalf("parseMemory(%q) = %d, want %d", tc.input, got, tc.want)
			}
		})
	}
}
