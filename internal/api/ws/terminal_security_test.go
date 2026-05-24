package ws

import "testing"

func TestTerminalCommandSafety_BlocksShellAndEvalFlags(t *testing.T) {
	tests := []struct {
		name    string
		command string
		want    bool
	}{
		{name: "normal command", command: "ls -la", want: true},
		{name: "shell command flag", command: "bash -c id", want: false},
		{name: "shell login command flag", command: "sh -lc id", want: false},
		{name: "python eval flag", command: "python3 -c 'print(1)'", want: false},
		{name: "node eval flag", command: "node --eval 'console.log(1)'", want: false},
		{name: "unknown command", command: "busybox sh", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isCommandSafe(tt.command); got != tt.want {
				t.Fatalf("isCommandSafe(%q) = %v, want %v", tt.command, got, tt.want)
			}
		})
	}
}
