package handlers

import "testing"

func TestExecCommandSafety_BlocksShellAndEvalFlags(t *testing.T) {
	tests := []struct {
		name   string
		tokens []string
		want   bool
	}{
		{name: "normal command", tokens: []string{"ls", "-la"}, want: true},
		{name: "shell command flag", tokens: []string{"bash", "-c", "id"}, want: false},
		{name: "shell login command flag", tokens: []string{"sh", "-lc", "id"}, want: false},
		{name: "python eval flag", tokens: []string{"python3", "-c", "print(1)"}, want: false},
		{name: "node eval flag", tokens: []string{"node", "--eval", "console.log(1)"}, want: false},
		{name: "unknown command", tokens: []string{"busybox", "sh"}, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := areCommandTokensSafe(tt.tokens); got != tt.want {
				t.Fatalf("areCommandTokensSafe(%v) = %v, want %v", tt.tokens, got, tt.want)
			}
		})
	}
}
