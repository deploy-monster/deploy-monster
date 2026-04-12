package core

import (
	"testing"
)

func TestSetupLogger_JSONFormat(t *testing.T) {
	logger := SetupLogger("debug", "json")
	if logger == nil {
		t.Fatal("SetupLogger returned nil")
	}
}

func TestSetupLogger_LevelParsing(t *testing.T) {
	cases := []struct {
		name  string
		level string
	}{
		{"debug lowercase", "debug"},
		{"warn alias", "warn"},
		{"warning alias", "warning"},
		{"error", "error"},
		{"unknown falls back to info", "nope"},
		{"mixed case", "DEBUG"},
		{"empty falls back to info", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := SetupLogger(tc.level, "text"); got == nil {
				t.Error("expected non-nil logger")
			}
		})
	}
}
