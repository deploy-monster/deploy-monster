package core

import (
	"reflect"
	"testing"
)

func TestCommandTokensSafe(t *testing.T) {
	tests := []struct {
		name   string
		tokens []string
		want   bool
	}{
		{name: "allowed argv", tokens: []string{"ls", "-la"}, want: true},
		{name: "path command", tokens: []string{"/bin/echo", "ok"}, want: true},
		{name: "unknown command", tokens: []string{"busybox", "sh"}, want: false},
		{name: "shell eval flag", tokens: []string{"bash", "-c", "id"}, want: false},
		{name: "node eval flag", tokens: []string{"node", "--eval", "process.exit()"}, want: false},
		{name: "shell operator", tokens: []string{"echo", "ok", "&&", "id"}, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := CommandTokensSafe(tt.tokens); got != tt.want {
				t.Fatalf("CommandTokensSafe(%v) = %v, want %v", tt.tokens, got, tt.want)
			}
		})
	}
}

func TestSplitCommand(t *testing.T) {
	got := SplitCommand(`echo "hello world"`)
	want := []string{"echo", "hello world"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("SplitCommand = %v, want %v", got, want)
	}
}
