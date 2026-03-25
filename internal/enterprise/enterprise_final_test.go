package enterprise

import (
	"testing"
)

// The only uncovered code in enterprise is init() at 50%.
// init() functions are partially covered by the test binary startup.
// We cannot increase init() coverage beyond what's inherently exercised.
// This file ensures the module is loadable and functional.

func TestModule_ID_Final(t *testing.T) {
	m := New()
	if m.ID() != "enterprise" {
		t.Errorf("ID = %q, want %q", m.ID(), "enterprise")
	}
}
