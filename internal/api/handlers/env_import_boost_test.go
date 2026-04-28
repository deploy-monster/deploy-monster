package handlers

import (
	"testing"
)

func TestSanitizeEnvValue(t *testing.T) {
	cases := []struct {
		input string
		want  string
	}{
		{"simple", `"simple"`},
		{"hello world", `"hello world"`},
		{`with\backslash`, `"with\\backslash"`},
		{`with"quotes`, `"with\"quotes"`},
		{`with$dollar`, `"with$$dollar"`},
		{"with\nnewline", `"with\nnewline"`},
		{"with\rcarriage", `"with\rcarriage"`},
		{"", `""`},
		{"mixed\\\"$\n\r", `"mixed\\\"$$\n\r"`},
	}

	for _, tc := range cases {
		t.Run(tc.input, func(t *testing.T) {
			got := sanitizeEnvValue(tc.input)
			if got != tc.want {
				t.Errorf("sanitizeEnvValue(%q) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}
