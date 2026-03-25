package mcp

import (
	"testing"
)

// The only uncovered code in mcp is init() at 50%.
// init() functions are partially covered by the test binary startup.
// This file ensures the module is loadable and functional.

func TestModule_ID_Final(t *testing.T) {
	m := New()
	if m.ID() != "mcp" {
		t.Errorf("ID = %q, want %q", m.ID(), "mcp")
	}
}
