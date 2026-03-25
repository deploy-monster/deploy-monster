package dns

import (
	"testing"
)

// The only uncovered code in dns is init() at 50%.
// init() functions are partially covered by the test binary startup.
// This file ensures the module is loadable and functional.

func TestModule_ID_Final(t *testing.T) {
	m := New()
	if m.ID() != "dns.sync" {
		t.Errorf("ID = %q, want %q", m.ID(), "dns.sync")
	}
}
